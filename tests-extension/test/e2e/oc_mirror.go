package workloads

import (
	"github.com/openshift/oc-mirror-tests-extension/test/e2e/testdata"
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"github.com/openshift/origin/test/extended/util/compat_otp/architecture"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[OTP][sig-cli] Workloads ocmirror v1 works well", func() {
	defer g.GinkgoRecover()

	var (
		oc = compat_otp.NewCLI("ocmirror", compat_otp.KubeConfigPath())
	)

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-46517-List operator content with different options", func() {
		dirname := "/tmp/case46517"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		dockerCreFile, homePath, err := locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			os.RemoveAll(dockerCreFile)
			_, err = os.Stat(homePath + "/.docker/config.json.back")
			if err == nil {
				copyFile(homePath+"/.docker/config.json.back", homePath+"/.docker/config.json")
			}
		}()

		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--version=4.11", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage := []string{
			"registry.redhat.io/redhat/redhat-operator-index:v4.11",
			"registry.redhat.io/redhat/certified-operator-index:v4.11",
			"registry.redhat.io/redhat/community-operator-index:v4.11",
			"registry.redhat.io/redhat/redhat-marketplace-index:v4.11",
		}
		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		out, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--version=4.11", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage = []string{
			"3scale-operator",
			"amq-online",
			"amq-streams",
			"amq7-interconnect-operator",
			"ansible-automation-platform-operator",
			"ansible-cloud-addons-operator",
			"apicast-operator",
			"businessautomation-operator",
			"cincinnati-operator",
			"cluster-logging",
			"compliance-operator",
			"container-security-operator",
			"costmanagement-metrics-operator",
			"cryostat-operator",
			"datagrid",
			"devworkspace-operator",
			"eap",
			"elasticsearch-operator",
			"external-dns-operator",
			"file-integrity-operator",
			"fuse-apicurito",
			"fuse-console",
			"fuse-online",
			"gatekeeper-operator-product",
			"jaeger-product",
			"jws-operator",
			"kiali-ossm",
			"kubevirt-hyperconverged",
			"mcg-operator",
			"mtc-operator",
			"mtv-operator",
			"node-healthcheck-operator",
			"node-maintenance-operator",
			"ocs-operator",
			"odf-csi-addons-operator",
			"odf-lvm-operator",
			"odf-multicluster-orchestrator",
			"odf-operator",
			"odr-cluster-operator",
			"odr-hub-operator",
			"openshift-cert-manager-operator",
			"openshift-gitops-operator",
			"openshift-pipelines-operator-rh",
			"openshift-secondary-scheduler-operator",
			"opentelemetry-product",
			"quay-bridge-operator",
			"quay-operator",
			"red-hat-camel-k",
			"redhat-oadp-operator",
			"rh-service-binding-operator",
			"rhacs-operator",
			"rhpam-kogito-operator",
			"rhsso-operator",
			"sandboxed-containers-operator",
			"serverless-operator",
			"service-registry-operator",
			"servicemeshoperator",
			"skupper-operator",
			"submariner",
			"web-terminal",
		}

		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--package=cluster-logging", "--channel=stable", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--package=cluster-logging", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

	})
	g.It("ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-46818-Low-46523-check the User Agent for oc-mirror", func() {
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "catlog-loggings.yaml")

		dirname := "/tmp/case46523"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("/tmp/case46523/oc-mirror-workspace")
		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/case46523", "-v", "7", "--dry-run", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//check user-agent and dry-run should write mapping file
		checkMessage := []string{
			"User-Agent: oc-mirror",
			"Writing image mapping",
		}
		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		_, err = os.Stat("/tmp/case46523/oc-mirror-workspace/mapping.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("[Level0] Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Medium-46770-Low-46520-Local backend support for oc-mirror", func() {
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "ocmirror-localbackend.yaml")

		dirname := "/tmp/46770test"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {

			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46770test", "--continue-on-error", "-v", "3", "--v1").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !strings.Contains(out, "Using local backend at location") {
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but the oc-mirror still failed"))

		_, err = os.Stat("/tmp/46770test/publish/.metadata.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("describe", "/tmp/46770test/mirror_seq1_000000.tar", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-46506-High-46817-Mirror a single image works well [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "config_singleimage.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Mirror to registry")
		err := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if strings.Contains(out, "using stateless mode") {
				return true, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Can't see the stateless mode log with %s", err))
		g.By("Mirror to localhost")
		dirname := "/tmp/46506test"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out1, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46506test", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out1, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
		_, err = os.Stat("/tmp/46506test/mirror_seq1_000000.tar")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Mirror to registry from archive")
		defer os.RemoveAll("oc-mirror-workspace")
		out2, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "/tmp/46506test/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName+"/mirrorachive", "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out2, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
	})
	g.It("[Level0] Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-NonPreRelease--Low-51093-oc-mirror init", func() {
		if !assertPullSecret(oc) {
			g.Skip("the cluster do not has all pull-secret for public registry")
		}
		g.By("Set podman registry config")
		dirname := "/tmp/case51093"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out1, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init", "--output", "json", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out1, "registry") {
			e2e.Failf("Can't find the storageconfig of registry")
		}
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-46769-Critical-46515-High-registry backend test [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		g.By("Set podman registry config")
		dirname := "/tmp/case46769"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		operatorConfigS := filepath.Join(ocmirrorBaseDir, "registry_backend_operator_helm.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, operatorConfigS)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Mirroring selected operator and helm image")
		defer os.RemoveAll("oc-mirror-workspace")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--continue-on-error", "--v1").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, "oc-mirror command still falied")
	})
	g.It("NonHyperShiftHOST-Author:yinzhou-NonPreRelease-Longduration-Medium-37372-High-40322-oc adm release extract pull from localregistry when given a localregistry image [Disruptive]", func() {
		var imageDigest string
		g.By("Set podman registry config")
		dirname := "/tmp/case37372"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ocpPlatformConfigS := filepath.Join(ocmirrorBaseDir, "registry_backend_ocp_latest.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, ocpPlatformConfigS)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeOcMirrorLog()
		g.By("Create the mapping file by oc-mirror dry-run command")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ocpPlatformConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run", "--v1").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		g.By("Checkpoint for 40322, mirror with mapping")
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "-f", "oc-mirror-workspace/mapping.txt", "--max-per-registry", "1", "--skip-multiple-scopes=true", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check for the mirrored image and get the image digest")
		imageDigest = getDigestFromImageInfo(oc, serInfo.serviceName)

		g.By("Run oc-mirror to create ICSP file")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ocpPlatformConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checkpoint for 37372")
		g.By("Remove the podman Cred")
		os.RemoveAll(dirname)
		g.By("Try to extract without icsp file, will failed")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", "--to=oc-mirror-workspace/", serInfo.serviceName+"/openshift/release-images"+imageDigest, "--insecure").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		g.By("Try to extract with icsp file, will extract from localregisty")
		imageContentSourcePolicy := findImageContentSourcePolicy()
		waitErr := wait.Poll(120*time.Second, 600*time.Second, func() (bool, error) {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", "--to=oc-mirror-workspace/", "--icsp-file="+imageContentSourcePolicy, serInfo.serviceName+"/openshift/release-images"+"@"+imageDigest, "--insecure").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the mirror still failed"))
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Longduration-Medium-46518-List ocp release content with different options", func() {
		g.By("Set podman registry config")
		dirname := "/tmp/case46518"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("List releases for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List release channels for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channels", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List available releases from channel candidate for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channel=candidate-4.11", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List available releases from channel candidate for ocp 4.11 and specify arch arm64")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channel=candidate-4.11", "--filter-by-archs=arm64", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Longduration-Medium-60594-ImageSetConfig containing OCI FBC and release platform and additionalImages works well with --include-local-oci-catalogs flag [Serial]", func() {
		err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("version", "--v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry config")
		dirname := "/tmp/case60594"
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		var skopeooutStr string
		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			skopeoout, err := exec.Command("bash", "-c", command).Output()
			skopeooutStr = string(skopeoout)
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			e2e.Logf("output: %v", skopeooutStr)
		}
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ociFullConfig := filepath.Join(ocmirrorBaseDir, "config-oci-all.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFullConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run", "--v1").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Longduration-Author:yinzhou-NonPreRelease-Medium-60597-Critical-60595-oc-mirror support for TargetCatalog field for operator[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60597"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := testdata.FixturePath("workloads/config-60597")
		normalTargetConfig := filepath.Join(ocmirrorBaseDir, "config-60597-normal-target.yaml")
		ociTargetTagConfig := filepath.Join(ocmirrorBaseDir, "config-60597-oci-target-tag.yaml")
		normalConfig := filepath.Join(ocmirrorBaseDir, "config-60597-normal.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalTargetConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Can't find the expect target catalog %s", err))
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociTargetTagConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociTargetTagConfig, "docker://"+serInfo.serviceName+"/ocit", "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalConfig, "docker://"+serInfo.serviceName+"/testname", "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Checkpoint for 60595")
		ocmirrorDir := testdata.FixturePath("workloads")
		ociFirstConfig := filepath.Join(ocmirrorDir, "config-oci-f.yaml")
		ociSecondConfig := filepath.Join(ocmirrorDir, "config-oci-s.yaml")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFirstConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociSecondConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", output); !matched {
			e2e.Failf("Can't find the prune log\n")
		}
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-ConnectedOnly-Longduration-NonPreRelease-Medium-60707-oc mirror purne for mirror2disk and mirror2mirror with and without skip-pruning[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60707"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		buildPruningBaseDir := testdata.FixturePath("workloads/config-60603")
		configFirst := filepath.Join(buildPruningBaseDir, "config-normal-first.yaml")
		configSecond := filepath.Join(buildPruningBaseDir, "config-normal-second.yaml")
		configThird := filepath.Join(buildPruningBaseDir, "config-normal-third.yaml")

		fileList := []string{configFirst, configSecond, configThird}
		for _, file := range fileList {
			sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, file)
			_, err = exec.Command("bash", "-c", sedCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")

		defer os.RemoveAll("mirror_seq1_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configFirst, "file://", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		g.By("Check the tag for mirrored image")
		checkCmd := fmt.Sprintf(`curl -k 'https://%s/v2/kube-descheduler-operator/kube-descheduler-operator-bundle/tags/list'`, serInfo.serviceName)
		output, err := exec.Command("bash", "-c", checkCmd).Output()
		outputStr := string(output)
		e2e.Logf("after mirror first: %s", outputStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputStr).NotTo(o.ContainSubstring("null"))
		defer os.RemoveAll("mirror_seq2_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configSecond, "file://", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		outputMirror, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq2_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", outputMirror); !matched {
			e2e.Failf("Can't find the prune log\n")
		}
		g.By("Check the tag again, should be null")
		outputNew, err := exec.Command("bash", "-c", checkCmd).Output()
		outputStr = string(outputNew)
		e2e.Logf("after mirror second: %s", outputStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputStr).To(o.ContainSubstring("null"))
		defer os.RemoveAll("mirror_seq3_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configThird, "file://", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		outputMirror, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq3_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--skip-pruning", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", outputMirror); matched {
			e2e.Failf("Should not find the prune log\n")
		}
		output, err = exec.Command("bash", "-c", checkCmd).Output()
		outputStr = string(output)
		e2e.Logf("after mirror third: %s", outputStr)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputStr).To(o.ContainSubstring("null"))
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-60611-Medium-62694-oc mirror for oci fbc catalogs should work fine with registries.conf[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60611"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the skopeo copy still failed")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		g.By("Trying to launch the second registry app")
		secondSerInfo := registry.createregistrySpecifyName(oc, "secondregistry")
		g.By("Prepare test data to first registry")
		ocmirrorBaseDir := testdata.FixturePath("workloads/case60611")
		ociConfig := filepath.Join(ocmirrorBaseDir, "config.yaml")
		registryConfig := filepath.Join(ocmirrorBaseDir, "registry.conf")
		digestConfig := filepath.Join(ocmirrorBaseDir, "config-62694.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, registryConfig)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))

		g.By("Use oc-mirror with registry.conf")
		logOut, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociConfig, "docker://"+secondSerInfo.serviceName, "--dest-skip-tls", "--oci-registries-config", registryConfig, "--source-use-http", "--source-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(logOut, serInfo.serviceName)).To(o.BeTrue())

		g.By("Checkpoint for 62694")
		defer os.RemoveAll("mirror_seq1_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", digestConfig, "file://", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Medium-60601-Medium-60602-oc mirror support to filter operator by channels on oci fbc catalog [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60601"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-60601")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-60601")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy the registry as OCI FBC")
		var skopeooutStr string
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			skopeoout, err := exec.Command("bash", "-c", command).Output()
			skopeooutStr = string(skopeoout)
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		if waitErr != nil {
			e2e.Logf("output: %v", skopeooutStr)
		}
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-filter.yaml")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, ociFilterConfig)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("oc-mirror-workspace")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--ignore-history", "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checkpoint for 60602")
		defer removeCSAndISCP(oc)
		createCSAndISCPNoPackageCheck(oc, "cs-case60601-redhat-operator-index", "openshift-marketplace", "Running")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Hign-65149-mirror2disk and disk2mirror workflow for local oci catalog [Serial]", func() {
		_ = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("version", "--v1").Execute()
		g.By("Set registry config")
		dirname := "/tmp/case65149"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-65149")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-65149")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy the catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/oci-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65149.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		g.By("Starting mirror2disk ....")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "file://"+dirname, "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror to disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		g.By("Starting disk2mirror  ....")
		mirrorErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("disk to registry failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(mirrorErr, "max time reached but the disk to registry still failed")

		defer removeCSAndISCP(oc)
		createCSAndISCPNoPackageCheck(oc, "cs-test", "openshift-marketplace", "Running")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Critical-65150-mirror2disk and disk2mirror workflow for local multi oci catalog [Serial]", func() {
		_ = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("version", "--v1").Execute()
		g.By("Set registry config")
		dirname := "/tmp/case65150"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-65150")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-65150")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy the multi-arch catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/oci-multi-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65150.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, ociFilterConfig)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		os.RemoveAll("oc-mirror-workspace")
		g.By("Starting mirror2disk ....")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "file://"+dirname, "--ignore-history", "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror to disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		g.By("Starting disk2mirror  ....")
		mirrorErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("disk to registry failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(mirrorErr, "max time reached but the disk to registry still failed")
		defer removeCSAndISCP(oc)
		createCSAndISCPNoPackageCheck(oc, "cs-case65150-oci-multi-index", "openshift-marketplace", "Running")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-High-65151-mirror2disk and disk2mirror workflow for local oci catalog incremental  and prune testing [Serial]", func() {
		g.By("Set registry config")
		homePath := os.Getenv("HOME")
		dirname := homePath + "/case5151"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-65151")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-65151")
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("/tmp/redhat-operator-index")
		g.By("Copy the catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all docker://registry.redhat.io/redhat/redhat-operator-index:v4.16 oci://%s  --remove-signatures --insecure-policy  --authfile %s", "/tmp/redhat-operator-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ociFirstConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65151-1.yaml")
		ociSecondConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65151-2.yaml")
		for _, filename := range []string{ociFirstConfig, ociSecondConfig} {
			sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, filename)
			_, err = exec.Command("bash", "-c", sedCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		g.By("Start mirror2disk for the first time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFirstConfig, "file://"+dirname, "--v1").Execute()
			if err != nil {
				e2e.Logf("The first mirror2disk  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
		g.By("Start disk2mirror for the first time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("The first disk2mirror  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		g.By("Start mirror2disk for the second time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociSecondConfig, "file://"+dirname, "--v1").Execute()
			if err != nil {
				e2e.Logf("The second mirror2disk  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
		g.By("Start disk2mirror for the second time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq2_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if err != nil {
				e2e.Logf("The second disk2mirror  failed, retrying...")
				return false, nil
			}
			if !strings.Contains(output, "Deleting manifest") || strings.Contains(output, "secondary-scheduler-operator") {
				e2e.Failf("Don't find the prune logs and should not see logs about sso for incremental test")
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the second disk2mirror still failed")
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-NonPreRelease-Longduration-Critical-65202-Verify user is able to mirror multi payload via oc-mirror [Serial]", func() {
		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")

		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(strings.Contains(existingIcspOutput, "image-policy-aosqe")) {
			g.Skip("Image-policy-aosqe icsp not found, skipping the case")
		}

		buildPruningBaseDir := testdata.FixturePath("workloads")
		imageSetConfig65202 := filepath.Join(buildPruningBaseDir, "imageSetConfig65202.yaml")

		dirname, err := os.MkdirTemp("", "case65202-*")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		// err = locatePodmanCred(oc, dirname)
		// o.Expect(err).NotTo(o.HaveOccurred())
		authFile, _, err := locateDockerCred(oc, dirname)
		e2e.Logf("the auth file location: %s", authFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = appendPullSecretAuth(authFile, "quay.io", compat_otp.GetTestEnv().PullSecretLocation)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(readFileContent(authFile))

		// Retreive image registry name
		imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegistryName = strings.Split(imageRegistryName, ":")[0]
		e2e.Logf("ImageRegistryName is %s", imageRegistryName)

		imageSetConfigFilePath65202, err := replaceRegistryNameInCfg(imageSetConfig65202, imageRegistryName)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(imageSetConfigFilePath65202)

		defer os.RemoveAll("oc-mirror-workspace")
		// Start mirroring the payload
		g.By("Start mirroring the multi payload")
		var mirrorErr error
		waitErr := wait.PollUntilContextTimeout(context.Background(), 3*time.Minute, 30*time.Minute, true, func(ctx context.Context) (bool, error) {
			mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", imageSetConfigFilePath65202, "docker://"+imageRegistryName+":5000", "--dest-skip-tls", "--v1").Execute()
			if mirrorErr != nil {
				e2e.Logf("The multi payload mirroring failed...")
				return true, err
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the multipayload mirror still failed")
		o.Expect(mirrorErr).NotTo(o.HaveOccurred())
		// Validate if multi arch payload has been mirrored
		g.By("Validate if multi arch payload has been mirrored")
		o.Expect(assertMultiImage(imageRegistryName+":5000/openshift/release-images:4.19.16-multi", dirname+"/.dockerconfigjson")).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-NonPreRelease-Longduration-Critical-65203-Verify user is able to mirror multi payload along with single arch via oc-mirror [Serial]", func() {
		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")
		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(strings.Contains(existingIcspOutput, "image-policy-aosqe")) {
			g.Skip("Image-policy-aosqe icsp not found, skipping the case")
		}

		buildPruningBaseDir := testdata.FixturePath("workloads")
		imageSetConfig65203 := filepath.Join(buildPruningBaseDir, "imageSetConfig65203.yaml")

		dirname, err := os.MkdirTemp("", "case65203-*")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		// err = locatePodmanCred(oc, dirname)
		// o.Expect(err).NotTo(o.HaveOccurred())

		authFile, _, err := locateDockerCred(oc, dirname)
		e2e.Logf("the auth file location: %s", authFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = appendPullSecretAuth(authFile, "quay.io", compat_otp.GetTestEnv().PullSecretLocation)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(readFileContent(authFile))

		// Retreive image registry name
		imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegistryName = strings.Split(imageRegistryName, ":")[0]
		e2e.Logf("ImageRegistryName is %s", imageRegistryName)

		// Replace localhost with retreived registry name from the cluster in imageSetConfigFile
		imageSetConfigFilePath65203, err := replaceRegistryNameInCfg(imageSetConfig65203, imageRegistryName)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(imageSetConfigFilePath65203)

		// Start mirroring the payload
		g.By("Start mirroring the multi payload")
		cwd, err := os.Getwd()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = os.Chdir(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func(dir string) {
			err := os.Chdir(dir)
			o.Expect(err).NotTo(o.HaveOccurred())
		}(cwd)
		var mirrorErr error
		waitErr := wait.PollImmediate(3*time.Minute, 30*time.Minute, func() (bool, error) {
			mirrorErr = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", imageSetConfigFilePath65203, "docker://"+imageRegistryName+":5000", "--dest-skip-tls", "--v1").Execute()
			if mirrorErr != nil {
				e2e.Logf("The first multi payload mirroring failed")
				return true, mirrorErr
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the multipayload mirror still failed")
		o.Expect(mirrorErr).NotTo(o.HaveOccurred())

		// Validate if multi arch payload has been mirrored
		g.By("Validate if multi arch payload has been mirrored")
		archList := []architecture.Architecture{architecture.AMD64, architecture.ARM64, architecture.PPC64LE,
			architecture.S390X, architecture.MULTI}
		for _, arch := range archList {
			matcher := o.BeFalse()
			if arch == architecture.MULTI {
				matcher = o.BeTrue()
			}
			o.Expect(assertMultiImage(fmt.Sprintf("%s:5000/openshift/release-images:4.19.16-%s", imageRegistryName, arch.GNUString()), dirname+"/.dockerconfigjson")).To(matcher)
		}

	})

	//author yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-66194-High-66195-oc-mirror support multi-arch catalog for docker format [Serial]", func() {
		g.By("Set podman registry config")
		dirname := "/tmp/case66194"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Starting mirror2mirror")
		defer os.RemoveAll("oc-mirror-workspace")
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		operatorConfigS := filepath.Join(ocmirrorBaseDir, "config-66194.yaml")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, "oc-mirror command still falied")
		g.By("Check the mirrored image should still with multi-arch")
		o.Expect(assertMultiImage(serInfo.serviceName+"/cpopen/ibm-zcon-zosconnect-catalog:6f02ec", dirname+"/.dockerconfigjson")).To(o.BeTrue())

		g.By("Starting mirror2disk")
		defer os.RemoveAll("66195out")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "file://66195out", "--v1").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, "mirror2disk command still falied")

		g.By("Starting disk2mirror")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "66195out/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName+"/disktomirror", "--dest-skip-tls", "--v1").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(err, "disk2mirror command still falied")

		g.By("Check the mirrored image should still with multi-arch")
		o.Expect(assertMultiImage(serInfo.serviceName+"/disktomirror/cpopen/ibm-zcon-zosconnect-catalog:6f02ec", dirname+"/.dockerconfigjson")).To(o.BeTrue())
	})

	//author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Critical-65152-mirror2mirror workflow for local  multi-oci catalog [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case65152"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-65152")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-65152")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy the multi-arch catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all --format oci docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures --insecure-policy --authfile %s", dirname+"/oci-multi-index", dirname+"/.dockerconfigjson")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the skopeo copy still failed")
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65152.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		os.RemoveAll("oc-mirror-workspace")
		g.By("Starting mirror2mirror ....")
		waitErr = wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		defer removeCSAndISCP(oc)
		g.By("Starting createCSAndISCPNoPackageCheck ....")
		createCSAndISCPNoPackageCheck(oc, "cs-case65152-oci-multi-index", "openshift-marketplace", "Running")
		g.By("Starting installOperatorFromCustomCS ....")
		deschedulerSub, deschedulerOG := getOperatorInfo(oc, "cluster-kube-descheduler-operator", "openshift-kube-descheduler-operator-ns", "registry.redhat.io/redhat/redhat-operator-index:v4.13", "cs-case65152-oci-multi-index")
		defer removeOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator-ns")
		installOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator-ns", "descheduler-operator")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-High-66869-Medium-66871-oc mirror could mirror and install operator for catalog redhat-marketplace-index [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case66869"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-66869")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-66869")
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetConfig := filepath.Join(ocmirrorBaseDir, "config-66869.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		defer removeCSAndISCP(oc)
		g.By("createCSAndISCPNoPackageCheck")
		createCSAndISCPNoPackageCheck(oc, "cs-redhat-marketplace-index", "openshift-marketplace", "Running")
		g.By("installCustomOperator")
		aerospikeSub, aerospikeOG := getOperatorInfo(oc, "aerospike-kubernetes-operator-rhmp", "aerospike-ns", "registry.redhat.io/redhat/redhat-marketplace-index:v4.16", "cs-redhat-marketplace-index")
		defer removeOperatorFromCustomCS(oc, aerospikeSub, aerospikeOG, "aerospike-ns")
		installCustomOperator(oc, aerospikeSub, aerospikeOG, "aerospike-ns", "aerospike-operator-controller-manager", "2")
		g.By("installAllNSOperatorFromCustomCS")
		nsconfigSub, nsconfigOG := getOperatorInfo(oc, "namespace-configuration-operator", "namespace-configuration-operator", "registry.redhat.io/redhat/community-operator-index:v4.16", "cs-community-operator-index")
		defer removeOperatorFromCustomCS(oc, nsconfigSub, nsconfigOG, "namespace-configuration-operator")
		installAllNSOperatorFromCustomCS(oc, nsconfigSub, nsconfigOG, "namespace-configuration-operator", "namespace-configuration-operator-controller-manager", "1")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-High-66870-oc mirror could mirror and install operator for catalog certified-operator-index [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case66870"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-66870")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-66870")
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetConfig := filepath.Join(ocmirrorBaseDir, "config-66870.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		defer removeCSAndISCP(oc)
		createCSAndISCPNoPackageCheck(oc, "cs-certified-operator-index", "openshift-marketplace", "Running")
		nginxSub, nginxOG := getOperatorInfo(oc, "nginx-ingress-operator", "nginx-ingress-operator-ns", "registry.redhat.io/redhat/certified-operator-index:v4.16", "cs-certified-operator-index")
		defer removeOperatorFromCustomCS(oc, nginxSub, nginxOG, "nginx-ingress-operator-ns")
		installOperatorFromCustomCS(oc, nginxSub, nginxOG, "nginx-ingress-operator-ns", "nginx-ingress-operator-controller-manager")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-High-70047-Medium-70052-oc-mirror requires that the default channel of an operator is mirrored [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case70047"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
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
		defer restoreAddCA(oc, addCA, "trusted-ca-70047")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-70047")
		o.Expect(err).NotTo(o.HaveOccurred())
		publicRegistry := serInfo.serviceName

		compat_otp.By("Get the default channel for special package")
		getOperatorDefaultChannelCMD := fmt.Sprintf("oc-mirror list operators --v1 --catalog %s | awk '$1~/^%s/ {print $NF}'", "registry.redhat.io/redhat/redhat-operator-index:v4.14", "elasticsearch-operator")
		e2e.Logf("getOperatorDefaultChannelCMD: %s", getOperatorDefaultChannelCMD)
		channel, err := exec.Command("bash", "-c", getOperatorDefaultChannelCMD).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defaultChannelName := strings.ReplaceAll(string(channel), "\n", "")
		e2e.Logf("the default name %v", defaultChannelName)

		compat_otp.By("Get the channel list for special package exclude the default channel")
		getOperatorChannelCMD := fmt.Sprintf("oc-mirror list operators --v1 --catalog %s  --package %s |awk '$2 != \"%s\"{print}' |awk '$2~/^stable*/ {print $2}'", "registry.redhat.io/redhat/redhat-operator-index:v4.14", "elasticsearch-operator", defaultChannelName)
		e2e.Logf("getOperatorChannelCMD: %s", getOperatorChannelCMD)
		channelList, err := exec.Command("bash", "-c", getOperatorChannelCMD).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("the channel list raw %v", string(channelList))
		channelNameList := strings.Fields(strings.ReplaceAll(string(channelList), "\n", " "))
		o.Expect(channelNameList).NotTo(o.BeEmpty())
		e2e.Logf("the channel list %v", channelNameList)

		invalidImageSetYamlFile := dirname + "/invalidimagesetconfig.yaml"
		invalidImageSetYaml := fmt.Sprintf(`apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  operators:
    - catalog: %s
      packages:
        - name: %s
          defaultChannel: %s
          channels:
            - name: %s
`, "registry.redhat.io/redhat/redhat-operator-index:v4.14", "elasticsearch-operator", channelNameList[0], channelNameList[1])
		compat_otp.By("2 Create a invalid imageset configure file")
		f, err := os.Create(invalidImageSetYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, werr := w.WriteString(invalidImageSetYaml)
		w.Flush()
		o.Expect(werr).NotTo(o.HaveOccurred())
		_, outerr, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", invalidImageSetYamlFile, "docker://"+publicRegistry, "--dest-skip-tls", "--v1").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(outerr).To(o.ContainSubstring("defaultChannel has been set with"))
		imageSetYamlFile := dirname + "/imagesetconfig.yaml"
		imageSetYaml := fmt.Sprintf(`apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  operators:
    - catalog: %s
      packages:
        - name: %s
          defaultChannel: %s
          channels:
            - name: %s
            - name: %s
`, "registry.redhat.io/redhat/redhat-operator-index:v4.14", "elasticsearch-operator", channelNameList[0], channelNameList[0], channelNameList[1])

		compat_otp.By("3 Create a valid imageset configure file")
		imageSetF, err := os.Create(imageSetYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer imageSetF.Close()
		imageSetW := bufio.NewWriter(imageSetF)
		_, werr = imageSetW.WriteString(imageSetYaml)
		imageSetW.Flush()
		o.Expect(werr).NotTo(o.HaveOccurred())

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll(".oc-mirror.log")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFile, "docker://"+publicRegistry, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		defer removeCSAndISCP(oc)
		createCSAndISCPNoPackageCheck(oc, "cs-redhat-operator-index", "openshift-marketplace", "Running")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Medium-70105-oc-mirror should ignore the sequence check when use --skip-pruning [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case70105"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		compat_otp.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		publicRegistry := serInfo.serviceName
		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-70105-first.yaml")
		imageSetYamlFileS := filepath.Join(ocmirrorBaseDir, "config-70105-second.yaml")
		imageSetYamlFileT := filepath.Join(ocmirrorBaseDir, "config-70105-third.yaml")

		compat_otp.By("1 The first mirror")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("output70105")
		defer os.RemoveAll(".oc-mirror.log")

		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://output70105", "--v1").Execute()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		compat_otp.By("2 The second mirror")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileS, "file://output70105", "--v1").Execute()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		compat_otp.By("3 The third mirror")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileT, "file://output70105", "--v1").Execute()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		compat_otp.By("4 Mirror the tar by sequence")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "output70105/mirror_seq1_000000.tar", "docker://"+publicRegistry+"/sequence2", "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "output70105/mirror_seq2_000000.tar", "docker://"+publicRegistry+"/sequence2", "--dest-skip-tls", "--v1").Output()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			if matched, _ := regexp.MatchString("Deleting manifest", out); !matched {
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "output70105/mirror_seq3_000000.tar", "docker://"+publicRegistry+"/sequence2", "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		compat_otp.By("5 Mirror the tar without sequence and skip-pruning")
		_, outerr, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "output70105/mirror_seq3_000000.tar", "docker://"+publicRegistry+"/nonesequence3", "--dest-skip-tls", "--v1").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(outerr).To(o.ContainSubstring("invalid mirror sequence order"))

		compat_otp.By("6 Mirror the tar without sequence but with skip-pruning")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "output70105/mirror_seq3_000000.tar", "docker://"+publicRegistry+"/nonesequence3", "--skip-pruning", "--dest-skip-tls", "--v1").Output()
			if err != nil {
				e2e.Logf("Mirror operator failed, retrying...")
				return false, nil
			}
			if matched, _ := regexp.MatchString("skipped pruning", out); !matched {
				return false, nil
			}
			return true, nil

		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-NonPreRelease-Longduration-Critical-75502-make sure IBM Operator Index pod always works fine when mirrored by oc-mirror [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case75502"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create an internal registry")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)
		e2e.Logf("Registry is %s", registry)

		g.By("Configure the Registry Certificate as trusted for cincinnati")
		addCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("image.config.openshift.io/cluster", "-o=jsonpath={.spec.additionalTrustedCA}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer restoreAddCA(oc, addCA, "trusted-ca-75502")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-75502")
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetConfig := filepath.Join(ocmirrorBaseDir, "config-75502.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		waitErr := wait.PollImmediate(300*time.Second, 600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Execute()
			if err != nil {
				e2e.Logf("mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		defer removeCSAndISCP(oc)
		createCSAndISCPNoPackageCheck(oc, "cs-ibm-operator-catalog", "openshift-marketplace", "Running")
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-70861-Should return error code when oc-mirror hit operator not found [Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case70861"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-70861.yaml")
		defer os.RemoveAll("output70861")
		defer os.RemoveAll(".oc-mirror.log")
		compat_otp.By("Mirror should failed when operator not found")
		_, outerr, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://output70861", "--dry-run", "--v1").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(outerr).To(o.ContainSubstring("Operator cluster-logging was not found"))
		compat_otp.By("Mirror should not failed when operator not found as used continue-on-error")
		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://output70861", "--dry-run", "--continue-on-error", "--v1").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("Operator cluster-logging was not found"))
	})

	g.It("Author:yinzhou-NonHyperShiftHOST-NonPreRelease-Longduration-Medium-70858-High-66986-Make sure archiveSize is 1 in fully disconnected Openshift cluster works well for oc-mirror[Serial]", func() {
		compat_otp.By("Set registry config")
		dirname := "/tmp/case70858"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		ocmirrorBaseDir := testdata.FixturePath("workloads")
		imageSetYamlFileF := filepath.Join(ocmirrorBaseDir, "config-66986.yaml")
		defer os.RemoveAll("output66986")
		defer os.RemoveAll(".oc-mirror.log")
		defer os.RemoveAll("oc-mirror-workspace")
		err = getRouteCAToFile(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Create an internal registry")
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
		defer restoreAddCA(oc, addCA, "trusted-ca-66986")
		err = trustCert(oc, serInfo.serviceName, dirname+"/tls.crt", "trusted-ca-66986")
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Start mirror2disk")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", imageSetYamlFileF, "file://output66986", "--v1").Execute()
			if err != nil {
				e2e.Logf("The mirror2disk  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
		compat_otp.By("Start disk2mirror")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "output66986/", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--v1").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if err != nil {
				e2e.Logf("The disk2mirror  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		defer removeCSAndISCP(oc)
		compat_otp.By("createCSAndISCPNoPackageCheck")
		createCSAndISCPNoPackageCheck(oc, "cs-certified-operator-index", "openshift-marketplace", "Running")

		compat_otp.By("installAllNSOperatorFromCustomCS")
		portworxSub, portworxOG := getOperatorInfo(oc, "nginx-ingress-operator", "70858-ns", "registry.redhat.io/redhat/certified-operator-index:v4.19", "cs-certified-operator-index")
		defer removeOperatorFromCustomCS(oc, portworxSub, portworxOG, "70858-ns")
		installOperatorFromCustomCS(oc, portworxSub, portworxOG, "70858-ns", "nginx-ingress-operator-controller-manager")
	})
})
