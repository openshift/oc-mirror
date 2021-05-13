package bundle

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

// Define command and sub-commands
