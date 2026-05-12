package workloads

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"

	o "github.com/onsi/gomega"

	"math/rand"
	"net/http"

	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ControlplaneInfo ...
type ControlplaneInfo struct {
	HolderIdentity       string `json:"holderIdentity"`
	LeaseDurationSeconds int    `json:"leaseDurationSeconds"`
	AcquireTime          string `json:"acquireTime"`
	RenewTime            string `json:"renewTime"`
	LeaderTransitions    int    `json:"leaderTransitions"`
}

type serviceInfo struct {
	serviceIP   string
	namespace   string
	servicePort string
	serviceURL  string
	serviceName string
}

type registry struct {
	dockerImage string
	namespace   string
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "workload-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)

	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func (registry *registry) createregistry(oc *exutil.CLI) serviceInfo {
	err := oc.AsAdmin().Run("new-app").Args("--image", registry.dockerImage, "REGISTRY_STORAGE_DELETE_ENABLED=true", "--import-mode=PreserveOriginal", "-n", registry.namespace).Execute()
	if err != nil {
		e2e.Failf("Failed to create the registry server")
	}
	err = oc.AsAdmin().Run("set").Args("probe", "deploy/registry", "--readiness", "--liveness", "--get-url="+"http://:5000/v2", "-n", registry.namespace).Execute()
	if err != nil {
		e2e.Failf("Failed to config the registry with err %v", err)
	}
	if ok := waitForAvailableRsRunning(oc, "deployment", "registry", registry.namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		e2e.Logf("private registry pod is not running even afer waiting for about 3 minutes")
		oc.AsAdmin().Run("get").Args("-l", "deployment=registry", "-n", registry.namespace, "-o=jsonpath={.items[*].status}").Execute()
	}

	err = waitForPodWithLabelReady(oc, registry.namespace, "deployment=registry")
	compat_otp.AssertWaitPollNoErr(err, "this pod with label deployment=registry not ready")
	err = waitForPodContainerWithLabelReady(oc, registry.namespace, "deployment=registry")
	compat_otp.AssertWaitPollNoErr(err, "this pod with label deployment=registry container not ready")

	e2e.Logf("Get the service info of the registry")
	regSvcIP, err := oc.AsAdmin().Run("get").Args("svc", "registry", "-n", registry.namespace, "-o=jsonpath={.spec.clusterIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("create").Args("route", "edge", "my-route", "--service=registry", "-n", registry.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	regSvcPort, err := oc.AsAdmin().Run("get").Args("svc", "registry", "-n", registry.namespace, "-o=jsonpath={.spec.ports[0].port}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	regRoute, err := oc.AsAdmin().Run("get").Args("route", "my-route", "-n", registry.namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	regSvcURL := regSvcIP + ":" + regSvcPort
	svc := serviceInfo{
		serviceIP:   regSvcIP,
		namespace:   registry.namespace,
		servicePort: regSvcPort,
		serviceURL:  regSvcURL,
		serviceName: regRoute,
	}
	return svc

}

func (registry *registry) deleteregistry(oc *exutil.CLI) {
	_ = oc.Run("delete").Args("svc", "registry", "-n", registry.namespace).Execute()
	_ = oc.Run("delete").Args("deploy", "registry", "-n", registry.namespace).Execute()
	_ = oc.Run("delete").Args("is", "registry", "-n", registry.namespace).Execute()
}

func getScanNodesLabels(oc *exutil.CLI, nodeList []string, expected string) []string {
	var machedLabelsNodeNames []string
	for _, nodeName := range nodeList {
		nodeLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(expected, nodeLabels); matched {
			machedLabelsNodeNames = append(machedLabelsNodeNames, nodeName)
		}
	}
	return machedLabelsNodeNames
}

func assertMultiImage(url, authfile string) bool {
	cmd := fmt.Sprintf(`skopeo inspect --raw --authfile %s --tls-verify=false docker://%s | jq .mediaType`, authfile, url)
	e2e.Logf("cmd: %v", cmd)
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	result := string(output)
	if err != nil {
		e2e.Logf("the output: %v with error %v", result, err)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	e2e.Logf("the correct output: %v", result)
	if strings.Contains(result, "application/vnd.docker.distribution.manifest.list.v2+json") ||
		strings.Contains(result, "application/vnd.oci.image.index.v1+json") {
		return true
	}
	return false
}

func copyFile(source string, dest string) {
	bytesRead, err := ioutil.ReadFile(source)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = ioutil.WriteFile(dest, bytesRead, 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func locatePodmanCred(oc *exutil.CLI, dst string) error {
	err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dst, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	key := "XDG_RUNTIME_DIR"
	currentRuntime, ex := os.LookupEnv(key)
	if !ex {
		err = os.MkdirAll("/tmp/configocmirror/containers", 0700)
		o.Expect(err).NotTo(o.HaveOccurred())
		os.Setenv(key, "/tmp/configocmirror")
		copyFile(dst+"/"+".dockerconfigjson", "/tmp/configocmirror/containers/auth.json")
		return nil
	}
	_, err = os.Stat(currentRuntime + "/containers/auth.json")
	if os.IsNotExist(err) {
		err1 := os.MkdirAll(currentRuntime+"/containers", 0700)
		o.Expect(err1).NotTo(o.HaveOccurred())
		copyFile(dst+"/"+".dockerconfigjson", currentRuntime+"/containers/auth.json")
		return nil
	}
	if err != nil {
		return err
	}
	return nil
}

func getCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(.type == '%s')].status}`, key)
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", args, coName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

func (registry *registry) createregistrySpecifyName(oc *exutil.CLI, registryname string) serviceInfo {
	err := oc.AsAdmin().Run("new-app").Args("--image", registry.dockerImage, "REGISTRY_STORAGE_DELETE_ENABLED=true", "--name", registryname, "-n", registry.namespace, "--import-mode=PreserveOriginal").Execute()
	if err != nil {
		e2e.Failf("Failed to create the registry server")
	}
	err = oc.AsAdmin().Run("set").Args("probe", "deploy/"+registryname, "--readiness", "--liveness", "--get-url="+"http://:5000/v2", "-n", registry.namespace).Execute()
	if err != nil {
		e2e.Failf("Failed to config the registry")
	}
	if ok := waitForAvailableRsRunning(oc, "deployment", registryname, registry.namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		e2e.Failf("private registry pod is not running even afer waiting for about 3 minutes")
	}

	e2e.Logf("Get the service info of the registry")
	regSvcIP, err := oc.AsAdmin().Run("get").Args("svc", registryname, "-n", registry.namespace, "-o=jsonpath={.spec.clusterIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("create").Args("route", "edge", registryname, "--service="+registryname, "-n", registry.namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	regSvcPort, err := oc.AsAdmin().Run("get").Args("svc", registryname, "-n", registry.namespace, "-o=jsonpath={.spec.ports[0].port}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	regRoute, err := oc.AsAdmin().Run("get").Args("route", registryname, "-n", registry.namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Check the route of registry available")
	ingressOpratorPod, err := oc.AsAdmin().Run("get").Args("pod", "-l", "name=ingress-operator", "-n", "openshift-ingress-operator", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	waitErr := wait.Poll(5*time.Second, 90*time.Second, func() (bool, error) {
		err := oc.AsAdmin().Run("exec").Args("pod/"+ingressOpratorPod, "-n", "openshift-ingress-operator", "--", "curl", "-v", "https://"+regRoute, "-I", "-k").Execute()
		if err != nil {
			e2e.Logf("route is not yet resolving, retrying...")
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the route is not reachable"))

	regSvcURL := regSvcIP + ":" + regSvcPort
	svc := serviceInfo{
		serviceIP:   regSvcIP,
		namespace:   registry.namespace,
		servicePort: regSvcPort,
		serviceURL:  regSvcURL,
		serviceName: regRoute,
	}
	return svc
}

// check data by running curl on a pod

func assertPodOutput(oc *exutil.CLI, podLabel string, namespace string, expected string) {
	err := wait.PollUntilContextTimeout(context.Background(), 1*time.Minute, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		podStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", podLabel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the result of pod:%v", podStatus)
		if strings.Contains(podStatus, expected) {
			return true, nil
		} else {
			podDesp, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pods", "-n", namespace, "-l", podLabel).Output()
			e2e.Logf("the details of pod: %v", podDesp)
			o.Expect(err).NotTo(o.HaveOccurred())
			return false, nil
		}
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("the state of pod with %s is not expected %s", podLabel, expected))
}

func executeBashWithTimeout(timeout time.Duration, command string) (string, error) {
	// 1. Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // Always call cancel to release resources

	// 2. Use CommandContext to bind the command execution to the context
	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	// 3. Run the command and capture output
	output, err := cmd.Output()

	// Check for context-related errors first (Timeout)
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %v", timeout)
	}

	// Handle standard execution errors (non-zero exit code, etc.)
	if err != nil {
		// Output the full stderr/stdout on failure for better debugging
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("command failed with exit code %d: %s",
				exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func getOperatorInfo(oc *exutil.CLI, operatorName string, operatorNamespace string, catalogName string, catalogSourceName string) (*customsub, *operatorgroup) {
	getOperatorChannelCMD := fmt.Sprintf("oc-mirror list operators --catalog %s --v1 | grep -E \"^%s\\s+\" |awk '{print $NF}'", catalogName, operatorName)
	e2e.Logf("The command get operator channel %v", getOperatorChannelCMD)
	channel, err := executeBashWithTimeout(3*time.Minute, getOperatorChannelCMD)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The default channel %v", string(channel))
	channelName := strings.ReplaceAll(string(channel), "\n", "")
	e2e.Logf("The default channel name %s", channelName)

	getstartingCSVCMD := fmt.Sprintf("oc-mirror list operators --catalog %s --package %s --v1 |awk '{if($2~/^%s$/) print $3}'", catalogName, operatorName, channelName)
	e2e.Logf("The command get operator csv %v", getstartingCSVCMD)
	e2e.Logf("The csv name: %v", getstartingCSVCMD)
	startingCsv, err := executeBashWithTimeout(3*time.Minute, getstartingCSVCMD)
	o.Expect(err).NotTo(o.HaveOccurred())
	startingCsvName := strings.ReplaceAll(string(startingCsv), "\n", "")
	e2e.Logf("The csv name: %v", startingCsvName)

	buildPruningBaseDir := compat_otp.FixturePath("testdata", "workloads")
	subscriptionT := filepath.Join(buildPruningBaseDir, "customsub.yaml")
	operatorGroupT := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")

	sub := customsub{
		name:        operatorName,
		namespace:   operatorNamespace,
		channelName: channelName,
		opsrcName:   catalogSourceName,
		sourceName:  "openshift-marketplace",
		startingCSV: startingCsvName,
		template:    subscriptionT,
	}

	og := operatorgroup{
		name:      operatorName,
		namespace: operatorNamespace,
		template:  operatorGroupT,
	}

	return &sub, &og
}

// install the operator from custom catalog source and check the operator running pods numbers as 1
func installOperatorFromCustomCS(oc *exutil.CLI, operatorSub *customsub, operatorOG *operatorgroup, operatorNamespace string, operatorDeoloy string) {
	installCustomOperator(oc, operatorSub, operatorOG, operatorNamespace, operatorDeoloy, "1")
}

func removeOperatorFromCustomCS(oc *exutil.CLI, operatorSub *customsub, operatorOG *operatorgroup, operatorNamespace string) {
	e2e.Logf("Remove the subscription")
	operatorSub.deleteCustomSubscription(oc)

	e2e.Logf("Remove the operatorgroup")
	operatorOG.deleteOperatorGroup(oc)

	e2e.Logf("Remove the operatornamespace")
	// oc patch daemonset bpfman-daemon --type=json -p '[{"op":"remove", "path":"/metadata/finalizers"}]' -n bpfman-operator-ns
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", operatorNamespace, "--force=true", "--timeout=1m").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getRouteCAToFile(oc *exutil.CLI, dirname string) (err error) {
	if err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/router-ca", "-n", "openshift-ingress-operator",
		"--to="+dirname, "--confirm").Execute(); err != nil {
		return fmt.Errorf("failed to acquire default route ca bundle: %v", err)
	}
	return
}

// Configure the Registry Certificate as trusted for cincinnati
func trustCert(oc *exutil.CLI, registry string, cert string, configmapName string) (err error) {
	var output string
	certRegistry := registry
	before, after, found := strings.Cut(registry, ":")
	if found {
		certRegistry = before + ".." + after
	}

	if err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-config", "configmap", configmapName, "--from-file="+certRegistry+"="+cert, "--from-file=updateservice-registry="+cert).Execute(); err != nil {
		err = fmt.Errorf("create trust-ca configmap failed: %v", err)
		return
	}
	if err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "-p", fmt.Sprintf("{\"spec\": {\"additionalTrustedCA\": {\"name\": \"%s\"}}}", configmapName), "--type=merge").Execute(); err != nil {
		err = fmt.Errorf("patch image.config.openshift.io/cluster failed: %v", err)
		return
	}
	waitErr := wait.Poll(60*time.Second, 10*time.Minute, func() (bool, error) {
		registryHealth := checkCOHealth(oc, "image-registry")
		if registryHealth {
			return true, nil
		}
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].message}").Output()
		e2e.Logf("Waiting for image-registry coming ready...")
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready with info %s\n", output))
	return nil
}

func restoreAddCA(oc *exutil.CLI, addCA string, configmapName string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-config", "configmap", configmapName).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	var message string
	if addCA == "" {
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "--type=json", "-p", "[{\"op\":\"remove\", \"path\":\"/spec/additionalTrustedCA\"}]").Execute()
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("image.config.openshift.io/cluster", "--type=merge", "--patch", fmt.Sprintf("{\"spec\":{\"additionalTrustedCA\":%s}}", addCA)).Execute()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	waitErr := wait.Poll(60*time.Second, 10*time.Minute, func() (bool, error) {
		registryHealth := checkCOHealth(oc, "image-registry")
		if registryHealth {
			return true, nil
		}
		message, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].message}").Output()
		e2e.Logf("Wait for image-registry coming ready")
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Image registry is not ready with info %s\n", message))
}

// Check if image-registry is healthy
func checkCOHealth(oc *exutil.CLI, co string) bool {
	e2e.Logf("Checking CO %s is healthy...", co)
	status := "TrueFalseFalse"
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", co, "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}").Output()
	if err != nil {
		e2e.Logf("Get co status failed: %v", err.Error())
		return false
	}
	return strings.Contains(output, status)
}

// create or delete the catalogsource, idms and itms by yaml files
func operateCSAndMs(oc *exutil.CLI, rootPath string, operation string) {
	var files []string
	var yamlFiles []string
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		e2e.Failf("Can't walk the rootpath directory")
	}

	for _, file := range files {
		if matched, _ := regexp.MatchString("yaml", file); matched {
			fmt.Printf("file name is %f \n", file)
			yamlFiles = append(yamlFiles, file)
		}
	}

	for _, yamlFileName := range yamlFiles {
		if strings.Contains(operation, "create") {
			err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", yamlFileName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else if strings.Contains(operation, "delete") {
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", yamlFileName).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

	}
}

func setRegistryVolume(oc *exutil.CLI, resourcetype string, resourcename string, namespace string, volumeSize string, mountPath string) {
	err := oc.AsAdmin().Run("set").Args("volume", resourcetype, resourcename, "--add", "-t", "pvc", "-n", namespace, "--claim-size="+volumeSize, "-m", mountPath, "--overwrite").Execute()
	if err != nil {
		e2e.Failf("Failed to set volume for the resource %s", resourcename)
	}
	if ok := waitForAvailableRsRunning(oc, resourcetype, resourcename, namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		e2e.Failf("The pod is not running even afer waiting for about 3 minutes")
	}
}

func validateTargetcatalogAndTag(regsitryUri string, expectedTag string) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(regsitryUri)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The output  %v :", strings.ReplaceAll(string(content), "\n", ""))
	if strings.Contains(strings.ReplaceAll(string(content), "\n", ""), expectedTag) {
		e2e.Logf("Find the expected tag %v :", expectedTag)
	} else {
		e2e.Failf("Can't find the expected tag %v", expectedTag)
	}
}

func validateRepoSignature(serviceName string, prefixFilter string) error {
	return validateRepo(serviceName, prefixFilter, true)
}

func validTagSignature(tags []string) error {
	baseHashPattern := `^sha256-[0-9a-f]{64}$`
	sigHashPattern := `^sha256-[0-9a-f]{64}\.sig$`
	baseRegex := regexp.MustCompile(baseHashPattern)
	sigRegex := regexp.MustCompile(sigHashPattern)

	// 1. Separate the base hashes from the signed hashes
	baseHashes := make(map[string]struct{})
	signedHashes := make(map[string]struct{})

	for i, tag := range tags {
		if baseRegex.MatchString(tag) {
			baseHashes[tag] = struct{}{}
		} else if sigRegex.MatchString(tag) {
			// Extract the base part (remove the ".sig")
			base := tag[:len(tag)-4]
			signedHashes[base] = struct{}{}
		} else {
			return fmt.Errorf("invalid entry found at index %d: '%s'. Does not match hash or hash.sig pattern", i, tag)
		}
	}

	// 2. Iterate over all found base hashes and check for a signed counterpart
	for baseHash := range baseHashes {
		if _, found := signedHashes[baseHash]; !found {
			return fmt.Errorf("missing required signed tag for base hash: '%s'. Expected '%s.sig'", baseHash, baseHash)
		}
	}

	if len(signedHashes) == 0 {
		return fmt.Errorf("no signed hashes found")
	}

	return nil // Validation passed
}

// CatalogResponse matches the JSON structure of the /v2/_catalog endpoint.
type CatalogResponse struct {
	Repositories []string `json:"repositories"`
}

// TagsResponse matches the JSON structure of the /v2/<repo>/tags/list endpoint.
type TagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func validateRepoTags(serviceName string, prefixFilter string) error {
	return validateRepo(serviceName, prefixFilter, false)
}
func validateRepo(serviceName string, prefixFilter string, checkSignature bool) error {
	registryUri := fmt.Sprintf("https://%s/v2/_catalog", serviceName)

	e2e.Logf("Valide %s tags: the registries catalog url is %v", prefixFilter, registryUri)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(registryUri)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' fail to access the registry", prefixFilter)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var catalog CatalogResponse
	err = json.Unmarshal(content, &catalog)
	if err != nil {
		return err
	}

	var filteredRepositories []string
	prefixToFilter := prefixFilter + "/"
	for _, repo := range catalog.Repositories {
		if strings.HasPrefix(repo, prefixToFilter) {
			filteredRepositories = append(filteredRepositories, repo)
		}
	}
	if len(filteredRepositories) == 0 {
		return fmt.Errorf("fail to get the filtered registry")
	}
	hasTag := false
	for _, repo := range filteredRepositories {
		e2e.Logf("\nFetching tags for repository: %s\n", repo)

		tagsURL := fmt.Sprintf("https://%s/v2/%s/tags/list", serviceName, repo)

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}
		tagsResp, err := client.Get(tagsURL)
		if err != nil {
			return err
		}
		defer tagsResp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fail to access %s", tagsURL)
		}

		tagsBody, err := io.ReadAll(tagsResp.Body)
		if err != nil {
			return err
		}

		var tags TagsResponse
		err = json.Unmarshal(tagsBody, &tags)
		if err != nil {
			return err
		}
		if len(tags.Tags) == 0 {
			return fmt.Errorf("fail to find tags %v", tags)
		}

		hasTag = true
		// To replicate the pretty-printing of `jq`, we marshal the struct back to indented JSON.
		prettyTags, err := json.MarshalIndent(tags, "", "  ")
		if err != nil {
			return err
		}

		e2e.Logf("%s", string(prettyTags))

		if checkSignature {
			if !strings.HasSuffix(tags.Name, "redhat-operator-index") {
				err = validTagSignature(tags.Tags)
				if err != nil {
					return fmt.Errorf("failed to validate signature for %s: %v", tags.Name, err)
				}
			}
		}
	}

	if len(filteredRepositories) <= 1 || !hasTag {
		return fmt.Errorf("'%s' Fail to find the tags in the filterd registry", prefixFilter)
	}

	e2e.Logf("Found %d repositories starting with '%s'\n", len(filteredRepositories), prefixToFilter)

	return nil
}

func skopeExecute(skopeoCommand string) {
	e2e.Logf("skopeo command %v :", skopeoCommand)
	var skopeooutStr string
	waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
		skopeoout, err := exec.Command("bash", "-c", skopeoCommand).Output()
		if err != nil {
			e2e.Logf("copy failed, retrying...")
			skopeooutStr = string(skopeoout)
			return false, nil
		}
		return true, nil

	})
	if waitErr != nil {
		e2e.Logf("output: %v", skopeooutStr)
	}
	compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the skopeo copy still failed")
}

func readFileContent(filePath string) string {
	bytesRead, err := ioutil.ReadFile(filePath)
	o.Expect(err).NotTo(o.HaveOccurred())
	return string(bytesRead)
}

func validateFileContent(fileContent string, expectedStr string, resourceType string) {
	if matched, _ := regexp.MatchString(expectedStr, fileContent); !matched {
		e2e.Failf("Nest path for %s are not right", resourceType)
	} else {
		e2e.Logf("Nest paths for %s are set correctly", resourceType)
	}
}

// Since no regenerate the cache by default for oc-mirror , so add this function, just create all the resources and don't do packagemainifest check.

func validateStringFromFile(fileName string, expectedS string) bool {
	content, err := ioutil.ReadFile(fileName)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(string(content), expectedS) {
		return true
	}
	return false
}

func getHomePath() string {
	var homePath string
	u, err := user.Current()
	if err != nil {
		e2e.Logf("Failed to get the current user, pod running in Openshift")
		homePath = os.Getenv("HOME")
	} else {
		homePath = u.HomeDir
	}
	return homePath
}

func ensureContainersConfigDirectory(homePath string) (bool, error) {
	_, errStat := os.Stat(homePath + "/.config/containers")
	if errStat == nil {
		return true, nil
	}
	if os.IsNotExist(errStat) {
		err := os.MkdirAll(homePath+"/.config/containers", 0700)
		if err != nil {
			e2e.Logf("Failed to create the default continer config directory")
			return false, nil
		} else {
			return true, nil
		}
	}
	return false, nil
}

func backupContainersConfig(homePath string) {
	oldRegistryConf := homePath + "/.config/containers/registries.conf"
	backupRegistryConf := homePath + "/.config/containers/registries.conf.backup"
	e2e.Logf("Backup origin files")
	err := os.Rename(oldRegistryConf, backupRegistryConf)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func restoreRegistryConf(homePath string) {
	_, errStat := os.Stat(homePath + "/.config/containers/registries.conf.backup")

	if errStat == nil {
		e2e.Logf("Remove registry.conf file")
		os.Remove(homePath + "/.config/containers/registries.conf")
		e2e.Logf("Backup origin files")
		os.Rename(homePath+"/.config/containers/registries.conf.backup", homePath+"/.config/containers/registries.conf")
	} else if os.IsNotExist(errStat) {
		os.Remove(homePath + "/.config/containers/registries.conf")
	} else {
		e2e.Failf("Unexpected error %v", errStat)
	}
}

func getRegistryConfContentStr(registryRoute string, originRegistryF string, originRegistryS string) string {
	registryConfContent := fmt.Sprintf(`[[registry]]
  location = "%s"
  insecure = false
  blocked = false
  mirror-by-digest-only = false
  [[registry.mirror]]
    location = "%s"
    insecure = false
[[registry]]
  location = "%s"
  insecure = false
  blocked = false
  mirror-by-digest-only = false
  [[registry.mirror]]
    location = "%s"
    insecure = false
 `, originRegistryF, registryRoute, originRegistryS, registryRoute)

	return registryConfContent
}

func setRegistryConf(registryConfContent string, homePath string) {
	f, err := os.Create(homePath + "/.config/containers/registries.conf")
	o.Expect(err).NotTo(o.HaveOccurred())
	defer f.Close()
	w := bufio.NewWriter(f)
	_, werr := w.WriteString(registryConfContent)
	w.Flush()
	o.Expect(werr).NotTo(o.HaveOccurred())
}

// delete registry app with special name
func (registry *registry) deleteregistrySpecifyName(oc *exutil.CLI, registryName string) {
	_ = oc.Run("delete").Args("svc", registryName, "-n", registry.namespace).Execute()
	_ = oc.Run("delete").Args("deploy", registryName, "-n", registry.namespace).Execute()
	_ = oc.Run("delete").Args("is", registryName, "-n", registry.namespace).Execute()
}

func installAllNSOperatorFromCustomCS(oc *exutil.CLI, operatorSub *customsub, operatorOG *operatorgroup, operatorNamespace string, operatorDeoloy string, operatorunningNum string) {
	e2e.Logf("Create the operator namespace")
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", operatorNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Create the subscription")
	operatorSub.createCustomSub(oc)

	e2e.Logf("Create the operatorgroup")
	operatorOG.createOperatorGroup(oc)

	e2e.Logf("Remove the target Namespaces")
	err = oc.AsAdmin().WithoutNamespace().Run("get").Args("operatorgroup", "-n", operatorNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	pathYaml := `[{"op": "remove", "path": "/spec/targetNamespaces"}]`
	err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("og", operatorOG.name, "--type=json", "-p", pathYaml, "-n", operatorNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Wait for the operator pod running")
	if ok := waitForAvailableRsRunning(oc, "deploy", operatorDeoloy, operatorNamespace, operatorunningNum); ok {
		e2e.Logf("installed operator runnnig now\n")
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", operatorNamespace, "sub", "-o", "yaml").Execute()
		if err != nil {
			e2e.Logf("Could not retreive subscription")
		} else {
			err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", operatorNamespace, "sub").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		oc.AsAdmin().WithoutNamespace().Run("get").Args("all", "-n", operatorNamespace).Execute()
		oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", operatorNamespace).Execute()
		fmt.Println("Debug......")
		time.Sleep(20 * time.Minute)
		e2e.Failf("All pods related to deployment are not running")
	}
}

// install the operator from custom catalog source and check the operator running pods numbers as expected
func installCustomOperator(oc *exutil.CLI, operatorSub *customsub, operatorOG *operatorgroup, operatorNamespace string, operatorDeoloy string, operatorunningNum string) {
	e2e.Logf("Create the operator namespace")
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", operatorNamespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Create the subscription")
	operatorSub.createCustomSub(oc)

	e2e.Logf("Create the operatorgroup")
	operatorOG.createOperatorGroup(oc)

	e2e.Logf("Wait for the operator pod running")
	if ok := waitForAvailableRsRunning(oc, "deploy", operatorDeoloy, operatorNamespace, operatorunningNum); ok {
		e2e.Logf("installed operator runnnig now\n")
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", operatorNamespace, "sub", "-o", "yaml").Execute()
		if err != nil {
			e2e.Logf("Could not retreive subscription")
		} else {
			err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", operatorNamespace, "sub").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", operatorNamespace, "csv").Execute()
		oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", operatorNamespace, "csv", "-o", "yaml").Execute()
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", operatorNamespace).Execute()
		oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", operatorNamespace).Execute()
		e2e.Failf("All pods related to deployment are not running %s", operatorDeoloy)
	}
}

func validateConfigmapAndSignatureContent(oc *exutil.CLI, dirname string, filePrefix string) {
	var signatureContent = "None"

	// Check if signature configmap yaml and json files exists in the clusterresources folder directory
	compat_otp.By("Verify that there is a yaml and json signature configmap files present under the clusterresources folder")
	signatureConfigmapOutput, err := exec.Command("bash", "-c", fmt.Sprintf("ls -l %s", dirname+"/working-dir/cluster-resources")).Output()
	if err != nil {
		e2e.Failf("Error is %v", err)
	}
	if !strings.Contains(string(signatureConfigmapOutput), "signature-configmap.yaml") && strings.Contains(string(signatureConfigmapOutput), "signature-configmap.json") {
		e2e.Failf("Could not find signature configmap files in the output, actual output is %s", signatureConfigmapOutput)
	}

	// Check if file with ocp_version-arch-imagedigestid exists in the signatures folder
	compat_otp.By("Verify that there is a file inside working-dir/signatures folder")
	signaturesFileOutput, err := exec.Command("bash", "-c", fmt.Sprintf("ls -l %s", dirname+"/working-dir/signatures/")).Output()
	if err != nil {
		e2e.Failf("Error %s occured while trying to retreive signatures folder", signaturesFileOutput)
	}
	e2e.Logf("Displaying content from signatures directory %s", signaturesFileOutput)
	if !strings.Contains(string(signaturesFileOutput), "4.16.0") {
		e2e.Failf("Could not find any files starting with string 4.16.0 in the signatures folder")
	}

	compat_otp.By("Get content from signatures folder")
	err = filepath.Walk(dirname+"/working-dir/signatures/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			e2e.Failf("Err is %s", err)
		}
		// Check if the file starts with the given prefix
		if !info.IsDir() && strings.HasPrefix(info.Name(), filePrefix) {
			e2e.Logf("Found file: %s\n", path)

			// Read the file content
			fileContent, err := ioutil.ReadFile(path)
			if err != nil {
				e2e.Logf("Error reading file %s: %v", path, err)
			}

			signatureContent = base64.StdEncoding.EncodeToString(fileContent)

			e2e.Logf("Content of %s:\n%s\n", path, signatureContent)
		}
		return nil
	})
	if err != nil {
		e2e.Logf("Error walking the directory: %v", err)
	}

	compat_otp.By("Retreive content from signature configmap yaml and json files")
	signatureConfigmapContent, err := exec.Command("bash", "-c", fmt.Sprintf("cat %s | grep 'binaryData:' -A 1 | tail -n 1 | sed 's/^[[:space:]]*//'", dirname+"/working-dir/cluster-resources/signature-configmap.yaml")).Output()
	configmapYamlContent := strings.Split(string(signatureConfigmapContent), ":")[1]
	if err != nil {
		e2e.Failf("could retreive content from signature-configmap.yaml")
	}
	if !(strings.TrimSpace(configmapYamlContent) == strings.TrimSpace(signatureContent)) {
		e2e.Failf("Content from signatures directory %s and the configmap are not the same %s", signatureContent, configmapYamlContent)
	}

	signatureConfigmapContentone, err := exec.Command("bash", "-c", fmt.Sprintf("cat %s | jq -r '.binaryData' | sed 's/[{}]//g; s/\"//g'", dirname+"/working-dir/cluster-resources/signature-configmap.json")).Output()
	configmapJsonContent := strings.Split(string(signatureConfigmapContentone), ":")[1]
	if err != nil {
		e2e.Failf("could retreive content from signature-configmap.json")
	}
	if !(strings.TrimSpace(configmapJsonContent) == strings.TrimSpace(signatureContent)) {
		e2e.Failf("Content from signatures directory %s and the configmap json are not the same %s", signatureContent, configmapJsonContent)
	}
}

func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, `-ojsonpath={.items[*].status.conditions[?(@.type=="Ready")].status}`).Output()
		e2e.Logf("the Ready status of pod is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get pod status: %v, retrying...", err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("the pod Ready status not met; wanted True but got %v, retrying...", status)
			return false, nil
		}
		return true, nil
	})
}

type AuthEntry struct {
	Auth string `json:"auth"`
}
type AuthsData struct {
	Auths map[string]AuthEntry `json:"auths"`
}

// Helper function to load and parse the auth file.
// It gracefully handles the file not existing.

func waitForPodContainerWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, `-ojsonpath={.items[*].status.conditions[?(@.type=="ContainersReady")].status}`).Output()
		e2e.Logf("the ContainersReady status of pod is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get pod status: %v, retrying...", err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("the pod container Ready status not met; wanted True but got %v, retrying...", status)
			return false, nil
		}
		return true, nil
	})
}

type operatorgroup struct {
	name      string
	namespace string
	template  string
}

type customsub struct {
	name        string
	namespace   string
	channelName string
	opsrcName   string
	sourceName  string
	startingCSV string
	template    string
}

func (og *operatorgroup) createOperatorGroup(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("og %s is not created successfully", og.name))
}

func (og *operatorgroup) deleteOperatorGroup(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("operatorgroup", og.name, "-n", og.namespace).Execute()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("og %s is not deleted successfully", og.name))
}

func (sub *customsub) createCustomSub(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace,
			"CHANNELNAME="+sub.channelName, "OPSRCNAME="+sub.opsrcName, "SOURCENAME="+sub.sourceName, "CSVNAME="+sub.startingCSV)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("sub %s is not created successfully", sub.name))
}

func (sub *customsub) deleteCustomSubscription(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("subscription", sub.name, "-n", sub.namespace).Execute()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("sub %s is not deleted successfully", sub.name))
}

func waitForAvailableRsRunning(oc *exutil.CLI, rsKind string, rsName string, namespace string, expected string) bool {
	err := wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(rsKind, rsName, "-n", namespace, "-o=jsonpath={.status.availableReplicas}").Output()
		if err != nil {
			e2e.Logf("object is still inprogress, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString(expected, output); matched {
			e2e.Logf("object is up:\n%s", output)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return false
	}
	return true
}
