package bundle

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

var (
	bundleExample = `
	# Reference a bundle-config.yaml to create a new full OCP bundle.
	%[1]s bundle create full --dir=bundle

	# Reference a bundle-config.yaml to create a new full OCP bundle.
	%[1]s bundle create diff --dir=bundle

	# Reference a bundle-config.yaml to create a new full OCP bundle.
	%[1]s bundle publish --from-bundle=bundle.x.y.z.tar.gz --to-directory=v2-directory --to-mirror=registry.url.local:5000 --install
`
)

const (
	configName string = "/bundle-config.yaml"
	metadata   string = "/src/publish/.meta"
)

func readBundleConfig(rootDir string) (*BundleSpec, error) {
	buf, err := ioutil.ReadFile(rootDir + configName)
	if err != nil {
		return nil, err
	}

	c := &BundleSpec{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		return nil, fmt.Errorf("in file %q: %v", configName, err)
	}

	return c, nil
}
