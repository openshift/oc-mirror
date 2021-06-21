module github.com/RedHatGov/bundle

go 1.16

require (
	github.com/google/uuid v1.2.0
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/openshift/cluster-version-operator v1.0.1-0.20200804150713-ea6899ae6f7c
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.6.1 // indirect
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	golang.org/x/sys v0.0.0-20210225134936-a50acf3fe073 // indirect
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.8.0
)

exclude (
	github.com/kubevirt/terraform-provider-kubevirt v0.0.0-00010101000000-000000000000
	github.com/metal3-io/baremetal-operator v0.0.0-20210422153428-d22c5f710cdc
	github.com/metal3-io/cluster-api-provider-baremetal v0.0.0
	github.com/tencentcloud/tencentcloud-sdk-go v3.0.82+incompatible
	github.com/terraform-providers/terraform-provider-ignition/v2 v2.1.0
	kubevirt.io/client-go v0.0.0-00010101000000-000000000000
	sigs.k8s.io/cluster-api-provider-aws v0.0.0
	sigs.k8s.io/cluster-api-provider-aws v0.0.0-00010101000000-000000000000
	sigs.k8s.io/cluster-api-provider-azure v0.0.0
	sigs.k8s.io/cluster-api-provider-azure v0.0.0-00010101000000-000000000000
	sigs.k8s.io/cluster-api-provider-openstack v0.0.0
)
