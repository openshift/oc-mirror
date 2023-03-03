package mirror

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/klog/v2"
)

const mappingFile = "mapping.txt"

// func getCraneOpts(ctx context.Context, insecure bool) (options []crane.Option) {
// 	options = []crane.Option{
// 		crane.WithAuthFromKeychain(authn.DefaultKeychain),
// 		crane.WithTransport(createRT(insecure)),
// 		crane.WithContext(ctx),
// 	}
// 	if insecure {
// 		options = append(options, crane.Insecure)
// 	}
// 	return
// }

func getRemoteOpts(ctx context.Context, insecure bool) []remote.Option {
	return []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(createRT(insecure)),
		remote.WithContext(ctx),
	}
}

func getNameOpts(insecure bool) (options []name.Option) {
	if insecure {
		options = append(options, name.Insecure)
	}
	return options
}

func createRT(insecure bool) http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			// By default, we wrap the transport in retries, so reduce the
			// default dial timeout to 5s to avoid 5x 30s of connection
			// timeouts when doing the "ping" on certain http registries.
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
			MinVersion:         tls.VersionTLS12,
		},
	}
}

func (o *MirrorOptions) createResultsDir() (resultsDir string, err error) {
	resultsDir = filepath.Join(
		o.Dir,
		fmt.Sprintf("results-%v", time.Now().Unix()),
	)
	if err := os.MkdirAll(resultsDir, os.ModePerm); err != nil {
		return resultsDir, err
	}
	return resultsDir, nil
}

func (o *MirrorOptions) newMetadataImage(uid string) string {
	repo := path.Join(o.ToMirror, o.UserNamespace, "oc-mirror")
	return fmt.Sprintf("%s:%s", repo, uid)
}

func getTLSConfig() (*tls.Config, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}
	return config, nil
}

func (o *MirrorOptions) checkErr(err error, acceptableErr func(error) bool, logMessage func(error) string) error {

	if err == nil {
		return nil
	}

	var skip, skipAllTypes bool
	if acceptableErr != nil {
		skip = acceptableErr(err)
	} else {
		skipAllTypes = true
	}

	message := err.Error()
	if logMessage != nil {
		message = logMessage(err)
	}

	// Instead of returning an error, just log it.
	if o.ContinueOnError && (skip || skipAllTypes) {
		klog.Errorf("error: %v", message)
		o.continuedOnError = true
	} else {
		return fmt.Errorf("%v", message)
	}

	return nil
}

func getPlatformKey(typeWithPlatorm interface{}) (*OperatorCatalogPlatform, error) {
	if typeWithPlatorm == nil {
		return nil, errors.New("no manifest or config provided, so unable to generate a platform key")
	}
	switch v := typeWithPlatorm.(type) {
	case *v1.Platform:
		return &OperatorCatalogPlatform{
			os:           v.OS,
			architecture: v.Architecture,
			variant:      v.Variant,
			isIndex:      true,
		}, nil
	case *v1.Descriptor:
		if v.Platform == nil {
			return nil, errors.New("no platform provided in descriptor, so unable to generate a platform key")
		}
		return &OperatorCatalogPlatform{
			os:           v.Platform.OS,
			architecture: v.Platform.Architecture,
			variant:      v.Platform.Variant,
			isIndex:      true,
		}, nil
	case *v1.ConfigFile:
		// ConfigFile only comes into play when an image is a single architecture catalog.
		// Callers may need to override the isIndex field if necessary
		return &OperatorCatalogPlatform{
			os:           v.OS,
			architecture: v.Architecture,
			variant:      v.Variant,
			isIndex:      false,
		}, nil
	default:
		// should not happen... this is a programmer error
		return nil, fmt.Errorf("expected a manifest or config, but %T provided, so unable to generate a platform key", v)
	}
}

