package image

import "github.com/opencontainers/go-digest"

// getPartialDigest returns the first 6 characters of a digest
// if valid.
func getPartialDigest(d string) (string, error) {
	_, err := digest.Parse(d)
	if err != nil {
		return "", err
	}
	return d[7:13], nil
}
