module github.com/RedHatGov/bundle

go 1.16

require (
	github.com/Shopify/logrus-bugsnag v0.0.0-20171204204709-577dee27f20d // indirect
	github.com/blang/semver/v4 v4.0.0
	github.com/bshuster-repo/logrus-logstash-hook v1.0.2 // indirect
	github.com/containerd/containerd v1.4.8
	github.com/containers/image/v5 v5.15.2
	github.com/docker/distribution v2.7.1+incompatible
	github.com/go-git/go-git/v5 v5.4.2
	github.com/google/uuid v1.2.0
	github.com/joelanford/ignore v0.0.0-20210610194209-63d4919d8fb2
	github.com/mattn/go-sqlite3 v1.14.8 // indirect
	github.com/mholt/archiver/v3 v3.5.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/openshift/installer v0.16.1
	github.com/openshift/library-go v0.0.0-20210831102543-1a08f0c3bd9a
	github.com/openshift/oc v0.0.0-alpha.0.0.20210721184532-4df50be4d929
	github.com/operator-framework/operator-registry v1.18.1-0.20210917182743-3880486cea2b
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.6.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	k8s.io/apimachinery v0.22.1
	k8s.io/cli-runtime v0.22.0
	k8s.io/client-go v0.22.1
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.9.0
	k8s.io/kubectl v0.22.0
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Microsoft/hcsshim => github.com/Microsoft/hcsshim v0.8.7
	github.com/apcera/gssapi => github.com/openshift/gssapi v0.0.0-20161010215902-5fb4217df13b
	k8s.io/apimachinery => github.com/openshift/kubernetes-apimachinery v0.0.0-20210730111815-c26349f8e2c9
	k8s.io/cli-runtime => github.com/openshift/kubernetes-cli-runtime v0.0.0-20210730111823-1570202448c3
	k8s.io/client-go => github.com/openshift/kubernetes-client-go v0.0.0-20210730111819-978c4383ac68
	k8s.io/kubectl => github.com/openshift/kubernetes-kubectl v0.0.0-20210730111826-9c6734b9d97d
)
