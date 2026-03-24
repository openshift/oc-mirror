package workloads

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type operatorgroup struct {
	name      string
	namespace string
	template  string
}

type subscription struct {
	name        string
	namespace   string
	channelName string
	opsrcName   string
	sourceName  string
	startingCSV string
	template    string
}

type kubedescheduler struct {
	namespace        string
	interSeconds     int
	imageInfo        string
	logLevel         string
	operatorLogLevel string
	profile1         string
	profile2         string
	profile3         string
	template         string
}

type deploynodeaffinity struct {
	dName          string
	namespace      string
	replicaNum     int
	labelKey       string
	labelValue     string
	affinityKey    string
	operatorPolicy string
	affinityValue1 string
	affinityValue2 string
	template       string
}

type deploynodetaint struct {
	dName     string
	namespace string
	template  string
}

type deployinterpodantiaffinity struct {
	dName            string
	namespace        string
	replicaNum       int
	podAffinityKey   string
	operatorPolicy   string
	podAffinityValue string
	template         string
}

type deployduplicatepods struct {
	dName      string
	namespace  string
	replicaNum int
	template   string
}

type deploypodtopologyspread struct {
	dName     string
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

func (sub *subscription) createSubscription(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace,
			"CHANNELNAME="+sub.channelName, "OPSRCNAME="+sub.opsrcName, "SOURCENAME="+sub.sourceName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("sub %s is not created successfully", sub.name))
}

func (sub *subscription) deleteSubscription(oc *exutil.CLI) {
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

func (dsch *kubedescheduler) createKubeDescheduler(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", dsch.template, "-p", "NAMESPACE="+dsch.namespace, "INTERSECONDS="+strconv.Itoa(dsch.interSeconds),
			"IMAGEINFO="+dsch.imageInfo, "LOGLEVEL="+dsch.logLevel, "OPERATORLOGLEVEL="+dsch.operatorLogLevel,
			"PROFILE1="+dsch.profile1, "PROFILE2="+dsch.profile2, "PROFILE3="+dsch.profile3)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("dsch with image %s is not created successfully", dsch.imageInfo))
}

func (dsch *kubedescheduler) createKubeDeschedulerExpectError(oc *exutil.CLI, expectedErrPattern string) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", dsch.template, "-p", "NAMESPACE="+dsch.namespace, "INTERSECONDS="+strconv.Itoa(dsch.interSeconds),
		"IMAGEINFO="+dsch.imageInfo, "LOGLEVEL="+dsch.logLevel, "OPERATORLOGLEVEL="+dsch.operatorLogLevel,
		"PROFILE1="+dsch.profile1, "PROFILE2="+dsch.profile2, "PROFILE3="+dsch.profile3)
	o.Expect(err).To(o.HaveOccurred(), "Expected KubeDescheduler creation to fail but it succeeded")
	matched, _ := regexp.MatchString(expectedErrPattern, fmt.Sprintf("%v", err))
	o.Expect(matched).To(o.BeTrue(), fmt.Sprintf("Expected error to match pattern %q, but got: %v", expectedErrPattern, err))
	e2e.Logf("KubeDescheduler creation failed as expected with error: %v", err)
}

