package image

import (
	"fmt"
	"net"
	"strings"
	"unicode"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
)

// ValidateDockerDestinationRegistry validates a docker:// destination argument.
// CRI-O rejects uppercase letters in registry hostnames, so oc-mirror fails early
// rather than producing mirrored content that cluster nodes cannot pull (OCPBUGS-78497).
func ValidateDockerDestinationRegistry(dest string) error {
	if !strings.HasPrefix(dest, consts.DockerProtocol) {
		return nil
	}

	ref := strings.TrimPrefix(dest, consts.DockerProtocol)
	hostname := registryHostname(ref)
	if hostname == "" {
		return fmt.Errorf("destination registry hostname is empty")
	}

	// IP literals (IPv4 / IPv6) are not subject to DNS hostname case rules.
	// IPv6 hex digits may be uppercase and remain valid.
	if hostForIP := hostname; strings.HasPrefix(hostForIP, "[") && strings.HasSuffix(hostForIP, "]") {
		hostForIP = hostForIP[1 : len(hostForIP)-1]
		if net.ParseIP(hostForIP) != nil {
			return nil
		}
	} else if net.ParseIP(hostname) != nil {
		return nil
	}

	for _, r := range hostname {
		if unicode.IsUpper(r) {
			return fmt.Errorf("destination registry hostname %q contains uppercase characters; use lowercase (e.g. %q) because CRI-O rejects uppercase registry hostnames", hostname, strings.ToLower(hostname))
		}
	}
	return nil
}

// registryHostname returns the host (without path) from a docker destination
// reference such as "registry.example.com:5000/ns" or "[::1]:5000/ns".
func registryHostname(ref string) string {
	hostPort := ref
	if i := strings.Index(ref, "/"); i >= 0 {
		hostPort = ref[:i]
	}
	if hostPort == "" {
		return ""
	}

	if strings.HasPrefix(hostPort, "[") {
		end := strings.Index(hostPort, "]")
		if end < 0 {
			return hostPort
		}
		return hostPort[:end+1]
	}

	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return host
	}
	return hostPort
}
