package workloads

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"regexp"

	o "github.com/onsi/gomega"

	"math/rand"
	"net/http"

	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type podNodeSelector struct {
	name       string
	namespace  string
	labelKey   string
	labelValue string
	nodeKey    string
	nodeValue  string
	template   string
}

type podSinglePts struct {
	name       string
	namespace  string
	labelKey   string
	labelValue string
	ptsKeyName string
	ptsPolicy  string
	skewNum    int
	template   string
}

type podSinglePtsNodeSelector struct {
	name       string
	namespace  string
	labelKey   string
	labelValue string
	ptsKeyName string
	ptsPolicy  string
	skewNum    int
	nodeKey    string
	nodeValue  string
	template   string
}

type deploySinglePts struct {
	dName      string
	namespace  string
	replicaNum int
	labelKey   string
	labelValue string
	ptsKeyName string
	ptsPolicy  string
	skewNum    int
	template   string
}

type deployNodeSelector struct {
	dName      string
	namespace  string
	replicaNum int
	labelKey   string
	labelValue string
	nodeKey    string
	nodeValue  string
	template   string
}

type podAffinityRequiredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	affinityMethod string
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podAffinityPreferredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	affinityMethod string
	weigthNum      int
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podNodeAffinityRequiredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	ptsKey2Name    string
	ptsPolicy2     string
	skewNum2       int
	affinityMethod string
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podSingleNodeAffinityRequiredPts struct {
	name           string
	namespace      string
	labelKey       string
	labelValue     string
	ptsKeyName     string
	ptsPolicy      string
	skewNum        int
	affinityMethod string
	keyName        string
	valueName      string
	operatorName   string
	template       string
}

type podTolerate struct {
	namespace      string
	keyName        string
	operatorPolicy string
	valueName      string
	effectPolicy   string
	tolerateTime   int
	template       string
}

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

type podMirror struct {
	name            string
	namespace       string
	cliImageID      string
	imagePullSecret string
	imageSource     string
	imageTo         string
	imageToRelease  string
	template        string
}

type debugPodUsingDefinition struct {
	name       string
	namespace  string
	cliImageID string
	template   string
}

type priorityClassDefinition struct {
	name          string
	priorityValue int
	template      string
}

type priorityPod struct {
	dName      string
	namespace  string
	replicaSum int
	template   string
}

type cronJobCreationTZ struct {
	cName     string
	namespace string
	schedule  string
	timeZone  string
	template  string
}

type priorityPodDefinition struct {
	name              string
	label             string
	memory            int
	priorityClassName string
	namespace         string
	template          string
}

func (pod *podNodeSelector) createPodNodeSelector(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"NODEKEY="+pod.nodeKey, "NODEVALUE="+pod.nodeValue, "LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSinglePts) createPodSinglePts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSinglePtsNodeSelector) createPodSinglePtsNodeSelector(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum),
			"NODEKEY="+pod.nodeKey, "NODEVALUE="+pod.nodeValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (deploy *deploySinglePts) createDeploySinglePts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", deploy.template, "-p", "DNAME="+deploy.dName, "NAMESPACE="+deploy.namespace,
			"REPLICASNUM="+strconv.Itoa(deploy.replicaNum), "LABELKEY="+deploy.labelKey, "LABELVALUE="+deploy.labelValue, "PTSKEYNAME="+deploy.ptsKeyName,
			"PTSPOLICY="+deploy.ptsPolicy, "SKEWNUM="+strconv.Itoa(deploy.skewNum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("deploy %s with %s is not created successfully", deploy.dName, deploy.labelKey))
}

func (pod *podAffinityRequiredPts) createPodAffinityRequiredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum),
			"AFFINITYMETHOD="+pod.affinityMethod, "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podAffinityPreferredPts) createPodAffinityPreferredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace,
			"LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum),
			"AFFINITYMETHOD="+pod.affinityMethod, "WEIGHTNUM="+strconv.Itoa(pod.weigthNum), "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSinglePts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podNodeSelector) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podSinglePtsNodeSelector) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podAffinityRequiredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podAffinityPreferredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
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

func applyResourceFromTemplate48681(oc *exutil.CLI, parameters ...string) (string, error) {
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
	return configFile, oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func describePod(oc *exutil.CLI, namespace string, podName string) string {
	podDescribe, err := oc.WithoutNamespace().Run("describe").Args("pod", "-n", namespace, podName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podDescribe)
	return podDescribe
}

func getPodStatus(oc *exutil.CLI, namespace string, podName string) string {
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podStatus)
	return podStatus
}

func getPodNodeListByLabel(oc *exutil.CLI, namespace string, labelKey string) []string {
	output, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", labelKey, "-o=jsonpath={.items[*].spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNameList := strings.Fields(output)
	return nodeNameList
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

func (pod *podNodeAffinityRequiredPts) createpodNodeAffinityRequiredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum), "PTSKEY2NAME="+pod.ptsKey2Name, "PTSPOLICY2="+pod.ptsPolicy2, "SKEWNUM2="+strconv.Itoa(pod.skewNum2), "AFFINITYMETHOD="+pod.affinityMethod, "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podNodeAffinityRequiredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podSingleNodeAffinityRequiredPts) createpodSingleNodeAffinityRequiredPts(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "LABELKEY="+pod.labelKey, "LABELVALUE="+pod.labelValue, "PTSKEYNAME="+pod.ptsKeyName, "PTSPOLICY="+pod.ptsPolicy, "SKEWNUM="+strconv.Itoa(pod.skewNum), "AFFINITYMETHOD="+pod.affinityMethod, "KEYNAME="+pod.keyName, "VALUENAME="+pod.valueName, "OPERATORNAME="+pod.operatorName)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.labelKey))
}

func (pod *podSingleNodeAffinityRequiredPts) getPodNodeName(oc *exutil.CLI) string {
	nodeName, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", pod.namespace, pod.name, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", pod.name, nodeName)
	return nodeName
}

func (pod *podTolerate) createPodTolerate(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAMESPACE="+pod.namespace, "KEYNAME="+pod.keyName,
			"OPERATORPOLICY="+pod.operatorPolicy, "VALUENAME="+pod.valueName, "EFFECTPOLICY="+pod.effectPolicy, "TOLERATETIME="+strconv.Itoa(pod.tolerateTime))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s is not created successfully", pod.keyName))
}

func getPodNodeName(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s lands on node %q", podName, nodeName)
	return nodeName
}

func createLdapService(oc *exutil.CLI, namespace string, podName string, initGroup string) {
	err := oc.AsAdmin().WithoutNamespace().Run("run").Args(podName, "--image", "quay.io/openshifttest/ldap@sha256:2700c5252cc72e12b845fe97ed659b8178db5ac72e13116b617de431c7826600", "-n", namespace).Execute()
	if err != nil {
		oc.Run("delete").Args("pod/ldapserver", "-n", namespace).Execute()
		e2e.Failf("failed to run the ldap pod")
	}
	checkPodStatus(oc, "run="+podName, namespace, "Running")
	err = oc.Run("cp").Args("-n", namespace, initGroup, podName+":/tmp/").Execute()
	if err != nil {
		oc.Run("delete").Args("pod/ldapserver", "-n", oc.Namespace()).Execute()
		e2e.Failf("failed to copy the init group to ldap server")
	}
	err = oc.Run("exec").Args(podName, "-n", namespace, "--", "ldapadd", "-x", "-h", "[::1]", "-p", "389", "-D", "cn=Manager,dc=example,dc=com", "-w", "admin", "-f", "/tmp/init.ldif").Execute()
	if err != nil {
		oc.Run("delete").Args("pod/ldapserver", "-n", namespace).Execute()
		e2e.Failf("failed to config the ldap server ")
	}

}