func checkEvents(oc *exutil.CLI, projectname string, strategyname string, expected string) {
	err := wait.Poll(5*time.Second, 100*time.Second, func() (bool, error) {
		output, err := oc.WithoutNamespace().Run("get").Args("events", "-n", projectname).Output()
		if err != nil {
			e2e.Logf("Can't get events from test project, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString("pod evicted by.*NodeAffinity", output); matched {
			e2e.Logf("Check the %s Strategy succeed:\n", strategyname)
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Check the %s Strategy not succeed", strategyname))
}

func checkAvailable(oc *exutil.CLI, rsKind string, rsName string, namespace string, expected string) {
	err := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(rsKind, rsName, "-n", namespace, "-o=jsonpath={.status.availableReplicas}").Output()
		if err != nil {
			e2e.Logf("deploy is still inprogress, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString(expected, output); matched {
			e2e.Logf("deploy is up:\n%s", output)
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("%s is not expected", expected))
}

func getImageFromCSV(oc *exutil.CLI, namespace string) string {
	var csvalm interface{}
	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", namespace, "-o=jsonpath={.items[0].metadata.annotations.alm-examples}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	out = strings.TrimLeft(out, "[")
	out = strings.TrimRight(out, "]")
	if err := json.Unmarshal([]byte(out), &csvalm); err != nil {
		e2e.Logf("unable to decode version with error: %v", err)
	}
	amlOject := csvalm.(map[string]interface{})
	imageInfo := amlOject["spec"].(map[string]interface{})["image"].(string)
	return imageInfo
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

func checkPodsStatusByLabel(oc *exutil.CLI, namespace string, labels string, expectedstatus string) bool {
	out, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", labels, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podsList := strings.Fields(out)
	for _, pod := range podsList {
		podstatus, err := oc.WithoutNamespace().Run("get").Args("pod", pod, "-n", namespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(expectedstatus, podstatus); !matched {
			e2e.Logf("%s is not with status:\n%s", pod, expectedstatus)
			return false
		}
	}
	return true
}

func createResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.WithoutNamespace().Run("process").Args(parameters...).OutputToFile(getRandomString() + "workload-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().Run("create").Args("-f", configFile).Execute()
}

func checkLogsFromRs(oc *exutil.CLI, projectname string, rsKind string, rsName string, expected string) {
	err := wait.Poll(60*time.Second, 540*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(rsKind+`/`+rsName, "-n", projectname).Output()
		if err != nil {
			e2e.Logf("Can't get logs from test project, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.Match(expected, []byte(output)); !matched {
			e2e.Logf("Can't find the expected string\n")
			return false, nil
		}
		e2e.Logf("Check the logs succeed!!\n")
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("%s is not expected for %s", expected, rsName))
}

func (deploy *deploynodeaffinity) createDeployNodeAffinity(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploy.template, "-p", "DNAME="+deploy.dName, "NAMESPACE="+deploy.namespace,
			"REPLICASNUM="+strconv.Itoa(deploy.replicaNum), "LABELKEY="+deploy.labelKey, "LABELVALUE="+deploy.labelValue, "AFFINITYKEY="+deploy.affinityKey,
			"OPERATORPOLICY="+deploy.operatorPolicy, "AFFINITYVALUE1="+deploy.affinityValue1, "AFFINITYVALUE2="+deploy.affinityValue2)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deploy.dName))
}

func (deployn *deploynodetaint) createDeployNodeTaint(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deployn.template, "-p", "DNAME="+deployn.dName, "NAMESPACE="+deployn.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deployn.dName))
}

func (deployp *deployinterpodantiaffinity) createDeployInterPodAntiAffinity(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deployp.template, "-p", "DNAME="+deployp.dName, "NAMESPACE="+deployp.namespace,
			"REPLICASNUM="+strconv.Itoa(deployp.replicaNum), "PODAFFINITYKEY="+deployp.podAffinityKey,
			"OPERATORPOLICY="+deployp.operatorPolicy, "PODAFFINITYVALUE="+deployp.podAffinityValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deployp.dName))
}

func (deploydp *deployduplicatepods) createDuplicatePods(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploydp.template, "-p", "DNAME="+deploydp.dName, "NAMESPACE="+deploydp.namespace,
			"REPLICASNUM="+strconv.Itoa(deploydp.replicaNum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deploydp.dName))
}

func (deploypts *deploypodtopologyspread) createPodTopologySpread(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploypts.template, "-p", "DNAME="+deploypts.dName, "NAMESPACE="+deploypts.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create %v", deploypts.dName))
}

