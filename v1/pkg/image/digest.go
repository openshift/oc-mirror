package image

import (
	"fmt"

	"github.com/opencontainers/go-digest"
)

// getPartialDigest returns the first 6 characters of a digest,
// if valid. The purpose is to use this as a unique image tag.
// The since the algorithm and deletimeter are the first 7 characters,
// range 7-13 is used here.
// Ex. sha256:fc07c1e2a5f012320ae672ca8546ff0d09eb8dba3c5acbbfc426c7984169ee84
// would result in fc07c1.
func getPartialDigest(d string) (string, error) {
	_, err := digest.Parse(d)
	if err != nil {
		return "", err
	}

	tagStart := 7
	tagEnd := 13

	if len(d) < tagEnd {
		return "", fmt.Errorf("digest %q does not meet length requirements for partial calculations", d)
	}
	return d[tagStart:tagEnd], nil
}