func getSyncGroup(oc *exutil.CLI, syncConfig string) string {
	var groupFile string
	err := wait.Poll(5*time.Second, 200*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("adm").Args("groups", "sync", "--sync-config="+syncConfig).OutputToFile(getRandomString() + "workload-group.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		groupFile = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "adm groups sync fails")
	if strings.Compare(groupFile, "") == 0 {
		e2e.Failf("Failed to get group infomation!")
	}
	return groupFile
}

func getLeaderKCM(oc *exutil.CLI) string {
	var leaderKCM string
	e2e.Logf("Get the control-plane from configmap")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lease/kube-controller-manager", "-n", "kube-system", "-o=jsonpath={.spec.holderIdentity}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	leaderIP := strings.Split(output, "_")[0]

	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master=", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	masterList := strings.Fields(out)
	for _, masterNode := range masterList {
		if matched, _ := regexp.MatchString(leaderIP, masterNode); matched {
			e2e.Logf("Find the leader of KCM :%s\n", masterNode)
			leaderKCM = masterNode
			break
		}
	}
	return leaderKCM
}

func removeDuplicateElement(elements []string) []string {
	result := make([]string, 0, len(elements))
	temp := map[string]struct{}{}
	for _, item := range elements {
		if _, ok := temp[item]; !ok { //if can't find the item，ok=false，!ok is true，then append item。
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
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

func (pod *podMirror) createPodMirror(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "CLIIMAGEID="+pod.cliImageID, "IMAGEPULLSECRET="+pod.imagePullSecret, "IMAGESOURCE="+pod.imageSource, "IMAGETO="+pod.imageTo, "IMAGETORELEASE="+pod.imageToRelease)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with %s is not created successfully", pod.name, pod.cliImageID))
}

func createPullSecret(oc *exutil.CLI, namespace string) {
	err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to=/tmp", "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", "my-secret", "--from-file="+"/tmp/.dockerconfigjson", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getCliImage(oc *exutil.CLI) string {
	cliImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("imagestreams", "cli", "-n", "openshift", "-o=jsonpath={.spec.tags[0].from.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return cliImage
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

func checkMustgatherPodNode(oc *exutil.CLI) {
	var nodeNameList []string
	e2e.Logf("Get the node list of the must-gather pods running on")
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "app=must-gather", "-A", "-o=jsonpath={.items[*].spec.nodeName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeNameList = strings.Fields(output)
		if nodeNameList == nil {
			e2e.Logf("Can't find must-gather pod now, and try next round")
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("must-gather pod is not created successfully"))
	e2e.Logf("must-gather scheduled on: %v", nodeNameList)

	e2e.Logf("make sure all the nodes in nodeNameList are not windows node")
	expectedNodeLabels := getScanNodesLabels(oc, nodeNameList, "windows")
	if expectedNodeLabels == nil {
		e2e.Logf("must-gather scheduled as expected, no windows node found in the cluster")
	} else {
		e2e.Failf("Scheduled the must-gather pod to windows node: %v", expectedNodeLabels)
	}
}

func (pod *debugPodUsingDefinition) createDebugPodUsingDefinition(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		outputFile, err1 := applyResourceFromTemplate48681(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "CLIIMAGEID="+pod.cliImageID)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		e2e.Logf("Waiting for pod running")
		err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
			phase, err := oc.AsAdmin().Run("get").Args("pods", pod.name, "--template", "{{.status.phase}}", "-n", pod.namespace).Output()
			if err != nil {
				return false, nil
			}
			if phase != "Running" {
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			e2e.Logf("Error waiting for pod to be in 'Running' phase: %v", err)
			return false, nil
		}

		debugPod, err := oc.Run("debug").Args("-f", outputFile).Output()
		if err != nil {
			e2e.Logf("Error running 'debug' command: %v", err)
			return false, nil
		}
		if match, _ := regexp.MatchString("Starting pod/pod48681-debug", debugPod); !match {
			e2e.Failf("Image debug container is being started instead of debug pod using the pod definition yaml file")
		}
		return true, nil
	})
	if err != nil {
		e2e.Failf("Error creating debug pod: %v", err)
	}
}

func createDeployment(oc *exutil.CLI, namespace string, deployname string) {
	err := oc.Run("create").Args("-n", namespace, "deployment", deployname, "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--replicas=20").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func triggerSucceedDeployment(oc *exutil.CLI, namespace string, deployname string, num int, expectedPods int) {
	var generation string
	var getGenerationerr error
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		generation, getGenerationerr = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", deployname, "-n", namespace, "-o=jsonpath={.status.observedGeneration}").Output()
		if getGenerationerr != nil {
			e2e.Logf("Err Occurred, try again: %v", getGenerationerr)
			return false, nil
		}
		if generation == "" {
			e2e.Logf("Can't get generation, try again: %v", generation)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to get  generation "))

	generationNum, err := strconv.Atoi(generation)
	o.Expect(err).NotTo(o.HaveOccurred())
	for i := 0; i < num; i++ {
		generationNum++
		err := oc.Run("set").Args("-n", namespace, "env", "deployment", deployname, "paramtest=test"+strconv.Itoa(i)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, currentRsName := getCurrentRs(oc, namespace, "app="+deployname, generationNum)
		err = wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
			availablePodNum, errGet := oc.Run("get").Args("-n", namespace, "rs", currentRsName, "-o=jsonpath='{.status.availableReplicas}'").Output()
			if errGet != nil {
				e2e.Logf("Err Occurred: %v", errGet)
				return false, errGet
			}
			availableNum, _ := strconv.Atoi(strings.ReplaceAll(availablePodNum, "'", ""))
			if availableNum != expectedPods {
				e2e.Logf("new triggered apps not deploy successfully, wait more times")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("failed to deploy %v", deployname))

	}
}
func triggerFailedDeployment(oc *exutil.CLI, namespace string, deployname string) {
	patchYaml := `[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "quay.io/openshifttest/hello-openshift:nonexist"}]`
	err := oc.Run("patch").Args("-n", namespace, "deployment", deployname, "--type=json", "-p", patchYaml).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
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

func getShouldPruneRSFromPrune(oc *exutil.CLI, pruneRsNumCMD string, pruneRsCMD string, prunedNum int) []string {
	e2e.Logf("Get pruned rs name by dry-run")
	e2e.Logf("pruneRsNumCMD %v:", pruneRsNumCMD)
	err := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		pruneRsNum, err := exec.Command("bash", "-c", pruneRsNumCMD).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pruneNum, err := strconv.Atoi(strings.ReplaceAll(string(pruneRsNum), "\n", ""))
		o.Expect(err).NotTo(o.HaveOccurred())
		if pruneNum != prunedNum {
			e2e.Logf("pruneNum is not equal %v: ", prunedNum)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Check pruned RS failed"))

	e2e.Logf("pruneRsCMD %v:", pruneRsCMD)
	pruneRsName, err := exec.Command("bash", "-c", pruneRsCMD).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	pruneRsList := strings.Fields(strings.ReplaceAll(string(pruneRsName), "\n", " "))
	sort.Strings(pruneRsList)
	e2e.Logf("pruneRsList %v:", pruneRsList)
	return pruneRsList
}

func getCompeletedRsInfo(oc *exutil.CLI, namespace string, deployname string) (completedRsList []string, completedRsNum int) {
	out, err := oc.Run("get").Args("-n", namespace, "rs", "--sort-by={.metadata.creationTimestamp}", "-o=jsonpath='{.items[?(@.spec.replicas == 0)].metadata.name}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("string out %v:", out)
	totalCompletedRsList := strings.Fields(strings.ReplaceAll(out, "'", ""))
	totalCompletedRsListNum := len(totalCompletedRsList)
	return totalCompletedRsList, totalCompletedRsListNum
}

func getShouldPruneRSFromCreateTime(totalCompletedRsList []string, totalCompletedRsListNum int, keepNum int) []string {
	rsList := totalCompletedRsList[0:(totalCompletedRsListNum - keepNum)]
	sort.Strings(rsList)
	e2e.Logf("rsList %v:", rsList)
	return rsList

}

func comparePrunedRS(rsList []string, pruneRsList []string) bool {
	e2e.Logf("Check pruned rs whether right")
	if !reflect.DeepEqual(rsList, pruneRsList) {
		return false
	}
	return true
}

func checkRunningRsList(oc *exutil.CLI, namespace string, deployname string) []string {
	e2e.Logf("Get all the running RSs")
	out, err := oc.Run("get").Args("-n", namespace, "rs", "-o=jsonpath='{.items[?(@.spec.replicas > 0)].metadata.name}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	runningRsList := strings.Fields(strings.ReplaceAll(out, "'", ""))
	sort.Strings(runningRsList)
	e2e.Logf("runningRsList %v:", runningRsList)
	return runningRsList
}

func pruneCompletedRs(oc *exutil.CLI, parameters ...string) {
	e2e.Logf("Delete all the completed RSs")
	err := oc.AsAdmin().WithoutNamespace().Run("adm").Args(parameters...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getRemainingRs(oc *exutil.CLI, namespace string, deployname string) []string {
	e2e.Logf("Get all the remaining RSs")
	remainRs, err := oc.WithoutNamespace().Run("get").Args("rs", "-l", "app="+deployname, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	remainRsList := strings.Fields(string(remainRs))
	sort.Strings(remainRsList)
	e2e.Logf("remainRsList %v:", remainRsList)
	return remainRsList
}

func getCurrentRs(oc *exutil.CLI, projectName string, labels string, generationNum int) (string, string) {
	var podTHash, rsName string
	e2e.Logf("Print the deploy current generation %v", generationNum)
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		generationC, err1 := oc.Run("get").Args("deploy", "-n", projectName, "-l", labels, "-o=jsonpath={.items[*].status.observedGeneration}").Output()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		if matched, _ := regexp.MatchString(strconv.Itoa(generationNum), generationC); !matched {
			e2e.Logf("the generation is not expected, and try next round")
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "The deploy generation is failed to update")
	err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		rsName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("rs", "-n", projectName, "-l", labels, fmt.Sprintf(`-o=jsonpath={.items[?(@.metadata.annotations.deployment\.kubernetes\.io/revision=='%s')].metadata.name}`, strconv.Itoa(generationNum))).Output()
		e2e.Logf("Print the deploy current rs is %v", rsName)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "Failed to get the current rs for deploy")
	podTHash, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("rs", rsName, "-n", projectName, "-o=jsonpath={.spec.selector.matchLabels.pod-template-hash}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podTHash, rsName
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

func getPodCred(oc *exutil.CLI, dst string) string {
	key := "XDG_RUNTIME_DIR"
	currentRuntime, ex := os.LookupEnv(key)
	authFile := currentRuntime + "/containers/auth.json"
	if !ex {
		os.Setenv(key, "/tmp/configocmirror")
		authFile = "/tmp/configocmirror/containers/auth.json"
	}
	_, err := os.Stat(authFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	return authFile
}

func checkPodStatus(oc *exutil.CLI, podLabel string, namespace string, expected string) {
	err := wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", podLabel, "-o=jsonpath={.items[*].status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the result of pod:%v", output)
		if strings.Contains(output, expected) && (!(strings.Contains(strings.ToLower(output), "error"))) && (!(strings.Contains(strings.ToLower(output), "crashLoopbackOff"))) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, "-l", podLabel, "-o", "yaml").Execute()
	}
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("the state of pod with %s is not expected %s", podLabel, expected))
}

func locateDockerCred(oc *exutil.CLI, dst string) (string, string, error) {
	err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dst, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	homePath := os.Getenv("HOME")
	dockerCreFile := homePath + "/.docker/config.json"
	_, err = os.Stat(homePath + "/.docker/config.json")
	if os.IsNotExist(err) {
		err1 := os.MkdirAll(homePath+"/.docker", 0700)
		o.Expect(err1).NotTo(o.HaveOccurred())
		copyFile(dst+"/"+".dockerconfigjson", homePath+"/.docker/config.json")
		return dockerCreFile, homePath, nil
	}
	if err != nil {
		return "", "", err
	}
	copyFile(homePath+"/.docker/config.json", homePath+"/.docker/config.json.back")
	copyFile(dst+"/"+".dockerconfigjson", homePath+"/.docker/config.json")
	return dockerCreFile, homePath, nil

}

func waitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	return wait.Poll(5*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		gottenStatus := getCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			eq := reflect.DeepEqual(expectedStatus, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
			if eq {
				// For True False False, we want to wait some bit more time and double check, to ensure it is stably healthy
				time.Sleep(100 * time.Second)
				gottenStatus := getCoStatus(oc, coName, expectedStatus)
				eq := reflect.DeepEqual(expectedStatus, gottenStatus)
				if eq {
					e2e.Logf("Given operator %s becomes available/non-progressing/non-degraded", coName)
					return true, nil
				}
			} else {
				e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
				return true, nil
			}
		}
		return false, nil
	})
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

func (pc *priorityClassDefinition) createPriorityClass(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pc.template, "-p", "NAME="+pc.name, "PRIORITYVALUE="+strconv.Itoa(pc.priorityValue))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("priorityClass %s has not been created successfully", pc.name))
}

func (pc *priorityClassDefinition) deletePriorityClass(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("priorityclass", pc.name).Execute()
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("priorityclass %s is not deleted successfully", pc.name))
}

func checkNetworkType(oc *exutil.CLI) string {
	output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.type}").Output()
	return strings.ToLower(output)
}

func checkDockerCred() bool {
	homePath := os.Getenv("HOME")
	_, err := os.Stat(homePath + "/.docker/config.json")
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func checkPodmanCred() bool {
	currentRuntime := os.Getenv("XDG_RUNTIME_DIR")
	_, err := os.Stat(currentRuntime + "containers/auth.json")
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func getPullSecret(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config", `--template={{index .data ".dockerconfigjson" | base64decode}}`).OutputToFile("auth.dockerconfigjson")
}

func getHostFromRoute(oc *exutil.CLI, routeName string, routeNamespace string) string {
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", routeNamespace, "-o", "jsonpath='{.spec.host}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	return stdout
}
func createEdgeRoute(oc *exutil.CLI, serviceName string, namespace string, routeName string) {
	err := oc.Run("create").Args("route", "edge", routeName, "--service", serviceName, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createDir(dirname string) {
	err := os.MkdirAll(dirname, 0755)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createSpecialRegistry(oc *exutil.CLI, namespace string, ssldir string, dockerConfig string) string {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("deploy", "mydauth", "-n", namespace, "--image=quay.io/openshifttest/registry-auth-server@sha256:f56cb68a6353a27dc08cef5d33a283875808def0e80fdda2e3c2d5ddb557c703", "--port=5001").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("deploy", "mydauth", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "passthrough", "r1", "--service=mydauth", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	hostD, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "r1", "-n", namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	caSubj := "/C=GB/CN=foo  -addext \"subjectAltName = DNS:" + hostD + "\""
	opensslCmd := fmt.Sprintf(`openssl req -x509 -nodes -days 3650 -newkey rsa:2048 -keyout  %s/server.key  -out  %s/server.pem -subj %s`, ssldir, ssldir, caSubj)
	e2e.Logf("opensslcmd is :%v", opensslCmd)
	_, err = exec.Command("bash", "-c", opensslCmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "dockerauthssl", "--from-file="+ssldir, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("volume", "deploy", "mydauth", "--add", "--name=v2", "--type=secret", "--secret-name=dockerauthssl", "--mount-path=/ssl", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "dockerautoconfig", "--from-file="+dockerConfig, "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("volume", "deploy", "mydauth", "--add", "--name=v1", "--type=secret", "--secret-name=dockerautoconfig", "--mount-path=/config", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Check the docker_auth pod should running")
	if ok := waitForAvailableRsRunning(oc, "deployment", "mydauth", namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "mydauth", "-o", "yaml", "-n", namespace).Execute()
		e2e.Failf("docker_auth pod is not running even afer waiting for about 3 minutes")
	}

	registryAuthToken := "https://" + hostD + "/auth"
	registryPara := fmt.Sprintf(`REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/tmp/registry REGISTRY_AUTH=token REGISTRY_AUTH_TOKEN_REALM=%s REGISTRY_AUTH_TOKEN_SERVICE="Docker registry" REGISTRY_AUTH_TOKEN_ISSUER="Acme auth server" REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE=/ssl/server.pem `, registryAuthToken)
	err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("--name=myregistry", fmt.Sprintf("%s", registryPara), "-n", namespace, "--image=quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3", "--import-mode=PreserveOriginal").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("volume", "deploy", "myregistry", "--add", "--name=v2", "--type=secret", "--secret-name=dockerauthssl", "--mount-path=/ssl", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "edge", "r2", "--service=myregistry", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	registryHost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "r2", "-n", namespace, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Check the registry pod should running")
	if ok := waitForAvailableRsRunning(oc, "deployment", "myregistry", namespace, "1"); ok {
		e2e.Logf("All pods are runnnig now\n")
	} else {
		_ = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "myregistry", "-o", "yaml", "-n", namespace).Execute()
		e2e.Failf("private registry pod is not running even afer waiting for about 3 minutes")
	}
	return registryHost
}

func checkNodeUncordoned(oc *exutil.CLI, workerNodeName string) error {
	return wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
		schedulableStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", workerNodeName, "-o=jsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Schedulable Status is %s\n", schedulableStatus)
		if !strings.Contains(schedulableStatus, "unschedulable") {
			e2e.Logf("\n WORKER NODE IS READY\n ")
		} else {
			e2e.Logf("\n WORKERNODE IS NOT READY\n ")
			return false, nil
		}
		return true, nil
	})
}

func (prio *priorityPod) createPodWithPriorityParam(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", prio.template, "-p", "NAMESPACE="+prio.namespace, "DNAME="+prio.dName,
			"REPLICASNUM="+strconv.Itoa(prio.replicaSum))
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("pod %s with priority has not been created successfully", prio.dName))
}
func nonAdminApplyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "workload-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func getDigestFromImageInfo(oc *exutil.CLI, registryRoute string) string {
	path := "/tmp/mirroredimageinfo.yaml"
	defer os.Remove(path)
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		imageInfo, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", registryRoute+"/openshift/release-images", "--insecure").Output()
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		e2e.Logf("the imageinfo is :%v", imageInfo)
		err1 := ioutil.WriteFile(path, []byte(imageInfo), 0o644)
		o.Expect(err1).NotTo(o.HaveOccurred())
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("failed to get the mirrored image info"))
	imageDigest, err := exec.Command("bash", "-c", "cat "+path+"|grep Digest | awk -F' ' '{print $2}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the imagedeigest is :%v, ", string(imageDigest))
	return strings.ReplaceAll(string(imageDigest), "\n", "")
}

func findImageContentSourcePolicy() string {
	imageContentSourcePolicyFile, err := exec.Command("bash", "-c", "find . -name 'imageContentSourcePolicy.yaml'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.ReplaceAll(string(imageContentSourcePolicyFile), "\n", "")
}

func removeOcMirrorLog() {
	os.RemoveAll("oc-mirror-workspace")
	os.RemoveAll(".oc-mirror.log")
}

func (cj *cronJobCreationTZ) createCronJobWithTimeZone(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cj.template, "-p", "CNAME="+cj.cName, "NAMESPACE="+cj.namespace,
			"SCHEDULE="+cj.schedule, "TIMEZONE="+cj.timeZone)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Cronjob with %s is not created successfully", cj.cName))
}

func getTimeFromTimezone(oc *exutil.CLI) (string, string) {
	// Get the local timezone
	localZone, err := time.LoadLocation("")
	e2e.Logf("localzone is %s", localZone)
	if err != nil {
		e2e.Failf("Could not get local timezone", err)
	}

	// Get the current time in the local timezone
	currentTime := time.Now().In(localZone)
	e2e.Logf("Local Timezone:", localZone, "Current Time:", currentTime.Format(time.RFC3339))

	// Caluclate the cron schedule based on the local timezone
	hour, minu, _ := currentTime.Clock()

	// Adjust the hour and minute components
	if minu >= 58 {
		hour = (hour + 1) % 24
		minu = (minu + 2) % 60
	} else {
		minu += 02
	}
	cronSchedule := fmt.Sprintf("%d %d * * *", minu, hour)
	return cronSchedule, localZone.String()
}

func getOauthAudit(mustgatherDir string) []string {
	var files []string
	filesUnderGather, err := ioutil.ReadDir(mustgatherDir)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to read the must-gather dir")
	dataDir := ""
	for _, fileD := range filesUnderGather {
		if fileD.IsDir() {
			dataDir = fileD.Name()
		}

	}
	e2e.Logf("The data dir is %v", dataDir)
	destDir := mustgatherDir + "/" + dataDir + "/audit_logs/oauth-server/"
	err = filepath.Walk(destDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			file_size := info.Size()
			// When the file_size is too little , the file maybe empty or too little records , so filter more than 1024
			if !info.IsDir() && file_size > 1024 {
				files = append(files, path)
			}
			return nil
		})
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to read the destDir")
	return files
}
func getLatestPayload(url string) string {
	res, err := http.Get(url)
	if err != nil {
		e2e.Failf("unable to get http with error: %v", err)
	}
	body, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		e2e.Failf("unable to parse the http result with error: %v", err)
	}

	pullSpec := gjson.Get(string(body), `pullSpec`).String()
	return pullSpec
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

// isTechPreviewNoUpgrade checks if a cluster is a TechPreviewNoUpgrade cluster
func isTechPreviewNoUpgrade(oc *exutil.CLI) bool {
	featureGate, err := oc.AdminConfigClient().ConfigV1().FeatureGates().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		e2e.Failf("could not retrieve feature-gate: %v", err)
	}

	return featureGate.Spec.FeatureSet == configv1.TechPreviewNoUpgrade
}

// check data by running curl on a pod
func checkMetric(oc *exutil.CLI, url, token, metricString string, timeout time.Duration) {
	var metrics string
	var err error
	getCmd := "curl -G -k -s -H \"Authorization:Bearer " + token + "\" " + url
	err = wait.Poll(10*time.Second, timeout*time.Second, func() (bool, error) {
		metrics, err = compat_otp.RemoteShPod(oc, "openshift-monitoring", "prometheus-k8s-0", "sh", "-c", getCmd)
		if err != nil || !strings.Contains(metrics, metricString) {
			return false, nil
		}
		return true, err
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("The metrics %s failed to contain %s", metrics, metricString))
}

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

// this function is used to check whether proxy is configured or not
func checkProxy(oc *exutil.CLI) bool {
	httpProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpProxy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	httpsProxy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("proxy", "cluster", "-o=jsonpath={.status.httpsProxy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if httpProxy != "" || httpsProxy != "" {
		return true
	}
	return false
}

func restartMicroshiftService(oc *exutil.CLI, nodeName string) {
	// As restart the microshift service, the debug node pod will quit with error
	compat_otp.DebugNodeWithChroot(oc, nodeName, "/bin/bash", "-c", "systemctl restart microshift")
	exec.Command("bash", "-c", "sleep 60").Output()
	checkNodeStatus(oc, nodeName, "Ready")
}

func checkNodeStatus(oc *exutil.CLI, nodeName string, expectedStatus string) {
	var expectedStatus1 string
	if expectedStatus == "Ready" {
		expectedStatus1 = "True"
	} else if expectedStatus == "NotReady" {
		expectedStatus1 = "Unknown"
	} else {
		err1 := fmt.Errorf("TBD supported node status")
		o.Expect(err1).NotTo(o.HaveOccurred())
	}
	err := wait.Poll(5*time.Second, 15*time.Minute, func() (bool, error) {
		statusOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeName, "-ojsonpath={.status.conditions[-1].status}").Output()
		if err != nil {
			e2e.Logf("\nGet node status with error : %v", err)
			return false, nil
		}
		e2e.Logf("Expect Node %s in state %v, kubelet status is %s", nodeName, expectedStatus, statusOutput)
		if statusOutput != expectedStatus1 {
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Node %s is not in expected status %s", nodeName, expectedStatus))
}

// get cluster resource name list
func getClusterResourceName(fileName string) ([]string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var clusterResourceNameList []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		clusterResourceNameList = append(clusterResourceNameList, strings.Split(scanner.Text(), " ")[0])
	}
	return clusterResourceNameList, scanner.Err()
}

func createCSAndISCP(oc *exutil.CLI, podLabel string, namespace string, expectedStatus string, packageNum int) {
	var files []string
	var yamlFiles []string

	root := "oc-mirror-workspace"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		e2e.Failf("Can't walk the oc-mirror-workspace directory")
	}

	for _, file := range files {
		if matched, _ := regexp.MatchString("yaml", file); matched {
			fmt.Println("file name is %v \n", file)
			yamlFiles = append(yamlFiles, file)
		}
	}

	for _, yamlFileName := range yamlFiles {
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", yamlFileName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "cat "+yamlFileName).Output()
	}

	e2e.Logf("Check the version and item from catalogsource")
	//oc get pod -n openshift-marketplace -l olm.catalogSource=redhat-operator-index
	assertPodOutput(oc, "olm.catalogSource="+podLabel, namespace, expectedStatus)

	//oc get packagemanifests --selector=catalog=redhat-operator-index -o=jsonpath='{.items[*].metadata.name}'
	waitErr := wait.Poll(10*time.Second, 90*time.Second, func() (bool, error) {
		out, err := oc.AsAdmin().Run("get").Args("packagemanifests", "--selector=catalog="+podLabel, "-o=jsonpath={.items[*].metadata.name}", "-n", namespace).Output()
		mirrorItemList := strings.Fields(out)
		if len(mirrorItemList) != packageNum || err != nil {
			e2e.Logf("the err:%v and mirrorItemList: %v, and try next round", err, mirrorItemList)
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but still can't find  packagemanifest")

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

func removeCSAndISCP(oc *exutil.CLI) {
	var files []string
	var yamlFiles []string
	root := "oc-mirror-workspace"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		e2e.Failf("Can't walk the oc-mirror-workspace directory")
	}

	for _, file := range files {
		if matched, _ := regexp.MatchString("yaml", file); matched {
			fmt.Println("file name is %v \n", file)
			yamlFiles = append(yamlFiles, file)
		}
	}

	for _, deleteFileName := range yamlFiles {
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", deleteFileName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Check if BaselineCapabilities have been set to None
func isBaselineCapsSet(oc *exutil.CLI, component string) bool {
	baselineCapabilitySet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.spec.capabilities.baselineCapabilitySet}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("baselineCapabilitySet parameters: %v\n", baselineCapabilitySet)
	return strings.Contains(baselineCapabilitySet, component)
}

// Check if component is listed in clusterversion.status.capabilities.enabledCapabilities
func isEnabledCapability(oc *exutil.CLI, component string) bool {
	enabledCapabilities, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.capabilities.enabledCapabilities}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster enabled capability parameters: %v\n", enabledCapabilities)
	return strings.Contains(enabledCapabilities, component)
}

// this function is used to check whether openshift-samples installed or not
func checkOpenshiftSamples(oc *exutil.CLI) bool {
	err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "openshift-samples").Execute()
	if err != nil {
		e2e.Logf("Get clusteroperator openshift-samples failed with error")
		return true
	}
	return false
}

func isSNOCluster(oc *exutil.CLI) bool {
	//Only 1 master, 1 worker node and with the same hostname.
	masterNodes, _ := compat_otp.GetClusterNodesBy(oc, "master")
	workerNodes, _ := compat_otp.GetClusterNodesBy(oc, "worker")
	if len(masterNodes) == 1 && len(workerNodes) == 1 && masterNodes[0] == workerNodes[0] {
		return true
	}
	return false
}

// this function is used to check the build status
func checkBuildStatus(oc *exutil.CLI, buildname string, namespace string, expectedStatus string) {
	err := wait.PollImmediate(10*time.Second, 15*time.Minute, func() (bool, error) {
		phase, err := oc.Run("get").Args("build", buildname, "-n", namespace, "--template", "{{.status.phase}}").Output()
		if err != nil {
			return false, nil
		}
		if phase != expectedStatus {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		oc.Run("describe").Args("build/"+buildname, "-n", namespace).Execute()
		oc.Run("logs").Args("build/"+buildname, "-n", namespace, "--tail", "5").Execute()
	}
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("build status is not same as expected"))
}

// this function is used to get the prune resource name by namespace
func getPruneResourceName(pruneCMD string) []string {
	var pruneRsList []string
	err := wait.PollImmediate(30*time.Second, 5*time.Minute, func() (bool, error) {
		pruneRsName, err := exec.Command("bash", "-c", pruneCMD).Output()
		pruneRsList = strings.Fields(strings.ReplaceAll(string(pruneRsName), "\n", " "))
		if err != nil {
			return false, nil
		}
		if len(pruneRsList) == 0 {
			return false, nil
		}
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "prune build num is not same as expected")
	sort.Strings(pruneRsList)
	e2e.Logf("pruneRsList %v:", pruneRsList)
	return pruneRsList
}

// WaitForDeploymentPodsToBeReady waits for the specific deployment to be ready
func waitForDeploymentPodsToBeReady(oc *exutil.CLI, namespace string, name string) {
	err := wait.Poll(20*time.Second, 300*time.Second, func() (done bool, err error) {
		deployment, err := oc.AdminKubeClient().AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				e2e.Logf("Waiting for availability of deployment/%s\n", name)
				return false, nil
			}
			return false, err
		}
		if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas && deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas {
			e2e.Logf("Deployment %s available (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
			return true, nil
		}
		e2e.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name, deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
	if err != nil {
		err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("--tail", "15", "deployment/"+name, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("deployment/"+name, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Failf("deployment %s is not availabile", name)
	}
}

func addLabelToNode(oc *exutil.CLI, label string, workerNodeName string, resource string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args(resource, workerNodeName, label, "--overwrite").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nLabel Added")
}

func removeLabelFromNode(oc *exutil.CLI, label string, workerNodeName string, resource string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("label").Args(resource, workerNodeName, label).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nLabel Removed")
}

func assertSpecifiedPodStatus(oc *exutil.CLI, podname string, namespace string, expected string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		podStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podname, "-n", namespace, "-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the result of pod:%v", podStatus)
		if strings.Contains(podStatus, expected) {
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("the state of pod with name %s is not expected %s", podname, expected))
}

func waitForResourceDisappear(oc *exutil.CLI, resource string, resourcename string, namespace string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		err := oc.Run("get").Args(resource, resourcename, "-n", namespace).Execute()
		if o.Expect(err.Error()).Should(o.ContainSubstring("exit status 1")) {
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("The specified resrouce %s with name %s is not disappear in time ", resource, resourcename))
}

// make sure the PVC is Bound to the PV
func waitForPvcStatus(oc *exutil.CLI, namespace string, pvcname string) {
	err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		pvStatus, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pvc", pvcname, "-o=jsonpath='{.status.phase}'").Output()
		if err != nil {
			return false, err
		}
		if match, _ := regexp.MatchString("Bound", pvStatus); match {
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "The PVC is not Bound as expected")
}

// wait for DC to be ready
func waitForDeploymentconfigToBeReady(oc *exutil.CLI, namespace string, name string) {
	err := wait.Poll(10*time.Second, 300*time.Second, func() (done bool, err error) {
		dcAvailableReplicas, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dc/"+name, "-n", namespace, "-o=jsonpath={.status.availableReplicas}").Outputs()
		if err != nil {
			e2e.Logf("error: %v happen, try next", err)
			return false, nil
		}
		dcReadyReplicas, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dc/"+name, "-n", namespace, "-o=jsonpath={.status.readyReplicas}").Outputs()
		if err != nil {
			e2e.Logf("error: %v happen, try next", err)
			return false, nil
		}
		dcReplicas, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dc/"+name, "-n", namespace, "-o=jsonpath={.spec.replicas}").Outputs()
		if err != nil {
			e2e.Logf("error: %v happen, try next", err)
			return false, nil
		}
		if dcAvailableReplicas == dcReadyReplicas && dcReadyReplicas == dcReplicas {
			e2e.Logf("Deploymentconfig is ready")
			return true, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Deploymentconfig %s is not available", name))
}

// this function is used to check whether must-gather imagestreamtag exist or not
func checkMustgatherImagestreamTag(oc *exutil.CLI) bool {
	err := oc.AsAdmin().WithoutNamespace().Run("get").Args("istag", "must-gather:latest", "-n", "openshift").Execute()
	if err != nil {
		e2e.Logf("Failed to get the must-gather imagestreamtag")
		return false
	}
	return true
}

func getWorkersList(oc *exutil.CLI) []string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(output, " ")
}

func getClusterRegion(oc *exutil.CLI) string {
	node := getWorkersList(oc)[0]
	region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", node, "-o=jsonpath={.metadata.labels.failure-domain\\.beta\\.kubernetes\\.io\\/region}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return region
}

func getOCPerKubeConf(oc *exutil.CLI, guestClusterKubeconfig string) *exutil.CLI {
	if guestClusterKubeconfig == "" {
		return oc
	}
	return oc.AsGuestKubeconf()
}

func assertPullSecret(oc *exutil.CLI) bool {
	dirName := "/tmp/" + compat_otp.GetRandomString()
	err := os.MkdirAll(dirName, 0o755)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirName)
	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to", dirName, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	oauthFilePath := dirName + "/.dockerconfigjson"
	secretContent, err := exec.Command("bash", "-c", fmt.Sprintf("cat %v", oauthFilePath)).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if matched, _ := regexp.MatchString("registry.ci.openshift.org", string(secretContent)); !matched {
		return false
	} else {
		return true
	}
}

func getTotalAllocatableMemory(oc *exutil.CLI, allocatableMemory string, totalRequests string) int {
	var (
		totalAllocatableMemoryInBytes int
		totalRequestedMemoryInBytes   int
	)
	if strings.Contains(totalRequests, "Gi") {
		requestedMemoryInBytesStr := strings.Split(totalRequests, "Gi")[0]
		requestedMemoryInBytes, _ := strconv.Atoi(requestedMemoryInBytesStr)
		totalRequestedMemoryInBytes = requestedMemoryInBytes * (1024 * 1024 * 1024)
	} else if strings.Contains(totalRequests, "Mi") {
		requestedMemoryInBytesStr := strings.Split(totalRequests, "Mi")[0]
		requestedMemoryInBytes, _ := strconv.Atoi(requestedMemoryInBytesStr)
		totalRequestedMemoryInBytes = requestedMemoryInBytes * (1024 * 1024)
	} else if strings.Contains(totalRequests, "Ki") {
		requestedMemoryInBytesStr := strings.Split(totalRequests, "Ki")[0]
		requestedMemoryInBytes, _ := strconv.Atoi(requestedMemoryInBytesStr)
		totalRequestedMemoryInBytes = requestedMemoryInBytes * 1024
	} else {
		totalRequestedMemoryInBytes, _ = strconv.Atoi(totalRequests)
	}
	if strings.Contains(allocatableMemory, "Gi") {
		allocatableMemoryInBytesStr := strings.Split(allocatableMemory, "Ki")[0]
		allocatableMemoryInBytes, _ := strconv.Atoi(allocatableMemoryInBytesStr)
		totalAllocatableMemoryInBytes = allocatableMemoryInBytes * (1024 * 1024 * 1024)
	} else if strings.Contains(allocatableMemory, "Mi") {
		allocatableMemoryInBytesStr := strings.Split(allocatableMemory, "Mi")[0]
		allocatableMemoryInBytes, _ := strconv.Atoi(allocatableMemoryInBytesStr)
		totalAllocatableMemoryInBytes = allocatableMemoryInBytes * (1024 * 1024)
	} else if strings.Contains(allocatableMemory, "Ki") {
		allocatableMemoryInBytesStr := strings.Split(allocatableMemory, "Ki")[0]
		allocatableMemoryInBytes, _ := strconv.Atoi(allocatableMemoryInBytesStr)
		totalAllocatableMemoryInBytes = allocatableMemoryInBytes * 1024
	} else {
		totalAllocatableMemoryInBytes, _ = strconv.Atoi(allocatableMemory)
	}

	totalMemoryInBytes := totalAllocatableMemoryInBytes - totalRequestedMemoryInBytes
	return totalMemoryInBytes
}

func (pd *priorityPodDefinition) createPriorityPod(oc *exutil.CLI) {
	o.Eventually(func() bool {
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pd.template, "-p", "NAME="+pd.name, "LABEL="+pd.label, "MEMORY="+strconv.Itoa(pd.memory), "PRIORITYCLASSNAME="+pd.priorityClassName, "NAMESPACE="+pd.namespace)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false
		}
		return true
	}).WithTimeout(20 * time.Second).WithPolling(5 * time.Second).Should(o.BeTrue())
}

func checkStatefulsetRollout(oc *exutil.CLI, namespace string, statefulSetName string) (bool, error) {
	statefulSet, err := oc.AdminKubeClient().AppsV1().StatefulSets(namespace).Get(context.Background(), statefulSetName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	// Check if the current number of ready replicas equals the desired replicas
	if statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas {
		return true, nil
	}
	return false, nil
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

func getSpecificFileName(fileDir string, pattern string) []string {
	files, err := ioutil.ReadDir(fileDir)
	o.Expect(err).NotTo(o.HaveOccurred())

	var matchingFiles []string
	e2e.Logf("the origin files %v", files)
	for _, file := range files {
		match, err := regexp.MatchString(pattern, string(file.Name()))
		o.Expect(err).NotTo(o.HaveOccurred())
		if match {
			matchingFiles = append(matchingFiles, string(file.Name()))
		}
	}
	e2e.Logf("the result files %v", matchingFiles)
	o.Expect(len(matchingFiles) > 0).To(o.BeTrue())
	return matchingFiles
}

func sha256File(fileName string) (string, error) {
	file, err := os.Open(fileName)
	defer file.Close()
	o.Expect(err).NotTo(o.HaveOccurred())
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	o.Expect(err).NotTo(o.HaveOccurred())
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getSha256SumFromFile(fileName string) string {
	var fileSum string
	content, err := ioutil.ReadFile(fileName)
	o.Expect(err).NotTo(o.HaveOccurred())
	lines := strings.Split(string(content), "\n")
	for _, v := range lines {
		trimline := strings.TrimSpace(v)
		if strings.Contains(trimline, "openshift-install") {
			fileSum = strings.Fields(trimline)[0]
			o.Expect(fileSum).NotTo(o.BeEmpty())
		}
	}
	return fileSum
}

func getTimeFromNode(oc *exutil.CLI, nodeName string, ns string) time.Time {
	timeStr, _, dErr := oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+nodeName, "-n", ns, "--", "chroot", "/host", "date", "+%Y-%m-%dT%H:%M:%SZ").Outputs()
	o.Expect(dErr).ShouldNot(o.HaveOccurred(), "Error getting date in node %s", nodeName)
	layout := "2006-01-02T15:04:05Z"
	returnTime, perr := time.Parse(layout, timeStr)
	o.Expect(perr).NotTo(o.HaveOccurred())
	return returnTime
}

func checkMustgatherLogTime(mustgatherDir string, nodeName string, timestamp string) {
	filesUnderGather, err := ioutil.ReadDir(mustgatherDir)
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to read the must-gather dir")
	dataDir := ""
	for _, fileD := range filesUnderGather {
		if fileD.IsDir() {
			dataDir = fileD.Name()
		}
	}
	e2e.Logf("The data dir is %v", dataDir)
	nodeLogsFile := mustgatherDir + "/" + dataDir + "/nodes/" + nodeName + "/" + nodeName + "_logs_kubelet.gz"
	e2e.Logf("The node log file is %v", nodeLogsFile)
	nodeLogsData, err := exec.Command("bash", "-c", fmt.Sprintf("zcat %v ", nodeLogsFile)).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	if strings.Contains(string(nodeLogsData), timestamp) {
		e2e.Failf("Got unexpected time %v, must-gather wrong", timestamp)
	} else {
		e2e.Logf("Only able to successfully retreieve logs after timestamp %v", timestamp)
	}
}
func checkInspectLogTime(inspectDir string, podName string, timestamp string) {
	podLogsDir := inspectDir + "/namespaces/openshift-multus/pods/" + podName + "/kube-multus/kube-multus/logs"
	var fileList []string
	err := filepath.Walk(podLogsDir, func(path string, info os.FileInfo, err error) error {
		fileList = append(fileList, path)
		return nil
	})
	if err != nil {
		e2e.Failf("Failed to check inspect directory")
	}
	for i := 1; i < len(fileList); i++ {
		podLogData, err := ioutil.ReadFile(fileList[i])
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(string(podLogData), timestamp) {
			e2e.Failf("Got unexpected time, inspect wrong")
		} else {
			e2e.Logf("Only able to successfully retreive the inspect logs after timestamp %v which is expected", timestamp)
		}
	}
}

func waitCRDAvailable(oc *exutil.CLI, crdName string) error {
	return wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", crdName).Execute()
		if err != nil {
			e2e.Logf("The crd with name %v still not ready, please try again", crdName)
			return false, nil
		}
		return true, nil
	})
}

func waitCreateCr(oc *exutil.CLI, crFileName string, namespace string) error {
	return wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", crFileName, "-n", namespace).Execute()
		if err != nil {
			e2e.Logf("The cr with file %v created failed, please try again", crFileName)
			return false, nil
		}
		return true, nil
	})
}

func checkGatherLogsForImage(oc *exutil.CLI, filePath string) {
	imageDir, err := os.Open(filePath)
	if err != nil {
		e2e.Logf("Error opening directory:", err)
	}

	defer imageDir.Close()

	// Read the contents of the directory
	gatherlogInfos, err := imageDir.Readdir(-1)
	if err != nil {
		e2e.Logf("Error reading directory contents:", err)
	}

	// Check if gather.logs exist for each image
	for _, gatherlogInfo := range gatherlogInfos {
		if gatherlogInfo.IsDir() {
			filesList, err := exec.Command("bash", "-c", fmt.Sprintf("ls -l %v/%v", filePath, gatherlogInfo.Name())).Output()
			if err != nil {
				e2e.Failf("Error listing directory:", err)
			}
			o.Expect(strings.Contains(string(filesList), "gather.logs")).To(o.BeTrue())
		} else {
			e2e.Logf("Not a directory, continuing to the next")
		}
	}
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
func createCSAndISCPNoPackageCheck(oc *exutil.CLI, podLabel string, namespace string, expectedStatus string) {
	var files []string
	var yamlFiles []string

	root := "oc-mirror-workspace"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		e2e.Failf("Can't walk the oc-mirror-workspace directory")
	}

	for _, file := range files {
		if matched, _ := regexp.MatchString("yaml", file); matched {
			fmt.Printf("file name is %v \n", file)
			yamlFiles = append(yamlFiles, file)
		}
	}

	for _, yamlFileName := range yamlFiles {
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", yamlFileName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exec.Command("bash", "-c", "cat "+yamlFileName).Output()
	}

	e2e.Logf("Check the version and item from catalogsource")
	assertPodOutput(oc, "olm.catalogSource="+podLabel, namespace, expectedStatus)
}

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

func checkImageRegistryPodNum(oc *exutil.CLI) bool {
	podNum, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.replicas}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	number, _ := strconv.Atoi(podNum)
	podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(context.Background(), metav1.ListOptions{LabelSelector: "docker-registry=default"})
	if len(podList.Items) != number {
		e2e.Logf("the pod number is not %d", number)
		return false
	}
	return true
}

func createEmptyAuth(authfilepath string) {
	authF, err := os.Create(authfilepath)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer authF.Close()
	authContent := fmt.Sprintf(`{}`)
	authW := bufio.NewWriter(authF)
	_, werr := authW.WriteString(authContent)
	authW.Flush()
	o.Expect(werr).NotTo(o.HaveOccurred())
}

func checkFileContent(filename string, expectedStr string) bool {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		e2e.Failf("failed to read the file ")
	}
	s := string(b)
	if strings.Contains(s, expectedStr) {
		return true
	} else {
		return false
	}
}

func checkOcPlatform(oc *exutil.CLI) string {
	ocVersion, err := oc.Run("version").Args("--client", "-o", "yaml").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(ocVersion, "amd64") {
		return "amd64"
	} else if strings.Contains(ocVersion, "arm64") {
		return "arm64"
	} else if strings.Contains(ocVersion, "s390x") {
		return "s390x"
	} else if strings.Contains(ocVersion, "ppc64le") {
		return "ppc64le"
	} else {
		return "Unknown platform"
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

func deleteNamespace(oc *exutil.CLI, namespace string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", namespace, "--ignore-not-found", "--timeout=60s").Execute()
	if err != nil {
		customColumns := "-o=custom-columns=NAME:.metadata.name,CR_NAME:.spec.names.singular,SCOPE:.spec.scope"
		crd, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "-n", namespace, customColumns).Output()
		e2e.Logf("The result of \"oc get crd -n %s %s\" is: %s", namespace, customColumns, crd)
		nsStatus, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", namespace, "-n", namespace, "-o=jsonpath={.status}").Output()
		e2e.Logf("The result of \"oc get ns %s -n %s =-o=jsonpath={.status}\" is: %s", namespace, namespace, nsStatus)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

type AuthEntry struct {
	Auth string `json:"auth"`
}
type AuthsData struct {
	Auths map[string]AuthEntry `json:"auths"`
}

// Helper function to load and parse the auth file.
// It gracefully handles the file not existing.
func loadAuthsFile(authFile string) (AuthsData, error) {
	var config AuthsData

	fileBytes, err := os.ReadFile(authFile)
	if err != nil {
		return config, err
	}

	// File exists, unmarshal it
	if err := json.Unmarshal(fileBytes, &config); err != nil {
		return config, fmt.Errorf("failed to parse auth file %s: %w", authFile, err)
	}

	// Ensure the map is initialized if the file has "auths": null
	if config.Auths == nil {
		config.Auths = make(map[string]AuthEntry)
	}

	return config, nil
}

func saveAuthsFile(authFile string, config AuthsData) error {
	// Marshal with "pretty-print" indentation
	fileBytes, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal auth data: %w", err)
	}

	// WriteFile atomically handles file creation/truncation and permissions
	if err := os.WriteFile(authFile, fileBytes, 0644); err != nil {
		return fmt.Errorf("failed to write auth file %s: %w", authFile, err)
	}

	return nil
}

func appendPullSecretAuth(authFile, regRouter, pullSecretLocation string) error {
	if pullSecretLocation == "" {
		return fmt.Errorf("Fail to get the original  %s !!", pullSecretLocation)
	}

	// 1. Read the pull secret
	pullSecret, err := loadAuthsFile(pullSecretLocation)
	if err != nil {
		return err
	}

	// 2. Find the auth entry
	regAuth, found := pullSecret.Auths[regRouter]
	if !found || regAuth.Auth == "" {
		return fmt.Errorf("error: '%s' key not found or auth is empty in %s", regRouter, pullSecretLocation)
	}

	// 3. Load the target authFile
	dockerConfig, err := loadAuthsFile(authFile)
	if err != nil {
		return err // Error already has context
	}

	// 4. Modify the auths map
	dockerConfig.Auths[regRouter] = regAuth

	// 5. Write the modified data back to the authFile
	if err := saveAuthsFile(authFile, dockerConfig); err != nil {
		return err // Error already has context
	}

	log.Printf("Successfully updated auth for '%s' in %s\n", regRouter, authFile)
	return nil
}

func replaceRegistryNameInCfg(imageSetConfigFile, imageRegistryName string) (configFilePath string, err error) {
	content, err := os.ReadFile(imageSetConfigFile)
	if err != nil {
		return "", err
	}

	fileContent := string(content)
	newContent := strings.Replace(fileContent, "localhost:", imageRegistryName+":", 1)

	newFile, err := os.CreateTemp("", "isc-*.yaml")
	defer newFile.Close()
	if err != nil {
		return "", err
	}

	fullPath := newFile.Name()

	err = os.WriteFile(fullPath, []byte(newContent), 0644)
	if err != nil {
		return "", err
	}
	return fullPath, nil
}

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