func checkDeschedulerMetrics(oc *exutil.CLI, strategyname string, metricName string, podName string) {
	olmToken, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	endpointIP, _, epGetErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-kube-descheduler-operator", "endpoints", "metrics", "-o", fmt.Sprintf(`jsonpath={.subsets[*].addresses[*].ip}`)).Outputs()
	o.Expect(epGetErr).NotTo(o.HaveOccurred())
	var metricsUrl = fmt.Sprintf(`https://%v:10258/metrics`, string(endpointIP))
	//Add code to check if ip address is ipv6 and add braces around it
	endpointAddress := net.ParseIP(endpointIP)
	if endpointAddress.To4() == nil {
		metricsUrl = fmt.Sprintf(`https://[%v]:10258/metrics`, string(endpointIP))
	}
	err = wait.Poll(6*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-kube-descheduler-operator", podName, "-c", "openshift-descheduler", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), metricsUrl).Output()
		if err != nil {
			e2e.Logf("Can't get descheduler metrics, error: %s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString(strategyname, output); matched {
			e2e.Logf("Check the %s Strategy succeed\n", strategyname)
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get metric %s via metrics endpoint of descheduler", strategyname))
}

func (sub *subscription) skipMissingCatalogsources(oc *exutil.CLI) {
	output, errQeReg := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "qe-app-registry").Output()
	if errQeReg != nil && strings.Contains(output, "NotFound") {
		output, errRed := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", "redhat-operators").Output()
		if errRed != nil && strings.Contains(output, "NotFound") {
			g.Skip("Skip since catalogsources not available")
		} else {
			o.Expect(errRed).NotTo(o.HaveOccurred())
		}
		sub.opsrcName = "redhat-operators"
	} else if errQeReg != nil && strings.Contains(output, "doesn't have a resource type \"catalogsource\"") {
		g.Skip("Skip since catalogsource is not available")
	} else {
		o.Expect(errQeReg).NotTo(o.HaveOccurred())
	}
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

func createDeschedulerNs(oc *exutil.CLI, kubeNamespace string) {
	nsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", kubeNamespace).Execute()
	if nsErr != nil {
		e2e.Logf("Kube-descheduler-operator namespace does not exist, proceeding further")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", kubeNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		patch := `[{"op":"add", "path":"/metadata/labels/openshift.io~1cluster-monitoring", "value":"true"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ns", kubeNamespace, "--type=json", "-p", patch).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		e2e.Logf("Kube-descheduler-operator namespace exist, no other action")
	}
}

func packagemanifest_kdo(oc *exutil.CLI, packageName string, namespace string, catalogNames []string) subscription {
	e2e.Logf("Fetching packagemanifest values dynamically")

	var catalogSource, catalogSourceNamespace, defaultChannel, currentCSV, foundCatalog string
	var err error

	// Loop through each catalog to find the package
	for _, catalogName := range catalogNames {
		e2e.Logf("Checking catalog: %s", catalogName)
		catalogSource, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", "-n", namespace, "-l", "catalog="+catalogName, "-o=jsonpath={.items[?(@.metadata.name==\""+packageName+"\")].status.catalogSource}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if catalogSource != "" {
			foundCatalog = catalogName
			e2e.Logf("Package %s found in catalog: %s", packageName, catalogName)
			break
		}
	}

	// If not found in any catalog, skip the test
	if catalogSource == "" {
		g.Skip("Skip test: package " + packageName + " not found in any of the specified catalogs: " + strings.Join(catalogNames, ", "))
	}

	e2e.Logf("Catalog Source (opsrcName): %v", catalogSource)

	// Get catalogSourceNamespace (e.g., "openshift-marketplace") using list + filter approach
	catalogSourceNamespace, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", "-n", namespace, "-l", "catalog="+foundCatalog, "-o=jsonpath={.items[?(@.metadata.name==\""+packageName+"\")].status.catalogSourceNamespace}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Catalog Source Namespace (sourceName): %v", catalogSourceNamespace)

	// Get defaultChannel (e.g., "stable") using list + filter approach
	defaultChannel, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", "-n", namespace, "-l", "catalog="+foundCatalog, "-o=jsonpath={.items[?(@.metadata.name==\""+packageName+"\")].status.defaultChannel}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Default Channel (channelName): %v", defaultChannel)

	// Get currentCSV from the default channel using list + filter approach
	currentCSV, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", "-n", namespace, "-l", "catalog="+foundCatalog, "-o=jsonpath={.items[?(@.metadata.name==\""+packageName+"\")].status.channels[?(@.name==\""+defaultChannel+"\")].currentCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Current CSV (startingCSV): %v", currentCSV)

	// Return populated subscription struct with dynamically fetched values
	return subscription{
		name:        packageName,
		namespace:   "openshift-kube-descheduler-operator",
		channelName: defaultChannel,
		opsrcName:   catalogSource,
		sourceName:  catalogSourceNamespace,
		startingCSV: currentCSV,
	}
}
