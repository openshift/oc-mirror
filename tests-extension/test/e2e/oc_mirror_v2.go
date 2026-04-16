package workloads

import (
	"bufio"
	"context"
	"fmt"
	"github.com/openshift/oc-mirror-tests-extension/test/e2e/testdata"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	format "github.com/onsi/gomega/format"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"github.com/openshift/origin/test/extended/util/compat_otp/architecture"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[OTP][sig-cli] Workloads ocmirror v2 works well", func() {
	defer g.GinkgoRecover()

	var (
		oc = compat_otp.NewCLI("ocmirrorv2", compat_otp.KubeConfigPath())
	)
	format.MaxLength = 0

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-72973-support mirror multi-arch additional images for v2 [Serial]", func() {
		dirname := "/tmp/case72973"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72973.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72973")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72973")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirroring of additionalImages to disk")
		waitErr := wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk for additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk for additionalImages still failed")

		compat_otp.By("Start mirroring of additionalImages to registry")
		waitErr = wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/multiarch", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror of additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror for additionalImages still failed")

		// Validate if multi arch additionalImages have been mirrored
		compat_otp.By("Validate if multi arch additionalImages have been mirrored")
		additionalImageList := []string{"/multiarch/ubi8/ubi:latest", "/multiarch/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27", "/multiarch/openshifttest/scratch@sha256:b045c6ba28db13704c5cbf51aff3935dbed9a692d508603cc80591d89ab26308"}
		for _, image := range additionalImageList {
			if strings.Contains(image, "scratch") {
				o.Expect(assertMultiImage(serInfo.serviceName+image, dirname+"/.dockerconfigjson")).To(o.BeFalse())
			} else {
				o.Expect(assertMultiImage(serInfo.serviceName+image, dirname+"/.dockerconfigjson")).To(o.BeTrue())
			}
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Critical-73359-Validate mirror2mirror for operator for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case73359"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73359")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73359")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73359.yaml")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-redhatcatalog73359-v4-14", "openshift-marketplace", "Running")

		compat_otp.By("Install the operator from the new catalogsource")
		localstorageSub, localstorageOG := getOperatorInfo(oc, "local-storage-operator", "openshift-local-storage", "registry.redhat.io/redhat/redhat-operator-index:v4.14", "cs-redhatcatalog73359-v4-14")
		defer removeOperatorFromCustomCS(oc, localstorageSub, localstorageOG, "openshift-local-storage")
		installOperatorFromCustomCS(oc, localstorageSub, localstorageOG, "openshift-local-storage", "local-storage-operator")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-73452-Validate mirror2mirror for OCI operator  and addition image for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case73452"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73452")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73452")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73452.yaml")

		compat_otp.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.16 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-ocicatalog73452-v14", "openshift-marketplace", "Running")

		compat_otp.By("Install the operator from the new catalogsource")
		deschedulerSub, deschedulerOG := getOperatorInfo(oc, "cluster-kube-descheduler-operator", "openshift-kube-descheduler-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.16", "cs-ocicatalog73452-v14")
		defer removeOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator")
		installOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator", "descheduler-operator")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-73377-support dry-run for v2 [Serial]", func() {
		dirname := "/tmp/case73377"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73377.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-zcon-zosconnect-catalog@sha256:6f02ecef46020bcd21bdd24a01f435023d5fc3943972ef0d9769d5276e178e76 oci://%s", dirname+"/ibm-catalog")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("Copy of ibm catalog failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("Max time reached but skopeo copy of ibm catalog failed"))

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73377")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73377")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start dry run of mirrro2disk")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--dry-run", "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorToDiskOutput, "dry-run/missing.txt") && strings.Contains(mirrorToDiskOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to Disk dry run has been completed successfully")
				return true, nil
			}
			return false, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		// Validate if source and destination are right in the mapping.txt file
		compat_otp.By("check if source and destination are right in the mapping.txt file")
		mappingTextContent, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/case73377/working-dir/dry-run/mapping.txt | head -n 10")).Output()
		e2e.Logf("mappingTextContent is %s", mappingTextContent)
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		mappingTextContentStr := string(mappingTextContent)

		if matched, _ := regexp.MatchString(".*docker://registry.redhat.io.*=docker://localhost:55000.*", mappingTextContentStr); !matched {
			e2e.Failf("Source and destination for mirror2disk mode is incorrect in mapping.txt")
		} else {
			e2e.Logf("Source and destination for mirror2disk are set correctly")
		}

		compat_otp.By("Start mirror2disk")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		compat_otp.By("Start dry run of disk2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			diskToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/d2m", "--v2", "--dest-tls-verify=false", "--dry-run", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			if strings.Contains(diskToMirrorOutput, "dry-run/mapping.txt") {
				e2e.Logf("Disk to mirror dry run has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		// Check if source and destination are right for disk2mirror in mapping.txt file
		mappingTextContentd2m, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/case73377/working-dir/dry-run/mapping.txt | head -n 10")).Output()
		e2e.Logf("mappingTextContent is %s", mappingTextContentd2m)
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		mappingTextContentd2mStr := string(mappingTextContentd2m)

		if matched, _ := regexp.MatchString(".*docker://localhost:55000.*=docker://"+serInfo.serviceName+"/d2m.*", mappingTextContentd2mStr); !matched {
			e2e.Failf("Source and destination for disk2mirror mode is incorrect in mapping.txt")
		} else {
			e2e.Logf("Source and destination for disk2mirror are set correctly")
		}

		compat_otp.By("Start dry run of mirror2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			mirrorToMirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--dest-tls-verify=false", "--dry-run", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirrorToMirrorOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to mirror dry run has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed")

		// Check if source and destination are right for mirror2mirror in mapping.txt file
		mappingTextContentm2m, err := exec.Command("bash", "-c", fmt.Sprintf("cat /tmp/case73377/working-dir/dry-run/mapping.txt | head -n 10")).Output()
		e2e.Logf("mappingTextContent is %s", mappingTextContentm2m)
		if err != nil {
			e2e.Logf("Error reading file must-gather.logs:", err)
		}
		mappingTextContentm2mStr := string(mappingTextContentm2m)

		if matched, _ := regexp.MatchString(".*docker://registry.redhat.io.*=docker://"+serInfo.serviceName+"/m2m.*", mappingTextContentm2mStr); !matched {
			e2e.Failf("Source and destination for mirror2mirror mode is incorrect in mapping.txt")
		} else {
			e2e.Logf("Source and destination for mirror2mirror are set correctly")
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-72949-support targetCatalog and targetTag setting of mirror v2docker2 and oci for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72949"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72949-1.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-72949-2.yaml")

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-zcon-zosconnect-catalog@sha256:6f02ecef46020bcd21bdd24a01f435023d5fc3943972ef0d9769d5276e178e76 oci://%s --remove-signatures", dirname+"/ibm-catalog"))

		compat_otp.By("Start mirror2mirror for oci & rh marketplace operators")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		rhMarketUri := "https://" + serInfo.serviceName + "/v2/72949/redhat-marketplace-index/tags/list"
		validateTargetcatalogAndTag(rhMarketUri, "v15")
		ibmOciUri := "https://" + serInfo.serviceName + "/v2/72949/catalog/tags/list"
		validateTargetcatalogAndTag(ibmOciUri, "v15")

		os.RemoveAll(".oc-mirror.log")
		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")

		compat_otp.By("Start disk2mirror")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		compat_otp.By("Validate the target catalog and tag")
		rhOperatorUri := "https://" + serInfo.serviceName + "/v2/72949/redhat-operator-index/tags/list"
		e2e.Logf("The rhOperatorUri is %v", rhOperatorUri)
		validateTargetcatalogAndTag(rhOperatorUri, "v4.15")
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:knarra-Medium-72938-should give clear information for invalid operator filter setting [Serial]", func() {
		dirname := "/tmp/case72938"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72938.yaml")
		imageSetYamlFileT := filepath.Join(ocmirrorBaseDir, "config-72938-1.yaml")

		compat_otp.By("Start mirrro2disk with min/max filtering")
		mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		if err != nil {
			if strings.Contains(mirrorToDiskOutput, "mixing both filtering by minVersion/maxVersion and filtering by channel minVersion/maxVersion is not allowed") {
				e2e.Logf("Error related to invalid operator filter by min/max is seen")
			} else {
				e2e.Failf("Error related to filtering by channel and package min/max is not seen")
			}
		}

		compat_otp.By("Start mirror2disk min/max with full true filtering")
		mirrorToDiskOutputFT, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileT, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		if err != nil {
			if strings.Contains(mirrorToDiskOutputFT, "Full: true cannot be mixed with versionRange") {
				e2e.Logf("Error related to invalid operator filtering with full true is seen")
			} else {
				e2e.Failf("Error related to invalid operator filtering with full true is not seen")
			}
		}

	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-72942-High-72918-High-72709-support max-nested-paths for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72942"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72942")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72942")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72942.yaml")

		compat_otp.By("Start mirror2disk, checkpoint for 72947 and 72918")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")

		compat_otp.By("Start disk2mirror with max-nested-paths")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/test/72942", "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--max-nested-paths", "2").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		compat_otp.By("Check if net path is right in idms/itms file")
		idmsTextContentStr := readFileContent("/tmp/case72942/working-dir/cluster-resources/idms-oc-mirror.yaml")
		validateFileContent(idmsTextContentStr, "test/72942-albo-aws-load-balancer-operator-bundle", "operator")
		validateFileContent(idmsTextContentStr, "test/72942-openshifttest-hello-openshift", "additionalimage")

		itmsTextContentStr := readFileContent("/tmp/case72942/working-dir/cluster-resources/itms-oc-mirror.yaml")
		validateFileContent(itmsTextContentStr, "test/72942-ubi8-ubi", "additionalimage")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-72942-catalog-v15", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-72942-redhat-redhat-operator-index-v4-15", "openshift-marketplace", "Running")

		compat_otp.By("Checkpoint for 72709, validate the result for additional images")
		_, outErr, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", dirname+"/.dockerconfigjson", serInfo.serviceName+"/test/72942-ubi8-ubi:latest", "--insecure").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(outErr, "the image is a manifest list")).To(o.BeTrue())
		_, outErr, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", "--registry-config", dirname+"/.dockerconfigjson", serInfo.serviceName+"/test/72942-openshifttest-hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "--insecure").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(outErr, "the image is a manifest list")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-72947-High-72948-support OCI filtering for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72947"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "50G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72948.yaml")

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 oci://%s --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		compat_otp.By("Start mirror2mirror for oci operators")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72913-Respect archive max size for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72913"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72913.yaml")

		compat_otp.By("Start mirror2disk with strict-archive")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		outputMes, _, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--strict-archive").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(outputMes, "maxArchiveSize 1G is too small compared to sizes of files")).To(o.BeTrue())

		compat_otp.By("Start mirror2disk without strict-archive")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72972-Medium-73381-Medium-74519-support to specify architectures of payload for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72972"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72972.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-72972.yaml")

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s  --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "40G", "/var/lib/registry")

		compat_otp.By("Start mirror2mirror ")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
		payloadImageInfo, err := oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/openshift/release-images:4.15.19-s390x").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Payloadinfo is %s", payloadImageInfo)
		o.Expect(strings.Contains(payloadImageInfo, "s390x")).To(o.BeTrue())

		compat_otp.By("Checkpoint for 74519")
		compat_otp.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Execute delete without force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.By("Checked the payload manifest again should failed")
		_, err = oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/openshift/release-images:4.15.19-s390x").Output()
		o.Expect(err).Should(o.HaveOccurred())

		compat_otp.By("Checked the operator manifest again should failed")
		_, err = oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/redhat/redhat-operator-index:v4.15").Output()
		o.Expect(err).Should(o.HaveOccurred())
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74649-Low-74646-Show warning when an eus channel with minor versions range >=2 for v2[Serial]", func() {
		dirname := "/tmp/case74649"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-74649.yaml")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "80G", "/var/lib/registry")

		compat_otp.By("Checkpoint for v2 m2d")
		err = wait.Poll(300*time.Second, 20*time.Minute, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2d v2")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
		err = os.RemoveAll(dirname + "/m2d")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Checkpoint for v2 m2m")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2m v2")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		compat_otp.By("Checkpoint for 74646: show warning when an eus channle with minor versions range >=2 for v2 m2d with dry-run")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2d v2 dry-run")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		compat_otp.By("Checkpoint for 74646: show warning when an eus channle with minor versions range >=2 for  v2 m2m with dry-run")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is not seen for m2m v2 dry-run")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74650-Should no warning about eus when use the eus channel with minor versions range < 2  for V1[Serial]", func() {
		dirname := "/tmp/case74650"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetV1MinorFile := filepath.Join(ocmirrorBaseDir, "config-74650-minor-v1.yaml")
		imageSetV1PatchFile := filepath.Join(ocmirrorBaseDir, "config-74650-patch-v1.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		compat_otp.By("Step 1 : no warning when minor diff < 2 for v1")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1MinorFile, "file://"+dirname+"/m2d", "--dry-run", "--v1").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v1 m2d")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1MinorFile, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run", "--v1").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v1 m2m")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		compat_otp.By("Step 2 : no warning when patch diff for v1")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1PatchFile, "file://"+dirname+"/m2d", "--dry-run", "--v1").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v1 m2d")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV1PatchFile, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run", "--v1").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v1 m2m")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with  %s", err))

	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74660-Low-74646-Show warning when an eus channel with minor versions range >=2 for v1[Serial]", func() {
		dirname := "/tmp/case74660"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-74660.yaml")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "80G", "/var/lib/registry")

		defer os.RemoveAll("oc-mirror-workspace")
		compat_otp.By("Checkpoint for v1 m2d")
		err = wait.Poll(300*time.Second, 20*time.Minute, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname+"/m2d", "--v1").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2d test failed as can't find the expected warning")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
		err = os.RemoveAll(dirname + "/m2d")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Checkpoint for v1 m2m")
		err = wait.Poll(300*time.Second, 20*time.Minute, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2m test failed as can't find the expected warning")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		compat_otp.By("Checkpoint for 74646: show warning when an eus channle with minor versions range >=2 for v1 m2d with dry-run")
		err = wait.Poll(300*time.Second, 20*time.Minute, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname+"/m2d", "--dry-run", "--v1").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2d with dry-run test failed as can't find the expected warning")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		compat_otp.By("Checkpoint for 74646: show warning when an eus channel with minor versions range >=2 for v1 m2m with dry-run")
		err = wait.Poll(300*time.Second, 20*time.Minute, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run", "--v1").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("V1 m2m with dry-run test failed as can't find the expected warning")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-74733-Should no warning about eus when use the eus channel with minor versions range < 2  for V2[Serial]", func() {
		dirname := "/tmp/case74733"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetV2MinorFile := filepath.Join(ocmirrorBaseDir, "config-74650-minor-v2.yaml")
		imageSetV2PatchFile := filepath.Join(ocmirrorBaseDir, "config-74650-patch-v2.yaml")

		compat_otp.By("Step 1 : no warning when minor diff < 2 for v2")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2MinorFile, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v2 m2d")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with  %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2MinorFile, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for minor diff <2 for v2 m2m")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		compat_otp.By("Step 2 : no warning when patch diff for v2")
		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2PatchFile, "--v2", "file://"+dirname+"/m2d", "--authfile", dirname+"/.dockerconfigjson", "--dry-run").OutputToFile(getRandomString() + "workload-mirror.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v2 m2d")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with %s", err))

		err = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			mirrorOutFile, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetV2PatchFile, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--dry-run").OutputToFile(getRandomString() + "workload-m2m.txt")
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if validateStringFromFile(mirrorOutFile, "To correctly determine the upgrade path for EUS releases") {
				return false, fmt.Errorf("Upgrade warning related to correctly determing the upgrade path is showing for patch diff for v2 m2m")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Mirror command failed with  %s", err))
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-73783-Do not generate IDMS or ITMS if nothing has been mirrored [Serial]", func() {
		dirname := "/tmp/case73783"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73783.yaml")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		compat_otp.By("Start mirror2mirror and verify no idms and itms has been generated since nothing is mirrored")
		mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName+"/noidmsitms", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(mirrorOutput).To(o.ContainSubstring("Nothing mirrored. Skipping IDMS and ITMS files generation."))
		o.Expect(mirrorOutput).To(o.ContainSubstring("No catalogs mirrored. Skipping CatalogSource file generation"))
		o.Expect(mirrorOutput).To(o.ContainSubstring("No catalogs mirrored. Skipping ClusterCatalog file generation"))
		e2e.Logf("No ITMS & IDMS generated when nothing is mirrored, PASS")
		entries, err := os.ReadDir(dirname + "/working-dir/cluster-resources")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(entries) == 0).Should(o.BeTrue())
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72971-support mirror multiple catalogs (v2docker2 +oci) for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72971"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72971.yaml")

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.19 --remove-signatures  --insecure-policy oci://%s", dirname+"/redhat-operator-index"))
		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		compat_otp.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72971")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72971")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirror2mirror ")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-certified-operator-index-v4-19", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-marketplace-index-v4-19", "openshift-marketplace", "Running")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-latest", "openshift-marketplace", "Running")

		// compat_otp.By("Install operator from certified-operator CS")
		// nginxSub, nginxOG := getOperatorInfo(oc, "nginx-ingress-operator", "nginx-ingress-operator-ns", "registry.redhat.io/redhat/certified-operator-index:v4.19", "cs-certified-operator-index-v4-19")
		// defer removeOperatorFromCustomCS(oc, nginxSub, nginxOG, "nginx-ingress-operator-ns")
		// installOperatorFromCustomCS(oc, nginxSub, nginxOG, "nginx-ingress-operator-ns", "nginx-ingress-operator-controller-manager")

		// compat_otp.By("Install operator from redhat-marketplace CS")
		// aerospikeSub, aerospikeOG := getOperatorInfo(oc, "aerospike-kubernetes-operator-rhmp", "aerospike-ns", "registry.redhat.io/redhat/redhat-marketplace-index:v4.19", "cs-redhat-marketplace-index-v4-19")
		// defer removeOperatorFromCustomCS(oc, aerospikeSub, aerospikeOG, "aerospike-ns")
		// installCustomOperator(oc, aerospikeSub, aerospikeOG, "aerospike-ns", "aerospike-operator-controller-manager", "2")

		compat_otp.By("Install operator from redhat-operator CS")
		awslbSub, awslbOG := getOperatorInfo(oc, "dns-operator", "dns-operator-ns", "registry.redhat.io/redhat/redhat-operator-index:v4.19", "cs-redhat-operator-index-latest")
		defer removeOperatorFromCustomCS(oc, awslbSub, awslbOG, "dns-operator-ns")
		installAllNSOperatorFromCustomCS(oc, awslbSub, awslbOG, "dns-operator-ns", "dns-operator-controller-manager", "1")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-72917-support v2docker2 operator catalog filtering for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72917"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72917.yaml")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "35G", "/var/lib/registry")

		compat_otp.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-72917")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-72917")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirror2mirror ")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-19", "openshift-marketplace", "Running")
	})

	// Marking the test flaky due to issue https://issues.redhat.com/browse/CLID-214 just PASS on amd64 for now
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-73124-Validate operator mirroring works fine for the catalog that does not follow same structure as RHOI [Serial]", func() {
		serverPlatform := architecture.ClusterArchitecture(oc)
		if serverPlatform.String() != "amd64" {
			g.Skip("Test only runs on amd64")
		}
		compat_otp.By("Set registry config")
		dirname := "/tmp/case73124"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Get root ca")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-73124")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-73124")
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73124.yaml")

		compat_otp.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-bts-operator-catalog@sha256:866f0212eab7bc70cc7fcf7ebdbb4dfac561991f6d25900bd52f33cd90846adf  oci://%s  --remove-signatures --insecure-policy", dirname+"/ibm-catalog")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for oci ibm catalog failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for oci ibm catalog failed")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror for ibm oci catalog failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for ibm oci catalog still failed")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		ibmCatalogSourceName, err := exec.Command("bash", "-c", fmt.Sprintf("oc get catalogsource -n openshift-marketplace | awk '{print $1}' | grep ibm")).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ibmCatalogSourceName is %s", ibmCatalogSourceName)

		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource="+string(ibmCatalogSourceName), "openshift-marketplace", "Running")

		compat_otp.By("Install the operator from the new catalogsource")
		buildPruningBaseDir := testdata.FixturePath("workloads")
		ibmcatalogSubscription := filepath.Join(buildPruningBaseDir, "ibmcustomsub.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", ibmcatalogSubscription).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", ibmcatalogSubscription).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Wait for the operator pod running")
		if ok := waitForAvailableRsRunning(oc, "deploy", "ibm-bts-operator-controller-manager", "openshift-operators", "1"); ok {
			e2e.Logf("IBM operator with index structure different than RHOCI has been deployed successfully\n")
		} else {
			e2e.Failf("All pods related to ibm deployment are not running")
		}
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-72708-support delete function with force-cache-delete for V2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72708"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72708.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-72708.yaml")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "40G", "/var/lib/registry")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk for additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk for additionalImages still failed")

		compat_otp.By("Start mirroring to registry")
		waitErr = wait.Poll(300*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror of additionalImages failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror for additionalImages still failed")

		compat_otp.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Execute delete with force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--force-cache-delete=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72983-support registries.conf for normal operator mirror of v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72983"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72983.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch the first registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "35G", "/var/lib/registry")

		compat_otp.By("Trying to launch the second registry app")
		defer registry.deleteregistrySpecifyName(oc, "secondregistry")
		secondSerInfo := registry.createregistrySpecifyName(oc, "secondregistry")
		setRegistryVolume(oc, "deploy", "secondregistry", oc.Namespace(), "35G", "/var/lib/registry")

		compat_otp.By("Mirror to first registry")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create and set registries.conf")
		registryConfContent := getRegistryConfContentStr(serInfo.serviceName, "quay.io", "registry.redhat.io")
		homePath := getHomePath()
		homeRistryConfExist, _ := ensureContainersConfigDirectory(homePath)
		if !homeRistryConfExist {
			e2e.Failf("Failed to get or create the home registry config directory")
		}

		defer restoreRegistryConf(homePath)
		_, errStat := os.Stat(homePath + "/.config/containers/registries.conf")
		if errStat == nil {
			backupContainersConfig(homePath)
			setRegistryConf(registryConfContent, homePath)
		} else if os.IsNotExist(errStat) {
			setRegistryConf(registryConfContent, homePath)
		} else {
			e2e.Failf("Unexpected error %v", errStat)
		}

		compat_otp.By("Mirror to second registry")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+secondSerInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-72982-support registries.conf for OCI of v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72982"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72982.yaml")

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.15 --remove-signatures  --insecure-policy oci://%s --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson"))

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "35G", "/var/lib/registry")

		compat_otp.By("Trying to launch the second registry app")
		secondSerInfo := registry.createregistrySpecifyName(oc, "secondregistry")
		setRegistryVolume(oc, "deploy", "secondregistry", oc.Namespace(), "35G", "/var/lib/registry")

		compat_otp.By("Mirror to first registry")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create and set registries.conf")
		/*registryConfFile := dirname + "/registries.conf"
		createRegistryConf(registryConfFile, serInfo.serviceName)
		defer restoreRegistryConf()
		setRegistryConf(registryConfFile)*/

		registryConfContent := getRegistryConfContentStr(serInfo.serviceName, "quay.io", "registry.redhat.io")
		homePath := getHomePath()
		homeRistryConfExist, _ := ensureContainersConfigDirectory(homePath)
		if !homeRistryConfExist {
			e2e.Failf("Failed to get or create the home registry config directory")
		}

		defer restoreRegistryConf(homePath)
		_, errStat := os.Stat(homePath + "/.config/containers/registries.conf")
		if errStat == nil {
			backupContainersConfig(homePath)
			setRegistryConf(registryConfContent, homePath)
		} else if os.IsNotExist(errStat) {
			setRegistryConf(registryConfContent, homePath)
		} else {
			e2e.Failf("Unexpected error %v", errStat)
		}

		compat_otp.By("Mirror to second registry")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+secondSerInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75425-Validate oc-mirror is able to pull hypershift kubevirt coreos container image and mirror the same [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case75425"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75425.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2d, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("Mirror2disk for kubevirt coreos container image failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputm2d, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to disk for KubeVirt CoreOs Container image completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for kubevirt core os container failed")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputd2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("Disk2mirror for kubeVirt coreos container image failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputd2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Disk to mirror for KubeVirt CoreOs Container image completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for KubeVirt CoreOs Container image failed")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for KubeVirt Coreos Container image failed, retrying...")
				return false, nil
			}
			if strings.Contains(kubeVirtContainerImageOutputm2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to mirror for KubeVirt CoreOs Container image completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror for kubevirt coreos container image still failed")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75437-Validate oc-mirror does not error out when kubeVirtContainer is set to false in the ImageSetConfig yaml [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case75437"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75437.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2d, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("Mirror2disk when kubevirtContainer set to false is still failing, retrying...")
				return false, nil
			}
			if !strings.Contains(kubeVirtContainerImageOutputm2d, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to disk completed successfully when kubeVirtContainer is set to false in the imageSetConfig.yaml file")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed, when kubeVirtContainer is set to false")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputd2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("Disk2mirror when kubeVirtContainer set to false is still failing, retrying...")
				return false, nil
			}
			if !strings.Contains(kubeVirtContainerImageOutputd2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Disk to mirror when kubeVirtContainer set to false has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed, when kubeVirtContainer is set to false")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2m, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror when kubeVirtContainer set to false still failed, retrying...")
				return false, nil
			}
			if !strings.Contains(kubeVirtContainerImageOutputm2m, "kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f7f96da0be48b0010bcc45caec160409cbdbc50c15e3cf5f47abfa6203498c3b ]") {
				e2e.Logf("Mirror to mirror when kubeVirtContainer set to false has been completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed, when kubevirtContainer is set to false")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75438-Validate oc-mirror does not error out when kubeVirtContainer is set to true for a release that does not contain this image [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case75438"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75438.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		var kubeVirtContainerImageOutputm2d string
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2d, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("Mirror2disk when kubeVirtContainer set to true for a release that does not have this image failing, retrying...")
				return false, nil
			}
			return true, nil
		})
		o.Expect(kubeVirtContainerImageOutputm2d).To(o.ContainSubstring("Success collecting release quay.io/openshift-release-dev/ocp-release:4.19.17-x86_64"))
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but the mirror2disk still failed, when kubeVirtContainer set to true for a release that does not have this image")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		var kubeVirtContainerImageOutputd2m string
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputd2m, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("Disk2mirror when kubeVirtContainer set to true for a release that does not have this image is still failing, retrying...")
				return false, nil
			}
			return true, nil
		})
		o.Expect(kubeVirtContainerImageOutputd2m).To(o.ContainSubstring("kubeVirtContainer set to true [ including : quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:"))
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but the disk2mirror still failed, when kubeVirtContainer set to true for a release that does not have this image")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("~/.oc-mirror/")
		var kubeVirtContainerImageOutputm2m string
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			kubeVirtContainerImageOutputm2m, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName+"/m2m", "--workspace", "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror when kubeVirtContainer set to true that does not contain this image still failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		o.Expect(kubeVirtContainerImageOutputm2m).To(o.ContainSubstring("Success collecting release quay.io/openshift-release-dev/ocp-release:4.19.17-x86_64"))
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed, when kubevirtContainer set to true for a release that does not have this image")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Low-72920-support head-only for catalog [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case72920"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-72920.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "15G", "/var/lib/registry")

		compat_otp.By("Start m2d")
		m2dOutput := ""
		waitErr := wait.PollUntilContextTimeout(context.Background(), 60*time.Second, 300*time.Second, true, func(ctx context.Context) (bool, error) {
			m2dOutput, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")
		o.Expect(m2dOutput).To(o.ContainSubstring("images to copy "))
		compat_otp.By("Start d2m")
		waitErr = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			d2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			if !strings.Contains(d2mOutput, "images to copy ") {
				e2e.Logf("Failed to find the image num")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		compat_otp.By("Start m2m")
		waitErr = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {
			m2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname+"/m2m", "docker://"+serInfo.serviceName+"/m2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirro2mirror failed, retrying...")
				return false, nil
			}
			if !strings.Contains(m2mOutput, "images to copy ") {
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-75422-Verify Skip deletion of operator catalog image in delete feature [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case75422"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75422.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "config-75422-delete.yaml")

		compat_otp.By("Skopeo oci to localhost")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://icr.io/cpopen/ibm-bts-operator-catalog@sha256:866f0212eab7bc70cc7fcf7ebdbb4dfac561991f6d25900bd52f33cd90846adf oci://%s  --remove-signatures --insecure-policy", dirname+"/ibm-catalog")
		e2e.Logf(command)
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr = wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for skip deletion of operator catalog image in delete feature failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for skip deletion of operator catalog image in delete feature failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror for skip deletion of operator catalog image in delete feature failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for skip deletion of operator catalog image in delete feature failed")

		compat_otp.By("Generate delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "--generate", "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--src-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Validate delete-images yaml file does not contain any thing with respect to catalog index image")
		deleteImagesYamlOutput, err := exec.Command("bash", "-c", fmt.Sprintf("cat %s", dirname+"/working-dir/delete/delete-images.yaml")).Output()
		if err != nil {
			e2e.Failf("Error is %v", err)
		}
		e2e.Logf("deleteImagesYamlOutput is %s", deleteImagesYamlOutput)

		catalogIndexDetails := []string{"registry.redhat.io/redhat/redhat-operator-index:v4.19", "registry.redhat.io/redhat/certified-operator-index:v4.19", "registry.redhat.io/redhat/community-operator-index:v4.19", "ibm-catalog"}
		for _, catalogIndex := range catalogIndexDetails {
			o.Expect(deleteImagesYamlOutput).ShouldNot(o.ContainSubstring(catalogIndex), "UnExpected Catalog Index Found")
		}

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-73791-verify blockedImages feature for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case73791"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-73791.yaml")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "20G", "/var/lib/registry")

		compat_otp.By("Verify blockedImages feature for mirror2disk")
		waitErr := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk still failed")

		compat_otp.By("Verify blockedImages feature for disk2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName+"/d2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror still failed")

		compat_otp.By("Verify blockedImages feature for mirror2mirror")
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName+"/m2m", "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-76469-Verify Creating release signature configmap with oc-mirror v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case76469"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76469.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for creating release signature configmap with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for creating release signature configmap with oc-mirror v2 failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror for creating release signature configmap with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for creating release signature configmap with oc-mirror v2 failed")

		// Validate if the content from the signature configmap in cluster-resources directory and signature directory matches
		validateConfigmapAndSignatureContent(oc, dirname, "4.16.0")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case76469m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror for creating release signature configmap with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for creating release signature configmap with oc-mirror v2 still failed")

		// Validate if the content from the signature configmap in cluster-resources directory and signature directory matches
		validateConfigmapAndSignatureContent(oc, dirnameM2M, "4.16.0")

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-76596-oc-mirror should not GenerateSignatureConfigMap when not mirror the release images [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case76596"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76596.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for should not generate signature configmap when not mirror the release images  with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for should not generate signature configmap when not mirror the release images  with oc-mirror v2 failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			d2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(d2mOutput, "signature files not found, could not generate signature configmap") {
				e2e.Failf("Signature Configmaps are being generated when nothing related to platform is set in the isc which is not expected")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case76469m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			m2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(m2mOutput, "signature files not found, could not generate signature configmap") {
				e2e.Failf("ignature Configmaps are being generated when nothing related to platform is set in the isc which is not expected")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but mirror2mirror for should not generate signature configmap when not mirror the release images with oc-mirror v2 failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-76489-oc-mirror should fail when the cincinnati API has errors [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case76489"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76489.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-76596.yaml")

		// Set UPDATE_URL_OVERRIDE to a site that does not work
		compat_otp.By("Set UPDATE_URL_OVERRIDE to a site that does not work")
		defer os.Unsetenv("UPDATE_URL_OVERRIDE")
		err = os.Setenv("UPDATE_URL_OVERRIDE", "https://a-site-that-does-not-work")
		if err != nil {
			e2e.Failf("Error setting environment variable:", err)
		}

		// Verify that the environment variable is set
		e2e.Logf("UPDATE_URL_OVERRIDE: %s", os.Getenv("UPDATE_URL_OVERRIDE"))

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		m2dOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
		o.Expect(err).To(o.HaveOccurred())
		if matched, _ := regexp.Match("ERROR"+".*"+"RemoteFailed:"+".*"+"lookup a-site-that-does-not-work"+".*"+"no such host", []byte(m2dOutput)); !matched {
			e2e.Failf("Do not see the expected output while doing mirror2disk\n")
		}

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		m2mOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).To(o.HaveOccurred())
		if matched, _ := regexp.Match("ERROR"+".*"+"RemoteFailed:"+".*"+"lookup a-site-that-does-not-work"+".*"+"no such host", []byte(m2mOutput)); !matched {
			e2e.Failf("Do not see the expected output while doing mirror2disk\n")
		}

		// Unset the update_url_override
		err = os.Unsetenv("UPDATE_URL_OVERRIDE")
		if err != nil {
			e2e.Failf("Error unsetting environment variable: %v", err)
		}

		// Verify that the environment variable is unset
		e2e.Logf("UPDATE_URL_OVERRIDE: %s", os.Getenv("UPDATE_URL_OVERRIDE"))

		compat_otp.By("Start mirror2mirror unset UPDATE_URL_OVERRIDE")
		waitErr := wait.PollImmediate(300*time.Second, 20*time.Minute, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror after unsetting the UPDATE_URL_OVERRIDE for oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but mirror2mirror after unsetting the UPDATE_URL_OVERRIDE for oc-mirror v2 failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-76597-oc-mirror throws error when performing delete operation with --generate [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case76597"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76597.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "config-76597-delete.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for performing delete operatiorn with --generate failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk but performing delete operation with --generate failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror for performing delete operation with --generate failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for performing delete operation with --generate failed")

		compat_otp.By("Generate delete image file")
		dirnameDelete := "/tmp/case76597delete"
		defer os.RemoveAll(dirnameDelete)
		err = os.MkdirAll(dirnameDelete, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "--generate", "--workspace", "file://"+dirnameDelete, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-77060-support to mirror helm for oc-mirror v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case77060"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-77060.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for mirroring helm chars with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for mirroring helm charts with oc-mirror v2 failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			disk2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for mirroring helm charts with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(disk2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(disk2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via disk2mirror completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for mirroring helm charts with oc-mirror v2 failed")

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case77060m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirror2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for helm chart mirroring via oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirror2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(mirror2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via mirror2mirror completed successfully")
				return true, nil
			}
			return false, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for mirroring helm charts with oc-mirror v2 still failed")
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-77061-support the delete helm for v2 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case77061"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-77060.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-77061.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for mirroring helm chars with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for mirroring helm charts with oc-mirror v2 failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			disk2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for mirroring helm charts with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(disk2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(disk2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via disk2mirror completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for mirroring helm charts with oc-mirror v2 failed")

		compat_otp.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Execute delete with out force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case77061m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirror2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for helm chart mirroring via oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirror2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(mirror2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via mirror2mirror completed successfully")
				return true, nil
			}
			return false, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for mirroring helm charts with oc-mirror v2 still failed")

		compat_otp.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Execute delete with out force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-77693-support the delete helm for v2 with --force-cache-delete=true [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case77061"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-77060.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-77061.yaml")

		compat_otp.By("Start mirror2disk")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk for mirroring helm chars with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk for mirroring helm charts with oc-mirror v2 failed, retrying...")

		compat_otp.By("Start disk2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			disk2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--from", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror for mirroring helm charts with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(disk2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(disk2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via disk2mirror completed successfully")
				return true, nil
			}
			return false, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror for mirroring helm charts with oc-mirror v2 failed")

		compat_otp.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("version", "--v2", "--output", "yaml").Output()
		compat_otp.By("Execute delete with force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--force-cache-delete=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		dirnameM2M := "/tmp/case77061m2m"
		defer os.RemoveAll(dirnameM2M)
		err = os.MkdirAll(dirnameM2M, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirror2mirrorOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for helm chart mirroring via oc-mirror v2 failed, retrying...")
				return false, nil
			}
			if strings.Contains(mirror2mirrorOutput, "idms-oc-mirror.yaml") && strings.Contains(mirror2mirrorOutput, "itms-oc-mirror.yaml") {
				e2e.Logf("Helm chart mirroring via mirror2mirror completed successfully")
				return true, nil
			}
			return false, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror for mirroring helm charts with oc-mirror v2 still failed")

		compat_otp.By("Generete delete image file")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirnameM2M, "--authfile", dirname+"/.dockerconfigjson", "--generate").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Dump delete-images.yaml")
		delFile := dirnameM2M + "/working-dir/delete/delete-images.yaml"
		content, err := os.ReadFile(delFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("delete-images.yaml: %s", string(content))

		compat_otp.By("Execute delete with force-cache-delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", delFile, "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false", "--force-cache-delete=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-79217-v2 should able to delete operator images from oci catalogs mirrored with v1 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case79217"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-79217.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-79217-1.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-79217.yaml")

		compat_otp.By("Use skopoe copy catalogsource to localhost")
		skopeExecute(fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.16 --remove-signatures  --insecure-policy oci://%s", dirname+"/redhat-operator-index"))

		compat_otp.By("Start mirror2mirror for v1")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		defer os.RemoveAll("oc-mirror-workspace")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dest-use-http", "--v1").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for v1 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror failed for v1")

		compat_otp.By("Start mirror2disk with v2")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk2 for with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk with oc-mirror v2 failed")

		compat_otp.By("Generete delete image file using v2 for v1 images")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate", "--delete-v1-images").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Execute delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Critical-79215-oc mirror v2 to support creating clustercatalog [Serial]", func() {
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-79215.yaml")
		sa79215 := filepath.Join(ocmirrorBaseDir, "sa-79215.yaml")
		ceFile79215 := filepath.Join(ocmirrorBaseDir, "ceFile-79215.yaml")

		dirname := "/tmp/case79215"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		compat_otp.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-79215")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-79215")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirror2mirror")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror still failed")

		compat_otp.By("Create the catalogsource, idms and itms")
		defer operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "delete")
		operateCSAndMs(oc, dirname+"/working-dir/cluster-resources", "create")
		compat_otp.By("Check for the catalogsource pod status")
		assertPodOutput(oc, "olm.catalogSource=cs-redhat-operator-index-v4-19", "openshift-marketplace", "Running")

		compat_otp.By("Check if cluster catalog has been created")
		clusterCatalogExist, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercatalog").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterCatalogExist).To(o.ContainSubstring("cc-redhat-operator-index-v4-19"))

		compat_otp.By("Create namespace, sa, clusterRole & clusterRoleBinding")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "ns-79215").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "ns-79215").Execute()

		compat_otp.By("Create sa, clusterRole & clusterRoleBinding")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sa79215).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sa79215).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create clusterExtension")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", ceFile79215).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", ceFile79215).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Check clusterExtension got created")
		var clusterExtensionExist string
		waitErr = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			clusterExtensionExist, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterExtension", "extension-79215", "-n", "ns-79215").Output()
			if err != nil {
				e2e.Logf("Retreving ClusterExtension failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		e2e.Logf("clusterExtensionExist is %s", clusterExtensionExist)
		o.Expect(clusterExtensionExist).To(o.ContainSubstring("extension-79215"))
		o.Expect(clusterExtensionExist).To(o.ContainSubstring("volsync-product.v0.14.0"))

		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but cluster extension has not been installed yet")

		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but cluster extension has not been installed yet")

		compat_otp.By("Get all the pods in the namespace are running")
		waitForAvailableRsRunning(oc, "deploy", "volsync-controller-manager", "ns-79215", "1")

		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("all", "-n", "ns-79215").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("output is %v", output)
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-79452-v2 be able to delete the image mirrored by oc-mirror plugin v1 [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case79452"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-79452-v1.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-79452-v2.yaml")
		imageDeleteYamlFileF := filepath.Join(ocmirrorBaseDir, "delete-config-79452.yaml")

		compat_otp.By("Start mirror2mirror for v1")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll("~/.oc-mirror.log")
		defer os.RemoveAll("oc-mirror-workspace")
		waitErr := wait.PollImmediate(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dest-use-http", "--v1").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror for v1 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2mirror failed for v1")

		compat_otp.By("Start mirror2disk with v2")
		defer os.RemoveAll("~/.oc-mirror/")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "file://"+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk2 for with oc-mirror v2 failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk with oc-mirror v2 failed")

		compat_otp.By("Generete delete image file using v2 for v1 images")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--config", imageDeleteYamlFileF, "docker://"+serInfo.serviceName, "--v2", "--workspace", "file://"+dirname, "--authfile", dirname+"/.dockerconfigjson", "--generate", "--delete-v1-images").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Execute delete")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("delete", "--delete-yaml-file", dirname+"/working-dir/delete/delete-images.yaml", "docker://"+serInfo.serviceName, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-79408-Validate cache dir flags works fine for m2d,d2m workflow [Serial]", func() {
		dirname := "/tmp/case79408"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-79408.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		compat_otp.By("Start mirro2disk with --cachedir flag")
		waitErr := wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk with --cache-dir failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk with --cache-dir flag have not been completed")

		compat_otp.By("Start disk2mirror with --cachedir flag")
		waitErr = wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+dirname, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The disk2mirror with --cache-dir flag failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror for --cache-dir flag still failed")

		// Validate if /tmp/79408 has the oc mirror cache folder
		compat_otp.By("Validate if --cache-dir flag has been respected")
		checkCacheDirPresent, err := exec.Command("bash", "-c", fmt.Sprintf("ls -al %s", dirname)).Output()
		e2e.Logf("cache dir content  is %s", checkCacheDirPresent)
		if err != nil {
			e2e.Logf("Error reading cache dir directory", err)
		}
		o.Expect(strings.Contains(string(checkCacheDirPresent), ".oc-mirror")).To(o.BeTrue())
	})

	g.It("Author:knarra-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-79409-Validate cache dir flags works fine for m2m workflow [Serial]", func() {
		dirname := "/tmp/case79409"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-79408.yaml")

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		compat_otp.By("Start mirro2mirror with --cachedir flag")
		waitErr := wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+dirname, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Execute()
			if err != nil {
				e2e.Logf("The mirror2mirror with --cache-dir flag failed, retrying...")
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror for --cache-dir flag still failed")

		// Validate if /tmp/79409 has the oc mirror cache folder
		compat_otp.By("Validate if --cache-dir flag has been respected during m2m workflow")
		checkCacheDirPresent, err := exec.Command("bash", "-c", fmt.Sprintf("ls -al %s", dirname)).Output()
		e2e.Logf("cache dir content  is %s", checkCacheDirPresent)
		if err != nil {
			e2e.Logf("Error reading cache dir directory", err)
		}
		o.Expect(strings.Contains(string(checkCacheDirPresent), ".oc-mirror")).To(o.BeTrue())
	})

	g.It("Author:ngavali-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-83582-Verify that BlockedImages excludes the provided Images [Serial]", func() {
		dirname := "/tmp/case83582"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-83582.yaml")

		compat_otp.By("Start mirror2Disk with --dry-run flag")
		waitErr := wait.Poll(300*time.Second, 3600*time.Second, func() (bool, error) {
			mirrorToDiskOutput, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+dirname, "--dry-run", "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk with --dry-run failed, retrying...")
				return false, nil
			}
			// Validate that mapping.txt is created
			if strings.Contains(mirrorToDiskOutput, "dry-run/missing.txt") && strings.Contains(mirrorToDiskOutput, "dry-run/mapping.txt") {
				e2e.Logf("Mirror to Disk dry run has been completed successfully")
				return true, nil
			}
			return false, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk with --dry-run flag have not been completed")

		//Validate that BlockedImages are not present in the mapping.txt
		compat_otp.By("Check Blocked Images in mapping.txt")

		// Defined blocked images to search for
		blockedImages := []string{
			"aws",
			"gcp",
			"ibm",
			"azure",
			"openstack",
		}

		mappingFilePath := dirname + "/dry-run/mapping.txt"

		// Check that each blocked image is NOT present in mapping.txt
		for _, blockedImage := range blockedImages {
			grepCmd := fmt.Sprintf("grep -q '%s' '%s'", blockedImage, mappingFilePath)
			_, err := exec.Command("bash", "-c", grepCmd).Output()

			// If grep exit code is 0, it means the blocked image was found (should fail test)
			if err == nil {
				e2e.Failf("Blocked image '%s' was found in mapping.txt when it should be excluded", blockedImage)
			}

		}

		e2e.Logf("All blocked images are correctly excluded from mapping.txt")

	})

	//OCP-83849 - [OCPSTRAT-1967] mirror helm can discover Operand images referenced via Environment Variables
	g.It("Author:maxu-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-83849-Mirror helm can discover Operand images referenced via Environment Variables [Serial]", func() {
		dirname := "/tmp/case83849"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads/case83849")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-83849.yaml")

		_, errStat := os.Stat(ocmirrorBaseDir + "/test-mirror-helm-0.3.0.tgz")
		o.Expect(errStat).NotTo(o.HaveOccurred())
		// in order to find the helm tgz, change the current dir
		err = os.Chdir(ocmirrorBaseDir)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		diskPath := dirname + "/test"

		compat_otp.By("Start mirror2disk ...")
		output := ""
		waitErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed ..")
				return false, err
			}
			return true, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk has not been completed")

		// Validate the output
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("Success copying quay.io/nginx/nginx-ingress:latest"))
		o.Expect(output).To(o.ContainSubstring("Success copying quay.io/prometheus/prometheus:latest"))
		o.Expect(output).To(o.ContainSubstring("2 / 2 helm images mirrored successfully"))
		e2e.Logf("mirrorToDisk PASS")
		// Validate the tar file
		archiveFilePath := diskPath + "/mirror_000001.tar"
		archiveFile, err := os.Stat(archiveFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		isValid := archiveFile.Size() > 0
		o.Expect(isValid).Should(o.BeTrue())
		e2e.Logf("Created valid %s.", archiveFilePath)

		compat_otp.By("Start disk2mirror ...")

		compat_otp.By("	Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed ..")
				return false, err
			}
			return true, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but disk2mirror has not been completed")

		// Validate the output
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("Success copying quay.io/nginx/nginx-ingress:latest"))
		o.Expect(output).To(o.ContainSubstring("Success copying quay.io/prometheus/prometheus:latest"))
		o.Expect(output).To(o.ContainSubstring("2 / 2 helm images mirrored successfully"))
		o.Expect(output).To(o.ContainSubstring("working-dir/cluster-resources/itms-oc-mirror.yaml file created"))
		itmsFile := diskPath + "/working-dir/cluster-resources/itms-oc-mirror.yaml"
		_, err = os.Stat(itmsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("disk2mirror PASS")

		compat_otp.By("Start mirror2mirror ...")
		workPath := dirname + "/work"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--workspace", "file://"+workPath, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The mirror2mirror failed ..")
				return false, err
			}
			return true, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2mirror has not been completed")

		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToMirror"))
		o.Expect(output).To(o.ContainSubstring("Success copying quay.io/nginx/nginx-ingress:latest"))
		o.Expect(output).To(o.ContainSubstring("Success copying quay.io/prometheus/prometheus:latest"))
		o.Expect(output).To(o.ContainSubstring("2 / 2 helm images mirrored successfully"))
		o.Expect(output).To(o.ContainSubstring("working-dir/cluster-resources/itms-oc-mirror.yaml file created"))
		itmsFile = workPath + "/working-dir/cluster-resources/itms-oc-mirror.yaml"
		_, err = os.Stat(itmsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("mirror2mirror PASS")

		e2e.Logf("PASS")
	})

	//OCP-83864 - [OCPSTRAT-1967] mirror helm via Environment Variables with the invalid helm package file
	g.It("Author:maxu-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-83864-Mirror helm via Environment Variables with the invalid helm package file [Serial]", func() {
		dirname := "/tmp/case83864"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads/case83864")
		isc_no := filepath.Join(ocmirrorBaseDir, "isc_no.yaml")

		diskPath := dirname + "/no"

		compat_otp.By("Start mirror2disk without the helm package ...")
		output := ""
		waitErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", isc_no, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err == nil {
				e2e.Logf("The mirror2disk failed ..")
				return false, nil
			}
			return true, err
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "test_mirror-helm-no.tgz: no such file or directory")

		// Validate the output
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("[Executor] collection error: failed to load test_mirror-helm-no.tgz: stat test_mirror-helm-no.tgz: no such file or directory"))
		e2e.Logf("with not existed helm package mirror2Disisk PASS")

		compat_otp.By("Start mirror2disk with an empty helm package ...")
		err = os.Chdir(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		emptypackage := dirname + "/test_mirror-helm-empty.tgz"
		file, err := os.Create(emptypackage)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer file.Close()
		archiveFile, errStat := os.Stat(emptypackage)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Clean up the created file
		defer os.Remove(emptypackage)

		o.Expect(errStat).NotTo(o.HaveOccurred())
		isEmpty := archiveFile.Size() == 0
		o.Expect(isEmpty).Should(o.BeTrue())
		e2e.Logf("Created an empty helm archive %s.", emptypackage)

		diskPath = dirname + "/empty"

		isc_empty := filepath.Join(ocmirrorBaseDir, "isc_empty.yaml")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", isc_empty, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err == nil {
				e2e.Logf("The mirror2disk failed ..")
				return false, nil
			}
			return true, err
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "does not appear to be a gzipped archive")

		// Validate the output
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("[Executor] collection error: failed to load test_mirror-helm-empty.tgz: file 'test_mirror-helm-empty.tgz' does not appear to be a gzipped archive; got 'application/octet-stream'"))
		e2e.Logf("with empty helm package mirror2Disisk PASS PASS")

		// in order to find the helm tgz, change the current dir
		compat_otp.By("Start mirror2disk with invalid Environment Variables ...")
		err = os.Chdir(ocmirrorBaseDir)
		o.Expect(err).NotTo(o.HaveOccurred())

		isc_err := filepath.Join(ocmirrorBaseDir, "isc_err.yaml")
		diskPath = dirname + "/err"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", isc_err, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err == nil {
				e2e.Logf("The mirror2disk failed ..")
				return false, nil
			}
			return true, err
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "some errors occurred during the mirroring")

		// Validate the output
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("[Worker] error mirroring image quay.io/nginx/nginx-ingres:latest error: initializing source docker://quay.io/nginx/nginx-ingres:latest: reading manifest latest in quay.io/nginx/nginx-ingres: unauthorized: access to the requested resource is not authorized"))
		o.Expect(output).To(o.ContainSubstring("[Worker] error mirroring image quay.io/prometheus/prometheu:latest error: initializing source docker://quay.io/prometheus/prometheu:latest: reading manifest latest in quay.io/prometheus/prometheu: unauthorized: access to the requested resource is not authorized"))
		o.Expect(output).To(o.ContainSubstring("0 / 2 helm images mirrored: Some helm images failed to be mirrored - please check the logs"))

		e2e.Logf("with error helm package mirror2Disisk PASS PASS")

		e2e.Logf("PASS")
	})

	//OCP-83875 - [CLID-387] Verify credentials, hostname, and certs before populating the cache during disk to mirror operations
	g.It("Author:maxu-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-83875-Verify credentials, hostname, and certs before populating the cache during disk to mirror operations [Serial]", func() {
		dirname := "/tmp/case83875"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-83875.yaml")

		diskPath := dirname + "/test"

		compat_otp.By("**1. setup mirror2disk ...")
		output := ""
		waitErr := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The mirror2disk failed")
				return false, err
			}
			return true, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk has not been completed")

		// Validate the output
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("5 / 5 operator images mirrored successfully"))
		e2e.Logf("mirrorToDisk PASS")

		// Validate the tar file
		archiveFilePath := diskPath + "/mirror_000001.tar"
		archiveFile, err := os.Stat(archiveFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		isValid := archiveFile.Size() > 0
		o.Expect(isValid).Should(o.BeTrue())
		e2e.Logf("Created valid %s.", archiveFilePath)

		compat_otp.By("**2. Verify missing registry...")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://localhost", "--dest-tls-verify=false", "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed")
				return true, err
			}
			return false, nil
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "Unknow reason")

		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("connect: connection refused"))

		e2e.Logf("Verify missing registry PASS")

		compat_otp.By("	Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app ...")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)

		compat_otp.By("**3. Verify registry port...")
		errPort := "9999"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName+":"+errPort, "--dest-tls-verify=false", "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed")
				return true, err
			}
			return false, nil
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "Unknow reason")

		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("failed to authenticate: pinging container registry %s:", serInfo.serviceName+":"+errPort))
		e2e.Logf("Verify registry port PASS")

		compat_otp.By("**4. Verify hostname ...")
		errHost := "invalid.host"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://"+errHost, "--dest-tls-verify=false", "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed")
				return true, err
			}
			return false, nil
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "Unknow reason")

		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("no such host"))
		e2e.Logf("Verify hostname PASS")

		// compat_otp.By("**5. Verify auth file...")
		// emptyAuthFile := dirname + "/invalidAuth"
		// emptyContent := `{"auths": {"registry.redhat.io": {"auth":"dXNlcjpwc3cK"}}}`
		// err = os.WriteFile(emptyAuthFile, []byte(emptyContent), 0644)
		// o.Expect(err).NotTo(o.HaveOccurred())
		// val := os.Getenv("XDG_CONFIG_HOME")
		// f1 := val + "/containers/auth.json"
		// val = os.Getenv("HOME")
		// f2 := val + "/.docker/config.json"
		// f3 := val + "/.config/containers/auth.json"
		// f4 := val + "/.dockercfg"
		// val = os.Getenv("DOCKER_CONFIG")
		// f5 := val + "/config.json"
		// val = os.Getenv("XDG_RUNTIME_DIR")
		// f6 := val + "/containers/auth.json"
		// files := []string{f1, f2, f3, f4, f5, f6}
		// for _, f := range files {
		// 	_, err = os.Stat(f)
		// 	e2e.Logf("Checking the auth file: %s", f)
		// 	if err != nil {
		// 		e2e.Logf("error: %v", err)
		// 	} else {
		// 		str, err := os.ReadFile(f)
		// 		e2e.Logf(string(str))
		// 		o.Expect(err).NotTo(o.HaveOccurred())
		// 	}
		// 	o.Expect(err).To(o.HaveOccurred())
		// }

		// waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		// 	output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName, "--dest-tls-verify=false", "--cache-dir="+dirname, "--v2", "--authfile", emptyAuthFile, "--log-level", "debug").Output()
		// 	if err != nil {
		// 		e2e.Logf("The disk2mirror failed")
		// 		return true, err
		// 	}
		// 	return false, nil
		// })
		// o.Expect(err).To(o.HaveOccurred())
		// compat_otp.AssertWaitPollWithErr(waitErr, "Unknow reason")

		// o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		// o.Expect(output).To(o.ContainSubstring("[ERROR]"))
		// o.Expect(output).To(o.ContainSubstring("failed to authenticate: pinging container registry %s: received unexpected HTTP status: 503 Service Unavailable", serInfo.serviceName))
		// e2e.Logf("Verify auth file PASS")

		compat_otp.By("**6. Verify cert file...")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed")
				return true, err
			}
			return false, nil
		})
		o.Expect(err).To(o.HaveOccurred())
		compat_otp.AssertWaitPollWithErr(waitErr, "Unknow reason")

		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("tls: failed to verify certificate: x509: certificate signed by unknown authority"))
		e2e.Logf("Verify cert PASS")

		compat_otp.By("**7. All parameters are valid...")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", dirname+"/.dockerconfigjson", "--dest-tls-verify=false").Output()
			if err != nil {
				e2e.Logf("The disk2mirror failed: %v", err)
				return false, err
			}
			return true, nil
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but diskToMirror has not been completed")

		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("5 / 5 operator images mirrored successfully"))
		e2e.Logf("All parameters are valid PASS")

		e2e.Logf("PASS")
	})

	g.It("Author:maxu-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Medium-84007-Investigate oc-mirror v2 with olm v1 operators [Serial]", g.SpecTimeout(120*time.Minute), func(ctx g.SpecContext) {
		// whether test m2d & d2m
		const justM2M = true
		dirname := "/tmp/case84007"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads/case84007")
		operatorsFile := filepath.Join(ocmirrorBaseDir, "operators.lst")
		file, err := os.Open(operatorsFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer file.Close()

		var operators []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			op := strings.TrimSpace(scanner.Text())
			if len(op) > 0 {
				operators = append(operators, op)
			}
		}
		o.Expect(scanner.Err()).NotTo(o.HaveOccurred(), "Should be no errors scanning the file")

		templatePath := filepath.Join(ocmirrorBaseDir, "config-84007.yaml")
		templateBytes, err := os.ReadFile(templatePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		icsTemplateString := string(templateBytes)

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := filepath.Join(dirname, ".dockerconfigjson")
		o.Expect(authFile).To(o.BeAnExistingFile())

		compat_otp.By("	Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app ...")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)

		var passTests []string
		var testErrors []error
		var wg sync.WaitGroup
		concurrencyLimit := min(runtime.NumCPU(), 8)
		e2e.Logf("The currency is %d ", concurrencyLimit)
		semaphore := make(chan struct{}, concurrencyLimit)
		var mu sync.Mutex
		e2e.Logf("========== Total %d operators will be tested %v", len(operators), operators)
		for i, operator := range operators {
			if operator == "" {
				continue
			}
			wg.Add(1)
			semaphore <- struct{}{}
			go func(op string, idx int) {
				compat_otp.By(fmt.Sprintf("test mirror %s", op))
				// GinkgoRecover is crucial for handling panics (like failed assertions) inside goroutines
				defer g.GinkgoRecover()
				defer wg.Done()
				defer func() { <-semaphore }()
				compat_otp.By(fmt.Sprintf("-----test mirror %s -------", op))

				start := time.Now()
				runDir := filepath.Join(dirname, op)
				if err := os.Mkdir(runDir, 0755); err != nil {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' failed to create run directory: %w", op, err))
					mu.Unlock()
					return
				}
				defer os.RemoveAll(runDir)

				iscFile := filepath.Join(runDir, "isc.yaml")
				finalContent := strings.Replace(icsTemplateString, "${package}", op, 1)
				if err := os.WriteFile(iscFile, []byte(finalContent), 0644); err != nil {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' failed to write isc.yaml: %w", op, err))
					mu.Unlock()
					return
				}

				destRegistryURL := fmt.Sprintf("docker://%s/%s", serInfo.serviceName, op)
				port := fmt.Sprintf("%s%02d", "550", i)
				used := []int{8, 10, 18, 20}
				if slices.Contains(used, i) {
					port = fmt.Sprintf("%s%02d", "560", i)
				}
				var output string
				var mirrorErr, waitErr error
				files := []string{"idms-oc-mirror.yaml", "cs-redhat-operator-index-v4-19.yaml", "cc-redhat-operator-index-v4-19.yaml"}
				if !justM2M {
					compat_otp.By(op + " 1. mirro2disk ......")
					diskPath := filepath.Join(runDir, "test")

					waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 90*time.Minute, true, func(ctx context.Context) (bool, error) {
						output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", iscFile, "file://"+diskPath, "--cache-dir="+runDir, "--v2", "--authfile", authFile, "--port", port).Output()
						return mirrorErr == nil, mirrorErr
					})
					if waitErr != nil {
						mu.Lock()
						testErrors = append(testErrors, fmt.Errorf("'%s' mirror2Disk timed out after 90m: %w. Last error: %v", op, waitErr, mirrorErr))
						mu.Unlock()
						return
					}
					if strings.Contains(output, "[ERROR]") || !strings.Contains(output, "operator images mirrored successfully") {
						mu.Lock()
						testErrors = append(testErrors, fmt.Errorf("'%s' mirror2disk failed", op))
						mu.Unlock()
						return
					}

					// Validate the tar file
					archiveFilePath := filepath.Join(diskPath, "/mirror_000001.tar")
					o.Expect(archiveFilePath).To(o.BeAnExistingFile())
					compat_otp.By(op + " mirro2disk PASS.")

					compat_otp.By(op + " 2. disk2mirror ......")
					waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 90*time.Minute, true, func(ctx context.Context) (bool, error) {
						output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", iscFile, "--from", "file://"+diskPath, destRegistryURL, "--cache-dir="+runDir, "--v2", "--authfile", authFile,
							"--dest-tls-verify=false", "--port", port).Output()
						return err == nil, mirrorErr
					})
					if waitErr != nil {
						mu.Lock()
						testErrors = append(testErrors, fmt.Errorf("'%s' disk2mirror timed out after 90m: %w. Last error: %v", op, waitErr, mirrorErr))
						mu.Unlock()
						return
					}
					if strings.Contains(output, "[ERROR]") || !strings.Contains(output, "working-dir/cluster-resources/idms-oc-mirror.yaml file created") ||
						!strings.Contains(output, "working-dir/cluster-resources/cs-redhat-operator-index-v4-19.yaml file created") ||
						!strings.Contains(output, "working-dir/cluster-resources/cc-redhat-operator-index-v4-19.yaml file created") {
						mu.Lock()
						testErrors = append(testErrors, fmt.Errorf("'%s' disk2mirror failed", op))
						mu.Unlock()
						return
					}

					for _, f := range files {
						filePath := filepath.Join(diskPath, "working-dir/cluster-resources/", f)
						e2e.Logf("Checking %s...", filePath)
						o.Expect(filePath).To(o.BeAnExistingFile())
					}
					compat_otp.By(op + " disk2mirror PASS.")
				}

				compat_otp.By(op + " 3. mirro2mirror ......")
				workPath := filepath.Join(runDir, "work")
				waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 90*time.Minute, true, func(ctx context.Context) (bool, error) {
					output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", iscFile, "--workspace", "file://"+workPath, destRegistryURL, "--cache-dir="+runDir, "--v2", "--authfile", authFile, "--dest-tls-verify=false", "--port", port).Output()
					return err == nil, mirrorErr
				})

				if waitErr != nil {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' mirror2mirror timed out after 90m: %w. Last error: %v", op, waitErr, mirrorErr))
					mu.Unlock()
					return
				}
				if strings.Contains(output, "[ERROR]") || !strings.Contains(output, "working-dir/cluster-resources/idms-oc-mirror.yaml file created") ||
					!strings.Contains(output, "working-dir/cluster-resources/cs-redhat-operator-index-v4-19.yaml file created") ||
					!strings.Contains(output, "working-dir/cluster-resources/cc-redhat-operator-index-v4-19.yaml file created") {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' mirror2mirror failed", op))
					mu.Unlock()
					return
				}

				for _, f := range files {
					filePath := filepath.Join(workPath, "working-dir/cluster-resources/", f)
					e2e.Logf("Checking %s...", filePath)
					o.Expect(filePath).To(o.BeAnExistingFile())
				}
				cacheFolder := filepath.Join(runDir, ".oc-mirror")
				if err := os.RemoveAll(cacheFolder); err != nil {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' failed to remove the cache folder %s: %w", op, cacheFolder, err))
					mu.Unlock()
				}
				compat_otp.By(op + " mirro2mirror PASS.")

				compat_otp.By(op + " Validate the tags")

				waitErr = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
					rhOperatorUri := fmt.Sprintf("https://%s/v2/%s/redhat/redhat-operator-index/tags/list", serInfo.serviceName, op)
					validateTargetcatalogAndTag(rhOperatorUri, "v4.19")
					err = validateRepoTags(serInfo.serviceName, op)
					return err == nil, err
				})

				if waitErr != nil {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' validateRepTags timed out after 5m: %v", op, waitErr))
					mu.Unlock()
					return
				}

				if err != nil {
					mu.Lock()
					testErrors = append(testErrors, fmt.Errorf("'%s' validateRepTags failed: %v", op, err))
					mu.Unlock()
					return
				}

				mu.Lock()
				passTests = append(passTests, op)
				mu.Unlock()

				duration := time.Since(start)
				e2e.Logf("%s Time %d minutes, Test PASS", op, int(duration.Minutes()))
			}(operator, i)
		}
		// --- Wait for all concurrent jobs to complete ---
		e2e.Logf("All jobs launched. Now waiting for all of them to complete...")

		// We use a channel to signal when wg.Wait() is finished.
		// This pattern allows us to use Gomega's Eventually to poll for completion
		// with a timeout, which is the idiomatic way to handle this in Ginkgo.
		allJobsDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(allJobsDone)
		}()

		// Poll for up to * minutes for all jobs to be done.
		// This timeout must be less than the g.SpecTimeout.
		o.Eventually(ctx, allJobsDone, 110*time.Minute, 1*time.Minute).Should(o.BeClosed(), "Not all concurrent jobs finished within the expected timeframe.")

		// --- Final assertions ---
		// After waiting, check if any of the jobs reported an error.
		mu.Lock()
		defer mu.Unlock()
		if len(testErrors) > 0 {
			e2e.Logf("All the Errors: %v", testErrors)
		}
		e2e.Logf("All Pass operators: %s", passTests)
		o.Expect(testErrors).To(o.BeEmpty(), "One or more concurrent operator tests failed.")
		o.Expect(len(passTests) == len(operators)).To(o.BeTrue())

		e2e.Logf("Successfully verified all PASS.")
	})

	g.It("Author:maxu-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-86309-Enabling signature mirroring as default[Serial]", g.SpecTimeout(120*time.Minute), func(ctx g.SpecContext) {
		dirname := "/tmp/case86309"
		defer os.RemoveAll(dirname)
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		iscFile := filepath.Join(ocmirrorBaseDir, "config-86309.yaml")

		err := oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := filepath.Join(dirname, ".dockerconfigjson")
		o.Expect(authFile).To(o.BeAnExistingFile())

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app ...")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)

		compat_otp.By("mirro2mirror ......")
		var output string
		var mirrorErr error
		diskPath := filepath.Join(dirname, "mywork")
		prefix := "/86309"
		waitErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", iscFile, "docker://"+serInfo.serviceName+prefix, "--workspace", "file://"+diskPath, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror has not been completed")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToMirror"))
		o.Expect(output).To(o.ContainSubstring("5 / 5 operator images mirrored successfully"))

		files := []string{"idms-oc-mirror.yaml", "cs-redhat-operator-index-v4-19.yaml", "cc-redhat-operator-index-v4-19.yaml"}
		for _, f := range files {
			filePath := filepath.Join(diskPath, "working-dir/cluster-resources/", f)
			e2e.Logf("Checking %s...", filePath)
			o.Expect(filePath).To(o.BeAnExistingFile())
		}
		e2e.Logf("MirrorToMirror PASS")

		compat_otp.By("Validate the signature")
		validateRepoSignature(serInfo.serviceName, prefix)

	})

	g.It("Author:ngavali-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-88132-Verify targetTag and targetRepo for m2d and d2m workflow [Serial]", g.SpecTimeout(30*time.Minute), func(ctx g.SpecContext) {
		dirname := "/tmp/case88132"
		defer os.RemoveAll(dirname)
		defer os.RemoveAll(".oc-mirror.log")
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := filepath.Join(dirname, ".dockerconfigjson")
		o.Expect(authFile).To(o.BeAnExistingFile())

		ocmirrorBaseDir := testdata.FixturePath("workloads")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)

		var output string
		var mirrorErr error

		// === Sub-test 1: targetTag only ===
		compat_otp.By("Step 1a: targetTag - mirror2disk")
		iscTargetTag := filepath.Join(ocmirrorBaseDir, "config-88132-target-tag.yaml")
		diskPath1 := filepath.Join(dirname, "disk-targetTag")
		waitErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetTag, "file://"+diskPath1, "--cache-dir="+dirname+"/cache-tt", "--v2", "--authfile", authFile).Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d for targetTag timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetTag m2d output: %s", output)

		archiveFile1 := filepath.Join(diskPath1, "mirror_000001.tar")
		archiveStat1, err := os.Stat(archiveFile1)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveStat1.Size() > 0).Should(o.BeTrue())
		e2e.Logf("targetTag m2d PASS - archive: %s", archiveFile1)

		compat_otp.By("Step 1b: targetTag - disk2mirror")
		prefixTargetTag := "/88132targettag"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetTag, "--from", "file://"+diskPath1, "docker://"+serInfo.serviceName+prefixTargetTag,
				"--cache-dir="+dirname+"/cache-tt", "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "d2m for targetTag timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetTag d2m output: %s", output)

		itmsFile1 := filepath.Join(diskPath1, "working-dir/cluster-resources/itms-oc-mirror.yaml")
		o.Expect(itmsFile1).To(o.BeAnExistingFile())

		compat_otp.By("Step 1c: Validate targetTag - ubi8/ubi should have tag v8")
		tagUri1 := "https://" + serInfo.serviceName + "/v2/88132targettag/ubi8/ubi/tags/list"
		validateTargetcatalogAndTag(tagUri1, "v8")
		e2e.Logf("targetTag validation PASS")

		// === Sub-test 2: targetRepo only ===
		compat_otp.By("Step 2a: targetRepo - mirror2disk")
		iscTargetRepo := filepath.Join(ocmirrorBaseDir, "config-88132-target-repo.yaml")
		diskPath2 := filepath.Join(dirname, "disk-targetRepo")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetRepo, "file://"+diskPath2, "--cache-dir="+dirname+"/cache-tr", "--v2", "--authfile", authFile).Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d for targetRepo timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetRepo m2d output: %s", output)

		archiveFile2 := filepath.Join(diskPath2, "mirror_000001.tar")
		archiveStat2, err := os.Stat(archiveFile2)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveStat2.Size() > 0).Should(o.BeTrue())
		e2e.Logf("targetRepo m2d PASS - archive: %s", archiveFile2)

		compat_otp.By("Step 2b: targetRepo - disk2mirror")
		prefixTargetRepo := "/88132targetrepo"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetRepo, "--from", "file://"+diskPath2, "docker://"+serInfo.serviceName+prefixTargetRepo,
				"--cache-dir="+dirname+"/cache-tr", "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "d2m for targetRepo timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetRepo d2m output: %s", output)

		itmsFile2 := filepath.Join(diskPath2, "working-dir/cluster-resources/itms-oc-mirror.yaml")
		o.Expect(itmsFile2).To(o.BeAnExistingFile())

		compat_otp.By("Step 2c: Validate targetRepo - custom-ns/my-ubi8 should have tag latest")
		tagUri2 := "https://" + serInfo.serviceName + "/v2/88132targetrepo/custom-ns/my-ubi8/tags/list"
		validateTargetcatalogAndTag(tagUri2, "latest")
		e2e.Logf("targetRepo validation PASS")

		// === Sub-test 3: targetRepo + targetTag combined ===
		compat_otp.By("Step 3a: targetRepoTag - mirror2disk")
		iscTargetRepoTag := filepath.Join(ocmirrorBaseDir, "config-88132-target-repo-tag.yaml")
		diskPath3 := filepath.Join(dirname, "disk-targetRepoTag")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetRepoTag, "file://"+diskPath3, "--cache-dir="+dirname+"/cache-trt", "--v2", "--authfile", authFile).Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d for targetRepoTag timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetRepoTag m2d output: %s", output)

		archiveFile3 := filepath.Join(diskPath3, "mirror_000001.tar")
		archiveStat3, err := os.Stat(archiveFile3)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveStat3.Size() > 0).Should(o.BeTrue())
		e2e.Logf("targetRepoTag m2d PASS - archive: %s", archiveFile3)

		compat_otp.By("Step 3b: targetRepoTag - disk2mirror")
		prefixTargetRepoTag := "/88132targetrepotag"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetRepoTag, "--from", "file://"+diskPath3, "docker://"+serInfo.serviceName+prefixTargetRepoTag,
				"--cache-dir="+dirname+"/cache-trt", "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "d2m for targetRepoTag timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetRepoTag d2m output: %s", output)

		itmsFile3 := filepath.Join(diskPath3, "working-dir/cluster-resources/itms-oc-mirror.yaml")
		o.Expect(itmsFile3).To(o.BeAnExistingFile())

		compat_otp.By("Step 3c: Validate targetRepoTag - ubi-repo:ubi-v9")
		tagUri3a := "https://" + serInfo.serviceName + "/v2/88132targetrepotag/ubi-repo/tags/list"
		validateTargetcatalogAndTag(tagUri3a, "ubi-v9")
		compat_otp.By("Step 3c: Validate targetRepoTag - nginx-repo:stable-v1")
		tagUri3b := "https://" + serInfo.serviceName + "/v2/88132targetrepotag/nginx-repo/tags/list"
		validateTargetcatalogAndTag(tagUri3b, "stable-v1")
		compat_otp.By("Step 3c: Validate targetRepoTag - fedora-repo:fedora-test")
		tagUri3c := "https://" + serInfo.serviceName + "/v2/88132targetrepotag/fedora-repo/tags/list"
		validateTargetcatalogAndTag(tagUri3c, "fedora-test")
		e2e.Logf("targetRepoTag validation PASS")

		// === Sub-test 4: digest images with targetTag ===
		compat_otp.By("Step 4a: digest - mirror2disk")
		iscDigest := filepath.Join(ocmirrorBaseDir, "config-88132-digest.yaml")
		diskPath4 := filepath.Join(dirname, "disk-digest")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscDigest, "file://"+diskPath4, "--cache-dir="+dirname+"/cache-dig", "--v2", "--authfile", authFile).Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d for digest timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("digest m2d output: %s", output)

		archiveFile4 := filepath.Join(diskPath4, "mirror_000001.tar")
		archiveStat4, err := os.Stat(archiveFile4)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveStat4.Size() > 0).Should(o.BeTrue())
		e2e.Logf("digest m2d PASS - archive: %s", archiveFile4)

		compat_otp.By("Step 4b: digest - disk2mirror")
		prefixDig := "/88132dig"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscDigest, "--from", "file://"+diskPath4, "docker://"+serInfo.serviceName+prefixDig,
				"--cache-dir="+dirname+"/cache-dig", "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "d2m for digest timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("digest d2m output: %s", output)

		idmsFile4 := filepath.Join(diskPath4, "working-dir/cluster-resources/idms-oc-mirror.yaml")
		o.Expect(idmsFile4).To(o.BeAnExistingFile())

		compat_otp.By("Step 4c: Validate digest - hello-openshift should have tag-1 and tag-2")
		tagUri4 := "https://" + serInfo.serviceName + "/v2/88132dig/openshifttest/hello-openshift/tags/list"
		validateTargetcatalogAndTag(tagUri4, "tag-1")
		validateTargetcatalogAndTag(tagUri4, "tag-2")
		e2e.Logf("digest validation PASS")

		// === Sub-test 5: invalid targetRepo - should warn and skip ===
		compat_otp.By("Step 5: Verify invalid targetRepo is warned and skipped during m2d")
		iscInvalid := filepath.Join(ocmirrorBaseDir, "config-88132-invalid.yaml")
		diskPath5 := filepath.Join(dirname, "disk-invalid")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscInvalid, "file://"+diskPath5, "--cache-dir="+dirname+"/cache-inv", "--v2", "--authfile", authFile).Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d for invalid targetRepo timed out")
		e2e.Logf("invalid targetRepo m2d output: %s", output)
		o.Expect(output).To(o.ContainSubstring("[WARN]"))
		o.Expect(output).To(o.ContainSubstring("invalid targetRepo"))
		o.Expect(output).To(o.ContainSubstring("SKIPPING"))
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("invalid targetRepo validation PASS - warning emitted, invalid entry skipped, valid image mirrored")

		// === Sub-test 6: OCI local image with targetRepo + targetTag ===
		compat_otp.By("Step 6a: Create local OCI directory using skopeo")
		ociDir := filepath.Join(dirname, "oci-ubi")
		skopeExecute(fmt.Sprintf("skopeo copy docker://registry.redhat.io/ubi8/ubi:latest oci://%s --remove-signatures --insecure-policy --authfile %s", ociDir, authFile))

		compat_otp.By("Step 6b: Generate dynamic ISC for OCI")
		ociISCContent := fmt.Sprintf(`kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  additionalImages:
    - name: oci://%s
      targetRepo: custom/oci-dest
      targetTag: v3.0
`, ociDir)
		ociISCFile := filepath.Join(dirname, "config-88132-oci.yaml")
		err = os.WriteFile(ociISCFile, []byte(ociISCContent), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Step 6c: OCI - mirror2disk")
		diskPath6 := filepath.Join(dirname, "disk-oci")
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", ociISCFile, "file://"+diskPath6, "--cache-dir="+dirname+"/cache-oci", "--v2", "--authfile", authFile).Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d for OCI timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		e2e.Logf("OCI m2d output: %s", output)

		archiveFile6 := filepath.Join(diskPath6, "mirror_000001.tar")
		archiveStat6, err := os.Stat(archiveFile6)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveStat6.Size() > 0).Should(o.BeTrue())
		e2e.Logf("OCI m2d PASS - archive: %s", archiveFile6)

		compat_otp.By("Step 6d: OCI - disk2mirror")
		prefixOCI := "/88132oci"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", ociISCFile, "--from", "file://"+diskPath6, "docker://"+serInfo.serviceName+prefixOCI,
				"--cache-dir="+dirname+"/cache-oci", "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "d2m for OCI timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		e2e.Logf("OCI d2m output: %s", output)

		clusterResourcesDir6 := filepath.Join(diskPath6, "working-dir/cluster-resources")
		idmsFile6 := filepath.Join(clusterResourcesDir6, "idms-oc-mirror.yaml")
		itmsFile6 := filepath.Join(clusterResourcesDir6, "itms-oc-mirror.yaml")
		idmsExists6 := false
		itmsExists6 := false
		if _, statErr := os.Stat(idmsFile6); statErr == nil {
			idmsExists6 = true
		}
		if _, statErr := os.Stat(itmsFile6); statErr == nil {
			itmsExists6 = true
		}
		o.Expect(idmsExists6 || itmsExists6).To(o.BeTrue(), "Expected idms-oc-mirror.yaml or itms-oc-mirror.yaml to exist in cluster-resources")
		e2e.Logf("OCI cluster-resources: idms=%v itms=%v", idmsExists6, itmsExists6)

		compat_otp.By("Step 6e: Validate OCI - custom/oci-dest should have tag v3.0")
		tagUri6 := "https://" + serInfo.serviceName + "/v2/88132oci/custom/oci-dest/tags/list"
		validateTargetcatalogAndTag(tagUri6, "v3.0")
		e2e.Logf("OCI validation PASS")

		e2e.Logf("All targetTag/targetRepo sub-tests PASS")
	})

	g.It("Author:ngavali-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-88156-Verify targetTag and targetRepo for m2m workflow [Serial]", g.SpecTimeout(30*time.Minute), func(ctx g.SpecContext) {
		dirname := "/tmp/case88156"
		defer os.RemoveAll(dirname)
		defer os.RemoveAll(".oc-mirror.log")
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := filepath.Join(dirname, ".dockerconfigjson")
		o.Expect(authFile).To(o.BeAnExistingFile())

		ocmirrorBaseDir := testdata.FixturePath("workloads")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)

		var output string
		var mirrorErr error

		// === Sub-test 1: targetTag only ===
		compat_otp.By("Step 1a: targetTag - mirror2mirror")
		iscTargetTag := filepath.Join(ocmirrorBaseDir, "config-88132-target-tag.yaml")
		workspaceTT := filepath.Join(dirname, "ws-targetTag")
		prefixTargetTag := "/88156targettag"
		waitErr := wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetTag, "docker://"+serInfo.serviceName+prefixTargetTag,
				"--workspace", "file://"+workspaceTT, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m for targetTag timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetTag m2m output: %s", output)

		itmsFile1 := filepath.Join(workspaceTT, "working-dir/cluster-resources/itms-oc-mirror.yaml")
		o.Expect(itmsFile1).To(o.BeAnExistingFile())

		compat_otp.By("Step 1b: Validate targetTag - ubi8/ubi should have tag v8")
		tagUri1 := "https://" + serInfo.serviceName + "/v2/88156targettag/ubi8/ubi/tags/list"
		validateTargetcatalogAndTag(tagUri1, "v8")
		e2e.Logf("targetTag validation PASS")

		// === Sub-test 2: targetRepo only ===
		compat_otp.By("Step 2a: targetRepo - mirror2mirror")
		iscTargetRepo := filepath.Join(ocmirrorBaseDir, "config-88132-target-repo.yaml")
		workspaceTargetRepo := filepath.Join(dirname, "ws-targetRepo")
		prefixTargetRepo := "/88156targetrepo"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetRepo, "docker://"+serInfo.serviceName+prefixTargetRepo,
				"--workspace", "file://"+workspaceTargetRepo, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m for targetRepo timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetRepo m2m output: %s", output)

		itmsFile2 := filepath.Join(workspaceTargetRepo, "working-dir/cluster-resources/itms-oc-mirror.yaml")
		o.Expect(itmsFile2).To(o.BeAnExistingFile())

		compat_otp.By("Step 2b: Validate targetRepo - custom-ns/my-ubi8 should have tag latest")
		tagUri2 := "https://" + serInfo.serviceName + "/v2/88156targetrepo/custom-ns/my-ubi8/tags/list"
		validateTargetcatalogAndTag(tagUri2, "latest")
		e2e.Logf("targetRepo validation PASS")

		// === Sub-test 3: targetRepo + targetTag combined ===
		compat_otp.By("Step 3a: targetRepoTag - mirror2mirror")
		iscTargetRepoTag := filepath.Join(ocmirrorBaseDir, "config-88132-target-repo-tag.yaml")
		workspaceTargetRepoTag := filepath.Join(dirname, "ws-targetRepoTag")
		prefixTargetRepoTag := "/88156targetrepotag"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscTargetRepoTag, "docker://"+serInfo.serviceName+prefixTargetRepoTag,
				"--workspace", "file://"+workspaceTargetRepoTag, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m for targetRepoTag timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("targetRepoTag m2m output: %s", output)

		itmsFile3 := filepath.Join(workspaceTargetRepoTag, "working-dir/cluster-resources/itms-oc-mirror.yaml")
		o.Expect(itmsFile3).To(o.BeAnExistingFile())

		compat_otp.By("Step 3b: Validate targetRepoTag - ubi-repo:ubi-v9")
		tagUri3a := "https://" + serInfo.serviceName + "/v2/88156targetrepotag/ubi-repo/tags/list"
		validateTargetcatalogAndTag(tagUri3a, "ubi-v9")
		compat_otp.By("Step 3b: Validate targetRepoTag - nginx-repo:stable-v1")
		tagUri3b := "https://" + serInfo.serviceName + "/v2/88156targetrepotag/nginx-repo/tags/list"
		validateTargetcatalogAndTag(tagUri3b, "stable-v1")
		compat_otp.By("Step 3b: Validate targetRepoTag - fedora-repo:fedora-test")
		tagUri3c := "https://" + serInfo.serviceName + "/v2/88156targetrepotag/fedora-repo/tags/list"
		validateTargetcatalogAndTag(tagUri3c, "fedora-test")
		e2e.Logf("targetRepoTag validation PASS")

		// === Sub-test 4: digest images with targetTag ===
		compat_otp.By("Step 4a: digest - mirror2mirror")
		iscDigest := filepath.Join(ocmirrorBaseDir, "config-88132-digest.yaml")
		workspaceDig := filepath.Join(dirname, "ws-digest")
		prefixDig := "/88156dig"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscDigest, "docker://"+serInfo.serviceName+prefixDig,
				"--workspace", "file://"+workspaceDig, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m for digest timed out")
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToMirror"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("digest m2m output: %s", output)

		idmsFile4 := filepath.Join(workspaceDig, "working-dir/cluster-resources/idms-oc-mirror.yaml")
		o.Expect(idmsFile4).To(o.BeAnExistingFile())

		compat_otp.By("Step 4b: Validate digest - hello-openshift should have tag-1 and tag-2")
		tagUri4 := "https://" + serInfo.serviceName + "/v2/88156dig/openshifttest/hello-openshift/tags/list"
		validateTargetcatalogAndTag(tagUri4, "tag-1")
		validateTargetcatalogAndTag(tagUri4, "tag-2")
		e2e.Logf("digest validation PASS")

		// === Sub-test 5: invalid targetRepo - should warn and skip ===
		compat_otp.By("Step 5: Verify invalid targetRepo is warned and skipped during m2m")
		iscInvalid := filepath.Join(ocmirrorBaseDir, "config-88132-invalid.yaml")
		workspaceInv := filepath.Join(dirname, "ws-invalid")
		prefixInv := "/88156inv"
		waitErr = wait.PollUntilContextTimeout(context.Background(), 20*time.Second, 20*time.Minute, true, func(ctx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(
				"-c", iscInvalid, "docker://"+serInfo.serviceName+prefixInv,
				"--workspace", "file://"+workspaceInv, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			return mirrorErr == nil, mirrorErr
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m for invalid targetRepo timed out")
		e2e.Logf("invalid targetRepo m2m output: %s", output)
		o.Expect(output).To(o.ContainSubstring("[WARN]"))
		o.Expect(output).To(o.ContainSubstring("invalid targetRepo"))
		o.Expect(output).To(o.ContainSubstring("SKIPPING"))
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("additional images mirrored successfully"))
		e2e.Logf("invalid targetRepo validation PASS - warning emitted, invalid entry skipped, valid image mirrored")

		e2e.Logf("All m2m targetTag/targetRepo sub-tests PASS")
	})

	g.It("Author:ngavali-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-87992-Verify m2d+d2m works using pinned catalog [Serial]", g.SpecTimeout(30*time.Minute), func(ctx g.SpecContext) {
		dirname := "/tmp/case87992"
		os.RemoveAll(dirname)
		defer os.RemoveAll(dirname)
		defer os.RemoveAll(".oc-mirror.log")
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := filepath.Join(dirname, ".dockerconfigjson")
		o.Expect(authFile).To(o.BeAnExistingFile())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		iscFile := filepath.Join(ocmirrorBaseDir, "config-87992.yaml")
		diskPath := filepath.Join(dirname, "test")

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		compat_otp.By("Trying to launch a registry app ...")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "30G", "/var/lib/registry")

		compat_otp.By("Step 1: mirror2disk ...")
		var output string
		var mirrorErr error
		waitErr := wait.PollUntilContextTimeout(ctx, 20*time.Second, 10*time.Minute, true, func(pollCtx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", iscFile, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", authFile).Output()
			if mirrorErr != nil {
				e2e.Logf("Step 1 mirror2disk failed: %v, retrying...", mirrorErr)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but mirror2disk has not been completed")
		e2e.Logf("Step 1 mirror2disk output:\n%s", output)

		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("operator images mirrored successfully"))
		o.Expect(output).To(o.ContainSubstring("Generating pinned configurations"))
		o.Expect(output).To(o.ContainSubstring("Pinned ISC written to"))
		o.Expect(output).To(o.ContainSubstring("Pinned DISC written to"))
		e2e.Logf("mirror2disk PASS")

		archiveFilePath := filepath.Join(diskPath, "mirror_000001.tar")
		archiveFile, err := os.Stat(archiveFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveFile.Size()).To(o.BeNumerically(">", int64(0)))
		firstM2DArchiveSize := archiveFile.Size()
		tarFile, err := os.Open(archiveFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer tarFile.Close()
		e2e.Logf("Created valid archive %s (size: %d bytes)", archiveFilePath, firstM2DArchiveSize)

		compat_otp.By("Step 2: Locate the pinned ISC and DISC generated by m2d")
		workingDir := filepath.Join(diskPath, "working-dir")
		entries, err := os.ReadDir(workingDir)
		o.Expect(err).NotTo(o.HaveOccurred())

		var pinnedISCPath, pinnedDISCPath string
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "isc_pinned_") && strings.HasSuffix(entry.Name(), ".yaml") {
				pinnedISCPath = filepath.Join(workingDir, entry.Name())
			}
			if strings.HasPrefix(entry.Name(), "disc_pinned_") && strings.HasSuffix(entry.Name(), ".yaml") {
				pinnedDISCPath = filepath.Join(workingDir, entry.Name())
			}
		}
		o.Expect(pinnedISCPath).NotTo(o.BeEmpty(), fmt.Sprintf("Pinned ISC file not found in working-dir: %s", workingDir))
		o.Expect(pinnedDISCPath).NotTo(o.BeEmpty(), fmt.Sprintf("Pinned DISC file not found in working-dir: %s", workingDir))
		o.Expect(pinnedISCPath).To(o.BeAnExistingFile())
		o.Expect(pinnedDISCPath).To(o.BeAnExistingFile())
		e2e.Logf("Found pinned ISC: %s", pinnedISCPath)
		e2e.Logf("Found pinned DISC: %s", pinnedDISCPath)

		compat_otp.By("Step 3: Validate pinned ISC contains digests instead of tags")
		pinnedISCContent, err := os.ReadFile(pinnedISCPath)
		o.Expect(err).NotTo(o.HaveOccurred())
		pinnedISCStr := string(pinnedISCContent)
		e2e.Logf("Pinned ISC content:\n%s", pinnedISCStr)
		o.Expect(pinnedISCStr).To(o.ContainSubstring("@sha256:"), "Pinned ISC should contain digest references (@sha256:)")
		o.Expect(pinnedISCStr).NotTo(o.MatchRegexp(`catalog:\s+\S+:v[0-9]+`), "Pinned ISC should not contain tag-based catalog references")

		// DISC is validated for correct generation only; exercising the delete workflow is out of scope here.
		compat_otp.By("Step 4: Validate pinned DISC contains digests instead of tags")
		pinnedDISCContent, err := os.ReadFile(pinnedDISCPath)
		o.Expect(err).NotTo(o.HaveOccurred())
		pinnedDISCStr := string(pinnedDISCContent)
		e2e.Logf("Pinned DISC content:\n%s", pinnedDISCStr)
		o.Expect(pinnedDISCStr).To(o.ContainSubstring("@sha256:"), "Pinned DISC should contain digest references (@sha256:)")
		o.Expect(pinnedDISCStr).NotTo(o.MatchRegexp(`catalog:\s+\S+:v[0-9]+`), "Pinned DISC should not contain tag-based catalog references")

		compat_otp.By("Step 5: Save pinned ISC, then delete first m2d data and cache")
		savedPinnedISCPath := filepath.Join(dirname, "saved_pinned_isc.yaml")
		err = os.WriteFile(savedPinnedISCPath, pinnedISCContent, 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Saved pinned ISC to: %s", savedPinnedISCPath)

		e2e.Logf("Deleting first m2d disk data and cache from: %s", dirname)
		preserveFiles := map[string]bool{
			".dockerconfigjson":     true,
			"saved_pinned_isc.yaml": true,
		}
		cacheEntries, err := os.ReadDir(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, ce := range cacheEntries {
			if preserveFiles[ce.Name()] {
				continue
			}
			entryPath := filepath.Join(dirname, ce.Name())
			e2e.Logf("Removing: %s", entryPath)
			if rmErr := os.RemoveAll(entryPath); rmErr != nil {
				e2e.Logf("Warning: failed to remove %s: %v", entryPath, rmErr)
			}
		}
		e2e.Logf("First m2d data and cache deleted")
		o.Expect(savedPinnedISCPath).To(o.BeAnExistingFile(), "Saved pinned ISC should survive cleanup")
		o.Expect(authFile).To(o.BeAnExistingFile(), "Auth file should survive cleanup")

		compat_otp.By("Step 6: Perform m2d using saved pinned ISC (fresh, no cache)")
		waitErr = wait.PollUntilContextTimeout(ctx, 20*time.Second, 10*time.Minute, true, func(pollCtx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", savedPinnedISCPath, "file://"+diskPath, "--cache-dir="+dirname, "--v2", "--authfile", authFile).Output()
			if mirrorErr != nil {
				e2e.Logf("Step 6 m2d with pinned ISC failed: %v, retrying...", mirrorErr)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but m2d with pinned ISC has not been completed")
		e2e.Logf("Step 6 m2d with pinned ISC output:\n%s", output)
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: mirrorToDisk"))
		o.Expect(output).To(o.ContainSubstring("operator images mirrored successfully"))
		o.Expect(output).NotTo(o.MatchRegexp(`catalog.*:v[0-9]+`), "Step 6 output should not contain tag-based catalog references when using pinned ISC")

		pinnedArchiveFile, err := os.Stat(archiveFilePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		pinnedArchiveSize := pinnedArchiveFile.Size()
		e2e.Logf("Pinned m2d archive size: %d bytes (first m2d: %d bytes)", pinnedArchiveSize, firstM2DArchiveSize)
		o.Expect(pinnedArchiveSize).To(o.BeNumerically("~", firstM2DArchiveSize, firstM2DArchiveSize/10))
		e2e.Logf("m2d using pinned ISC PASS")

		compat_otp.By("Step 7: disk2mirror using pinned ISC")
		waitErr = wait.PollUntilContextTimeout(ctx, 20*time.Second, 10*time.Minute, true, func(pollCtx context.Context) (bool, error) {
			output, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", savedPinnedISCPath, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
			if mirrorErr != nil {
				e2e.Logf("Step 7 d2m with pinned ISC failed: %v, retrying...", mirrorErr)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached but d2m with pinned ISC has not been completed")
		e2e.Logf("Step 7 d2m with pinned ISC output:\n%s", output)
		o.Expect(output).NotTo(o.ContainSubstring("[ERROR]"))
		o.Expect(output).To(o.ContainSubstring("workflow mode: diskToMirror"))
		o.Expect(output).To(o.ContainSubstring("operator images mirrored successfully"))
		o.Expect(output).NotTo(o.MatchRegexp(`catalog.*:v[0-9]+`), "Step 7 output should not contain tag-based catalog references when using pinned ISC")
		e2e.Logf("d2m using pinned ISC PASS")

		compat_otp.By("Step 8: Validate cluster-resources files")
		clusterResourcesDir := filepath.Join(diskPath, "working-dir/cluster-resources")
		idmsPath := filepath.Join(clusterResourcesDir, "idms-oc-mirror.yaml")
		e2e.Logf("Checking %s...", idmsPath)
		o.Expect(idmsPath).To(o.BeAnExistingFile())
		idmsContent, err := os.ReadFile(idmsPath)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(idmsContent)).To(o.ContainSubstring("ImageDigestMirrorSet"), "IDMS should be of kind ImageDigestMirrorSet")
		o.Expect(string(idmsContent)).To(o.ContainSubstring("imageDigestMirrors"), "IDMS should contain imageDigestMirrors entries")
		o.Expect(string(idmsContent)).To(o.ContainSubstring(serInfo.serviceName), "IDMS should reference the target registry")

		crEntries, err := os.ReadDir(clusterResourcesDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		var foundCS, foundCC bool
		for _, cre := range crEntries {
			e2e.Logf("cluster-resources entry: %s", cre.Name())
			if strings.HasPrefix(cre.Name(), "cs-") && strings.HasSuffix(cre.Name(), ".yaml") {
				foundCS = true
			}
			if strings.HasPrefix(cre.Name(), "cc-") && strings.HasSuffix(cre.Name(), ".yaml") {
				foundCC = true
			}
		}
		o.Expect(foundCS).To(o.BeTrue(), "CatalogSource YAML not found in cluster-resources")
		o.Expect(foundCC).To(o.BeTrue(), "ClusterCatalog YAML not found in cluster-resources")
		e2e.Logf("cluster-resources validation PASS")

		compat_otp.By("Step 9: Validate registry contains mirrored catalog")
		catalogUri := fmt.Sprintf("https://%s/v2/redhat/redhat-operator-index/tags/list", serInfo.serviceName)
		validateTargetcatalogAndTag(catalogUri, "sha256-")
		e2e.Logf("registry-side validation PASS")

		compat_otp.By("Step 10: Verify d2m with unpinned ISC fails (cache was deleted, digest-to-tag resolution impossible)")
		unpinnedOutput, unpinnedD2MErr := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", iscFile, "--from", "file://"+diskPath, "docker://"+serInfo.serviceName, "--cache-dir="+dirname, "--v2", "--authfile", authFile, "--dest-tls-verify=false").Output()
		e2e.Logf("Step 10 unpinned d2m output:\n%s", unpinnedOutput)
		o.Expect(unpinnedD2MErr).To(o.HaveOccurred(), "d2m with unpinned ISC should fail when cache is missing and disk data is digest-based")
		e2e.Logf("d2m with unpinned ISC correctly failed as expected")

		e2e.Logf("PASS")
	})

	g.It("Author:ngavali-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-High-87962-Verify operator images are pinned by digest [Serial]", g.SpecTimeout(30*time.Minute), func(ctx g.SpecContext) {
		dirname := "/tmp/case87962"
		os.RemoveAll(dirname)
		defer os.RemoveAll(dirname)
		defer os.RemoveAll(".oc-mirror.log")
		defer func() {
			if homeDir, err := os.UserHomeDir(); err == nil {
				os.RemoveAll(filepath.Join(homeDir, ".oc-mirror"))
			}
		}()
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		cleanupMirrorArtifacts := func(paths ...string) {
			for _, p := range paths {
				os.RemoveAll(p)
			}
			os.RemoveAll(".oc-mirror.log")
		}

		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		authFile := filepath.Join(dirname, ".dockerconfigjson")
		o.Expect(authFile).To(o.BeAnExistingFile())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		operatorsISC := filepath.Join(ocmirrorBaseDir, "config-87962-operators.yaml")
		multiCatalogISC := filepath.Join(ocmirrorBaseDir, "config-87962-multi-catalog.yaml")

		type mirrorConfig struct {
			iscPath          string
			destination      string
			workspace        string
			cacheDir         string
			removeSignatures bool
			workflowType     string
		}

		buildMirrorArgs := func(cfg mirrorConfig, mirrorAuthFile string, destTLSVerify bool) []string {
			args := []string{"-c", cfg.iscPath, cfg.destination}
			if cfg.workspace != "" {
				args = append(args, "--workspace", cfg.workspace)
			}
			if cfg.cacheDir != "" {
				args = append(args, "--cache-dir="+cfg.cacheDir)
			}
			args = append(args, "--v2", "--authfile", mirrorAuthFile)
			if !destTLSVerify {
				args = append(args, "--dest-tls-verify=false")
			}
			if cfg.removeSignatures {
				args = append(args, "--remove-signatures")
			}
			return args
		}

		executeMirrorWithRetry := func(mirrorCtx context.Context, mirrorAuthFile string,
			cfg mirrorConfig, destTLSVerify bool) (string, error) {
			var mirrorOutput string
			var mirrorErr error
			waitErr := wait.PollUntilContextTimeout(mirrorCtx, 20*time.Second, 10*time.Minute, true,
				func(pollCtx context.Context) (bool, error) {
					args := buildMirrorArgs(cfg, mirrorAuthFile, destTLSVerify)
					mirrorOutput, mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(args...).Output()
					if mirrorErr != nil {
						e2e.Logf("%s failed: %v, retrying...", cfg.workflowType, mirrorErr)
						return false, nil
					}
					return true, nil
				})
			return mirrorOutput, waitErr
		}

		validateMirrorOutput := func(mirrorOutput, expectedWorkflow string) {
			o.Expect(mirrorOutput).NotTo(o.ContainSubstring("[ERROR]"))
			o.Expect(mirrorOutput).To(o.ContainSubstring("workflow mode: " + expectedWorkflow))
			o.Expect(mirrorOutput).To(o.ContainSubstring("operator images mirrored successfully"))
			o.Expect(mirrorOutput).To(o.ContainSubstring("Generating pinned configurations"))
			o.Expect(mirrorOutput).To(o.ContainSubstring("Pinned ISC written to"))
			o.Expect(mirrorOutput).To(o.ContainSubstring("Pinned DISC written to"))
		}

		validateClusterResources := func(workingDir string, csPrefixes, ccPrefixes []string) {
			clusterResourcesDir := filepath.Join(workingDir, "cluster-resources")
			idmsPath := filepath.Join(clusterResourcesDir, "idms-oc-mirror.yaml")
			o.Expect(idmsPath).To(o.BeAnExistingFile())
			idmsContent, readErr := os.ReadFile(idmsPath)
			o.Expect(readErr).NotTo(o.HaveOccurred())
			o.Expect(string(idmsContent)).To(o.ContainSubstring("ImageDigestMirrorSet"))
			o.Expect(string(idmsContent)).To(o.ContainSubstring("imageDigestMirrors"), "IDMS should contain mirror entries")

			itmsPath := filepath.Join(clusterResourcesDir, "itms-oc-mirror.yaml")
			o.Expect(itmsPath).NotTo(o.BeAnExistingFile(), "ITMS should not be generated when all images are digest-based")

			entries, readErr := os.ReadDir(clusterResourcesDir)
			o.Expect(readErr).NotTo(o.HaveOccurred())
			for _, prefix := range csPrefixes {
				found := false
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), prefix) {
						found = true
						break
					}
				}
				o.Expect(found).To(o.BeTrue(), "CatalogSource with prefix "+prefix+" not found in cluster-resources")
			}
			for _, prefix := range ccPrefixes {
				found := false
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), prefix) {
						found = true
						break
					}
				}
				o.Expect(found).To(o.BeTrue(), "ClusterCatalog with prefix "+prefix+" not found in cluster-resources")
			}
		}

		findPinnedConfigs := func(workingDir string) (string, string, error) {
			entries, err := os.ReadDir(workingDir)
			if err != nil {
				return "", "", fmt.Errorf("failed to read directory %s: %w", workingDir, err)
			}
			var iscPath, discPath string
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "isc_pinned_") && strings.HasSuffix(entry.Name(), ".yaml") {
					iscPath = filepath.Join(workingDir, entry.Name())
				}
				if strings.HasPrefix(entry.Name(), "disc_pinned_") && strings.HasSuffix(entry.Name(), ".yaml") {
					discPath = filepath.Join(workingDir, entry.Name())
				}
			}
			if iscPath == "" {
				return "", "", fmt.Errorf("pinned ISC not found in %s", workingDir)
			}
			if discPath == "" {
				return "", "", fmt.Errorf("pinned DISC not found in %s", workingDir)
			}
			if _, err := os.Stat(iscPath); err != nil {
				return "", "", fmt.Errorf("pinned ISC file not accessible %s: %w", iscPath, err)
			}
			if _, err := os.Stat(discPath); err != nil {
				return "", "", fmt.Errorf("pinned DISC file not accessible %s: %w", discPath, err)
			}
			return iscPath, discPath, nil
		}

		validatePinnedDigests := func(filePath, label, expectedKind string) string {
			content, readErr := os.ReadFile(filePath)
			o.Expect(readErr).NotTo(o.HaveOccurred())
			contentStr := string(content)
			e2e.Logf("%s content:\n%s", label, contentStr)
			o.Expect(contentStr).To(o.ContainSubstring("kind: "+expectedKind), label+" should have kind: "+expectedKind)
			o.Expect(contentStr).To(o.ContainSubstring("@sha256:"), label+" should contain digest references")
			o.Expect(contentStr).NotTo(o.MatchRegexp(`catalog:\s+\S+:v[0-9]+`), label+" should not contain tag-based catalog references")
			return contentStr
		}

		compat_otp.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", serInfo.serviceName)
		setRegistryVolume(oc, "deploy", "registry", oc.Namespace(), "15G", "/var/lib/registry")

		// === Part 1: m2d with operators ISC ===
		compat_otp.By("Step 1a: mirror2disk with operators ISC")
		m2dDiskPath := filepath.Join(dirname, "m2d-test")
		output, waitErr := executeMirrorWithRetry(ctx, authFile, mirrorConfig{
			iscPath:      operatorsISC,
			destination:  "file://" + m2dDiskPath,
			cacheDir:     dirname + "/cache-m2d",
			workflowType: "m2d",
		}, true)
		compat_otp.AssertWaitPollNoErr(waitErr, "m2d with operators ISC timed out")
		e2e.Logf("m2d output:\n%s", output)
		validateMirrorOutput(output, "mirrorToDisk")

		archivePath := filepath.Join(m2dDiskPath, "mirror_000001.tar")
		archiveInfo, err := os.Stat(archivePath)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(archiveInfo.Size()).To(o.BeNumerically(">", int64(0)))
		e2e.Logf("m2d archive created: %s (size: %d bytes)", archivePath, archiveInfo.Size())

		compat_otp.By("Step 1b: Validate m2d pinned ISC/DISC contain digests")
		m2dWorkingDir := filepath.Join(m2dDiskPath, "working-dir")
		m2dPinnedISCPath, m2dPinnedDISCPath, err := findPinnedConfigs(m2dWorkingDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Found m2d pinned ISC: %s, DISC: %s", m2dPinnedISCPath, m2dPinnedDISCPath)
		m2dPinnedISCContent := validatePinnedDigests(m2dPinnedISCPath, "m2d pinned ISC", "ImageSetConfiguration")
		m2dPinnedDISCContent := validatePinnedDigests(m2dPinnedDISCPath, "m2d pinned DISC", "DeleteImageSetConfiguration")
		o.Expect(m2dPinnedISCContent).To(o.ContainSubstring("aws-load-balancer-operator"))
		o.Expect(m2dPinnedISCContent).To(o.ContainSubstring("file-integrity-operator"))
		o.Expect(m2dPinnedDISCContent).To(o.ContainSubstring("aws-load-balancer-operator"))
		o.Expect(m2dPinnedDISCContent).To(o.ContainSubstring("file-integrity-operator"))
		e2e.Logf("m2d pinned config validation PASS")

		e2e.Logf("Cleaning up m2d data, cache and logs...")
		cleanupMirrorArtifacts(m2dDiskPath, filepath.Join(dirname, "cache-m2d"))

		// === Part 2: m2m with operators ISC ===
		compat_otp.By("Step 2a: mirror2mirror with operators ISC")
		m2mWorkspace := filepath.Join(dirname, "m2m-operators")
		m2mRegistryPrefix := "/87962m2m"
		output, waitErr = executeMirrorWithRetry(ctx, authFile, mirrorConfig{
			iscPath:      operatorsISC,
			destination:  "docker://" + serInfo.serviceName + m2mRegistryPrefix,
			workspace:    "file://" + m2mWorkspace,
			workflowType: "m2m with operators ISC",
		}, false)
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m with operators ISC timed out")
		e2e.Logf("m2m operators output:\n%s", output)
		validateMirrorOutput(output, "mirrorToMirror")

		compat_otp.By("Step 2b: Validate m2m pinned ISC/DISC contain digests")
		m2mWorkingDir := filepath.Join(m2mWorkspace, "working-dir")
		m2mPinnedISCPath, m2mPinnedDISCPath, err := findPinnedConfigs(m2mWorkingDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Found m2m pinned ISC: %s, DISC: %s", m2mPinnedISCPath, m2mPinnedDISCPath)
		m2mPinnedISCContent := validatePinnedDigests(m2mPinnedISCPath, "m2m pinned ISC", "ImageSetConfiguration")
		m2mPinnedDISCContent := validatePinnedDigests(m2mPinnedDISCPath, "m2m pinned DISC", "DeleteImageSetConfiguration")
		o.Expect(m2mPinnedISCContent).To(o.ContainSubstring("aws-load-balancer-operator"))
		o.Expect(m2mPinnedISCContent).To(o.ContainSubstring("file-integrity-operator"))
		o.Expect(m2mPinnedDISCContent).To(o.ContainSubstring("aws-load-balancer-operator"))
		o.Expect(m2mPinnedDISCContent).To(o.ContainSubstring("file-integrity-operator"))

		compat_otp.By("Step 2c: Validate m2m cluster-resources")
		validateClusterResources(m2mWorkingDir, []string{"cs-"}, []string{"cc-"})

		compat_otp.By("Step 2d: Validate registry contains mirrored catalog")
		m2mCatalogTagsURL := fmt.Sprintf("https://%s/v2/87962m2m/redhat/redhat-operator-index/tags/list", serInfo.serviceName)
		validateTargetcatalogAndTag(m2mCatalogTagsURL, "v4.20")
		e2e.Logf("m2m operators pinned config + cluster-resources + registry validation PASS")

		e2e.Logf("Cleaning up m2m operators data, cache and logs...")
		cleanupMirrorArtifacts(m2mWorkspace)

		// === Part 3: m2m with multi-catalog ISC + --remove-signatures ===
		compat_otp.By("Step 3a: mirror2mirror with multi-catalog ISC + --remove-signatures")
		multiCatWorkspace := filepath.Join(dirname, "m2m-multi-catalog")
		multiCatRegistryPrefix := "/87962multicat"
		output, waitErr = executeMirrorWithRetry(ctx, authFile, mirrorConfig{
			iscPath:          multiCatalogISC,
			destination:      "docker://" + serInfo.serviceName + multiCatRegistryPrefix,
			workspace:        "file://" + multiCatWorkspace,
			removeSignatures: true,
			workflowType:     "m2m with multi-catalog ISC",
		}, false)
		compat_otp.AssertWaitPollNoErr(waitErr, "m2m with multi-catalog ISC timed out")
		e2e.Logf("m2m multi-catalog output:\n%s", output)
		validateMirrorOutput(output, "mirrorToMirror")

		compat_otp.By("Step 3b: Validate multi-catalog pinned configs contain digests for both catalogs")
		multiCatWorkingDir := filepath.Join(multiCatWorkspace, "working-dir")
		multiCatPinnedISCPath, multiCatPinnedDISCPath, err := findPinnedConfigs(multiCatWorkingDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Found multi-catalog pinned ISC: %s, DISC: %s", multiCatPinnedISCPath, multiCatPinnedDISCPath)
		multiCatPinnedISCContent := validatePinnedDigests(multiCatPinnedISCPath, "multi-catalog pinned ISC", "ImageSetConfiguration")
		multiCatPinnedDISCContent := validatePinnedDigests(multiCatPinnedDISCPath, "multi-catalog pinned DISC", "DeleteImageSetConfiguration")

		o.Expect(multiCatPinnedISCContent).To(o.ContainSubstring("redhat-operator-index@sha256:"))
		o.Expect(multiCatPinnedISCContent).To(o.ContainSubstring("certified-operator-index@sha256:"))
		o.Expect(multiCatPinnedISCContent).To(o.ContainSubstring("aws-load-balancer-operator"))
		o.Expect(multiCatPinnedISCContent).To(o.ContainSubstring("netscaler-operator"))

		o.Expect(multiCatPinnedDISCContent).To(o.ContainSubstring("redhat-operator-index@sha256:"))
		o.Expect(multiCatPinnedDISCContent).To(o.ContainSubstring("certified-operator-index@sha256:"))
		o.Expect(multiCatPinnedDISCContent).To(o.ContainSubstring("aws-load-balancer-operator"))
		o.Expect(multiCatPinnedDISCContent).To(o.ContainSubstring("netscaler-operator"))

		compat_otp.By("Step 3c: Validate multi-catalog cluster-resources")
		validateClusterResources(multiCatWorkingDir,
			[]string{"cs-redhat-operator-index", "cs-certified-operator-index"},
			[]string{"cc-redhat-operator-index", "cc-certified-operator-index"})

		compat_otp.By("Step 3d: Validate registry contains mirrored catalogs")
		redhatCatalogURL := fmt.Sprintf("https://%s/v2/87962multicat/redhat/redhat-operator-index/tags/list", serInfo.serviceName)
		validateTargetcatalogAndTag(redhatCatalogURL, "v4.20")
		certifiedCatalogURL := fmt.Sprintf("https://%s/v2/87962multicat/redhat/certified-operator-index/tags/list", serInfo.serviceName)
		validateTargetcatalogAndTag(certifiedCatalogURL, "v4.20")
		e2e.Logf("m2m multi-catalog pinned config validation PASS")

		e2e.Logf("Cleaning up m2m multi-catalog data, cache and logs...")
		cleanupMirrorArtifacts(multiCatWorkspace)

		e2e.Logf("OCP-87962 PASS")
	})
})
