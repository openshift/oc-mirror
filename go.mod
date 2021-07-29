module github.com/RedHatGov/bundle

go 1.16

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/google/uuid v1.2.0
	github.com/openshift/library-go v0.0.0-20210521084623-7392ea9b02ca
	github.com/openshift/oc v0.0.0-alpha.0.0.20210612233142-1077b0516d5b
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/cli-runtime v0.21.1
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.8.0
)

replace (
	github.com/Microsoft/hcsshim => github.com/Microsoft/hcsshim v0.8.7
	github.com/apcera/gssapi => github.com/openshift/gssapi v0.0.0-20161010215902-5fb4217df13b
	k8s.io/apimachinery => github.com/openshift/kubernetes-apimachinery v0.0.0-20210521074607-b6b98f7a1855
	k8s.io/cli-runtime => github.com/openshift/kubernetes-cli-runtime v0.0.0-20210521074950-112a61d2624f
	k8s.io/client-go => github.com/openshift/kubernetes-client-go v0.0.0-20210521075216-71b63307b5df
	k8s.io/kubectl => github.com/openshift/kubernetes-kubectl v0.0.0-20210521075729-633333dfccda
)
