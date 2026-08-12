package image

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
)

func TestValidateDockerDestinationRegistry(t *testing.T) {
	tests := []struct {
		name        string
		dest        string
		expectError string
	}{
		{
			name: "lowercase hostname",
			dest: consts.DockerProtocol + "registry.example.com:5000",
		},
		{
			name: "lowercase hostname with namespace",
			dest: consts.DockerProtocol + "registry.example.com:5000/ns",
		},
		{
			name: "ipv4 destination",
			dest: consts.DockerProtocol + "192.168.1.10:5000",
		},
		{
			name: "ipv6 destination with uppercase hex",
			dest: consts.DockerProtocol + "[2001:DB8::1]:5000",
		},
		{
			name: "non-docker destination is ignored",
			dest: consts.FileProtocol + "archive",
		},
		{
			name:        "uppercase hostname",
			dest:        consts.DockerProtocol + "REGISTRY.EXAMPLE.COM:5000",
			expectError: `destination registry hostname "REGISTRY.EXAMPLE.COM" contains uppercase characters; use lowercase (e.g. "registry.example.com") because CRI-O rejects uppercase registry hostnames`,
		},
		{
			name:        "mixed case hostname with namespace",
			dest:        consts.DockerProtocol + "Registry.Example.Com/ns",
			expectError: `destination registry hostname "Registry.Example.Com" contains uppercase characters; use lowercase (e.g. "registry.example.com") because CRI-O rejects uppercase registry hostnames`,
		},
		{
			name:        "empty hostname",
			dest:        consts.DockerProtocol,
			expectError: "destination registry hostname is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDockerDestinationRegistry(tt.dest)
			if tt.expectError == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.expectError)
		})
	}
}

func TestRegistryHostname(t *testing.T) {
	assert.Equal(t, "registry.example.com", registryHostname("registry.example.com:5000/ns"))
	assert.Equal(t, "registry.example.com", registryHostname("registry.example.com/ns"))
	assert.Equal(t, "[::1]", registryHostname("[::1]:5000/ns"))
	assert.Equal(t, "[2001:db8::1]", registryHostname("[2001:db8::1]/ns"))
	assert.Equal(t, "", registryHostname(""))
	assert.Equal(t, "", registryHostname("/ns"))
}
