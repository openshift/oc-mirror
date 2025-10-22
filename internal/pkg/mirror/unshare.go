// go: build !windows
// build !windows
package mirror

import (
	"fmt"

	"github.com/syndtr/gocapability/capability"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/storage/pkg/unshare"
)

var neededCapabilities = []capability.Cap{
	capability.CAP_CHOWN,
	capability.CAP_DAC_OVERRIDE,
	capability.CAP_FOWNER,
	capability.CAP_FSETID,
	capability.CAP_MKNOD,
	capability.CAP_SETFCAP,
	capability.CAP_SYS_ADMIN,
}

func maybeReexec() error {
	// With Skopeo we need only the subset of the root capabilities necessary
	// for pulling an image to the storage.  Do not attempt to create a namespace
	// if we already have the capabilities we need.
	capabilities, err := capability.NewPid2(0)
	if err != nil {
		return fmt.Errorf("error reading the current capabilities sets: %w", err)
	}
	for _, cap := range neededCapabilities {
		if !capabilities.Get(capability.EFFECTIVE, cap) {
			// We miss a capability we need, create a user namespaces
			unshare.MaybeReexecUsingUserNamespace(true)
			return nil
		}
	}
	return nil
}

func ReexecIfNecessaryForImages(imageNames ...string) error {
	// Check if container-storage is used before doing unshare
	for _, imageName := range imageNames {
		transport := alltransports.TransportFromImageName(imageName)
		// Hard-code the storage name to avoid a reference on c/image/storage.
		// See https://github.com/containers/skopeo/issues/771#issuecomment-563125006.
		if transport != nil && transport.Name() == "containers-storage" {
			return maybeReexec()
		}
	}
	return nil
}