/*
getImageDigests will fetch one or more image digests for a given image reference.
This function handles both "manifest list" and "image" references. A "manifest list"
can return one or more digest references, whereas a "image" reference only returns a single
digest reference.

# Arguments

• ctx: A cancellation context

• imageRef: a docker image reference which will be either remotely accessed (if necessary)
and used as a basis for the image references returned from this function

• layoutPath: an optional OCI layout path. If provided imageRef will NOT be fetched remotely.
This OCI layout path could refer to a "manifest list" or an "image".

• insecure: flag to indicate if plain http is used or when skipping TLS verification

# Returns

• map[OperatorCatalogPlatform]CatalogMetadata: If no error occurs, returns a map whose key is OperatorCatalogPlatform
and value is CatalogMetadata with its image digest references set.
If an error occurs the map will always be initialized (i.e non-nil) but could have partial results.

• error: non-nil if an error occurs, nil otherwise
*/
func getImageDigests(ctx context.Context, imageRef string, layoutPath *layout.Path, insecure bool) (map[OperatorCatalogPlatform]CatalogMetadata, error) {

	// initialize return values
	digestsMap := map[OperatorCatalogPlatform]CatalogMetadata{}

	reference, err := name.ParseReference(imageRef, getNameOpts(insecure)...)
	if err != nil {
		return digestsMap, err
	}

	// function to update the digestsMap
	updateDigestsMap := func(platformKey *OperatorCatalogPlatform, digestReference *name.Digest) error {
		if platformKey == nil {
			return errors.New("no platform key was provided, unable to update map")
		}

		if existingCatalogMetadata, exists := digestsMap[*platformKey]; exists {
			// we'll be updating the existing digests
			existingCatalogMetadata.catalogRef = digestReference
			digestsMap[*platformKey] = existingCatalogMetadata
		} else {
			// does not exist yet, initialize
			digestsMap[*platformKey] = CatalogMetadata{
				catalogRef: digestReference,
			}
		}
		return nil
	}

	processImage := func(img v1.Image, digestRef name.Digest, flagAsManifestList bool) error {
		config, err := img.ConfigFile()
		if err != nil {
			return err
		}
		platformKey, err := getPlatformKey(config)
		if err != nil {
			return err
		}
		// override the isIndex field if flag is set
		if flagAsManifestList {
			platformKey.isIndex = true
		}
		updateDigestsMap(platformKey, &digestRef)
		return nil
	}
	// processImageIndex is recursive, so needs to be defined as a var here
	var processImageIndex func(imageIndex v1.ImageIndex) error
	processImageIndex = func(imageIndex v1.ImageIndex) error {
		// get the manifest for the index
		indexManifest, err := imageIndex.IndexManifest()
		if err != nil {
			return err
		}

		// media type in this scenario is not guaranteed to be present, but if it is, use it
		flagAsManifestList := false
		if indexManifest.MediaType != "" && indexManifest.MediaType.IsIndex() {
			flagAsManifestList = true
		}
		for _, manifest := range indexManifest.Manifests {
			digestReference := reference.Context().Digest(manifest.Digest.String())

			// if this is an image, and we don't have platform information,
			// we'll have to delegate to processImage
			if manifest.MediaType.IsImage() && manifest.Platform == nil {
				// delegate to processImage
				img, err := imageIndex.Image(manifest.Digest)
				if err != nil {
					return err
				}
				err = processImage(img, digestReference, flagAsManifestList)
				if err != nil {
					return err
				}
				// continue with next manifest
				continue
			}
			// if we can get the platform from here, do so
			if manifest.Platform != nil {
				platformKey, err := getPlatformKey(manifest.Platform)
				if err != nil {
					return err
				}
				updateDigestsMap(platformKey, &digestReference)
			} else if manifest.MediaType.IsIndex() {
				// get the inner image index and recursively process it
				innerImageIndex, err := imageIndex.ImageIndex(manifest.Digest)
				if err != nil {
					return err
				}
				err = processImageIndex(innerImageIndex)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	// if we have a layoutPath, handle as an OCI layout and not as a docker image reference
	if layoutPath != nil {
		// OCI layouts are by definition an image index, but the image manifests may point to
		// a single image or a "manifest list"
		imageIndex, err := layoutPath.ImageIndex()
		if err != nil {
			return digestsMap, err
		}
		return digestsMap, processImageIndex(imageIndex)
	}

	// handle as docker image reference
	descriptor, err := remote.Get(reference, getRemoteOpts(ctx, insecure)...)
	if err != nil {
		return digestsMap, err
	}
	mediaType := descriptor.MediaType
	if mediaType.IsIndex() {
		// fetch the imageIndex (i.e. manifest list)
		imageIndex, err := remote.Index(reference, getRemoteOpts(ctx, false)...)
		if err != nil {
			return digestsMap, err
		}
		return digestsMap, processImageIndex(imageIndex)
	} else if mediaType.IsImage() {
		img, err := descriptor.Image()
		if err != nil {
			return digestsMap, err
		}
		digestReference := reference.Context().Digest(descriptor.Digest.String())
		return digestsMap, processImage(img, digestReference, false)
	}

	// should probably never get here... it means that the media type was not provided
	// at all, or was a type that's unexpected.
	return digestsMap, fmt.Errorf("unknown media type %q encountered", descriptor.MediaType)
}

/*
getDigestFromOCILayout obtains the hash of an image for a given platform within the specified layout.
If no match is found for the given platform, then no hash is returned. The first match that is discovered
is returned.

# Arguments

• ctx: A cancellation context

• layoutPath: an OCI layout path, which could refer to a "manifest list" or an "image"

• platformIn: the platform to search for

# Returns

• *v1.Hash: a non-nil value if a match was found, nil otherwise (including error conditions)

• error: non-nil if an error occurs, nil otherwise
*/
func getDigestFromOCILayout(ctx context.Context, layoutPath layout.Path, platformIn OperatorCatalogPlatform) (*v1.Hash, error) {
	var hashResult *v1.Hash

	processImage := func(img v1.Image, flagAsManifestList bool) error {
		config, err := img.ConfigFile()
		if err != nil {
			return err
		}
		platformKey, err := getPlatformKey(config)
		if err != nil {
			return err
		}
		// override the isIndex field if flag is set
		if flagAsManifestList {
			platformKey.isIndex = true
		}

		if *platformKey == platformIn {
			hash, err := img.Digest()
			if err != nil {
				return err
			}
			hashResult = &hash
		}
		return nil
	}

	// processImageIndex is recursive, so needs to be defined as a var here
	var processImageIndex func(imageIndex v1.ImageIndex) error
	processImageIndex = func(imageIndex v1.ImageIndex) error {
		// get the manifest for the index
		indexManifest, err := imageIndex.IndexManifest()
		if err != nil {
			return err
		}

		// media type in this scenario is not guaranteed to be present, but if it is, use it
		flagAsManifestList := false
		if indexManifest.MediaType != "" && indexManifest.MediaType.IsIndex() {
			flagAsManifestList = true
		}

		for _, manifest := range indexManifest.Manifests {
			// if this is an image, and we don't have platform information,
			// we'll have to delegate to processImage
			if manifest.MediaType.IsImage() && manifest.Platform == nil {
				// delegate to processImage
				img, err := imageIndex.Image(manifest.Digest)
				if err != nil {
					return err
				}
				err = processImage(img, flagAsManifestList)
				if err != nil {
					return err
				}
				// if result was found in processImage, bail out
				if hashResult != nil {
					return nil
				}
				// continue with next manifest
				continue
			}
			// if we can get the platform from here, do so
			if manifest.Platform != nil {
				platformKey, err := getPlatformKey(manifest.Platform)
				if err != nil {
					return err
				}
				// override the isIndex field if flag is set
				if flagAsManifestList {
					platformKey.isIndex = true
				}

				if *platformKey == platformIn {
					hashResult = manifest.Digest.DeepCopy()
					return nil
				}
			} else if manifest.MediaType.IsIndex() {
				// get the inner image index and recursively process it
				innerImageIndex, err := imageIndex.ImageIndex(manifest.Digest)
				if err != nil {
					return err
				}
				err = processImageIndex(innerImageIndex)
				if err != nil {
					return err
				}
				// if result was found in processImageIndex, bail out
				if hashResult != nil {
					return nil
				}
			}
		}
		return nil
	}

	imageIndex, err := layoutPath.ImageIndex()
	if err != nil {
		return hashResult, err
	}
	return hashResult, processImageIndex(imageIndex)
}
