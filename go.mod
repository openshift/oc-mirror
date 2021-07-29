module github.com/RedHatGov/bundle

go 1.16

require (
	github.com/Shopify/logrus-bugsnag v0.0.0-20171204204709-577dee27f20d // indirect
	github.com/blang/semver/v4 v4.0.0
	github.com/bshuster-repo/logrus-logstash-hook v1.0.2 // indirect
	github.com/google/uuid v1.2.0
	github.com/openshift/library-go v0.0.0-20210521084623-7392ea9b02ca
	github.com/openshift/oc v0.0.0-alpha.0.0.20210721184532-4df50be4d929
	github.com/operator-framework/operator-registry v0.0.0-00010101000000-000000000000
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	k8s.io/apimachinery v0.21.1
	k8s.io/cli-runtime v0.21.1
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.8.0
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Microsoft/hcsshim => github.com/Microsoft/hcsshim v0.8.7
	github.com/apcera/gssapi => github.com/openshift/gssapi v0.0.0-20161010215902-5fb4217df13b
	k8s.io/apimachinery => github.com/openshift/kubernetes-apimachinery v0.0.0-20210521074607-b6b98f7a1855
	k8s.io/cli-runtime => github.com/openshift/kubernetes-cli-runtime v0.0.0-20210521074950-112a61d2624f
	k8s.io/client-go => github.com/openshift/kubernetes-client-go v0.0.0-20210521075216-71b63307b5df
	k8s.io/kubectl => github.com/openshift/kubernetes-kubectl v0.0.0-20210521075729-633333dfccda
)

// TODO(estroz): remove this once main operator-registry repo exposes libs needed.
replace github.com/operator-framework/operator-registry => github.com/estroz/operator-registry v1.3.1-0.20210723182709-28179ca932d8
