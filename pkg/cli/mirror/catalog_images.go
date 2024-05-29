package mirror

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/otiai10/copy"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/image/builder"
)

const (
	opmCachePrefix  = "/tmp/cache"
	opmBinarySuffix = "opm"
	cacheFolderUID  = 1001
	cacheFolderGID  = 0
	tempFolder      = "/tmp/temp_folder"
)

type NoCacheArgsErrorType struct{}

var NoCacheArgsError = NoCacheArgsErrorType{}

func (m NoCacheArgsErrorType) Error() string {
	return "catalog container image command does not specify cache arguments - no cache generation will be attempted"
}

// unpackCatalog will unpack file-based catalogs if they exists
func (o *MirrorOptions) unpackCatalog(dstDir string, filesInArchive map[string]string) (bool, error) {
	var found bool
	if err := unpack(config.CatalogsDir, dstDir, filesInArchive); err != nil {
		nferr := &ErrArchiveFileNotFound{}
		if errors.As(err, &nferr) || errors.Is(err, os.ErrNotExist) {
			klog.V(2).Info("No catalogs found in archive, skipping catalog rebuild")
			return found, nil
		}
		return found, err
	}
	found = true
	return found, nil
}

/*
rebuildOrCopyCatalogs will modify an OCI catalog in <some path>/src/catalogs/<repoPath>/layout with
the index.json files found in <some path>/src/catalogs/<repoPath>/index/index.json

# Arguments

• ctx: cancellation context

• dstDir: the path to where the config.SourceDir resides

# Returns

• image.TypedImageMapping: the source/destination mapping for the catalog

• error: non-nil if error occurs, nil otherwise
*/
func (o *MirrorOptions) rebuildOrCopyCatalogs(ctx context.Context, dstDir string) (image.TypedImageMapping, error) {
	refs := image.TypedImageMapping{}
	var err error

	mirrorRef := imagesource.TypedImageReference{Type: imagesource.DestinationRegistry}
	mirrorRef.Ref, err = reference.Parse(o.ToMirror)
	if err != nil {
		return nil, err
	}

	dstDir = filepath.Clean(dstDir)
	catalogsByImage := map[image.TypedImage]string{}
	if o.RebuildCatalogs {
		if err := filepath.Walk(dstDir, func(fpath string, info fs.FileInfo, err error) error {

			// Skip the layouts dir because we only need
			// to process the parent directory one time
			if filepath.Base(fpath) == config.LayoutsDir {
				return filepath.SkipDir
			}

			if err != nil || info == nil {
				return err
			}

			// From the index path determine the artifacts (index and layout) directory.
			// Using that path to determine the corresponding catalog image for processing.
			slashPath := filepath.ToSlash(fpath)
			if base := path.Base(slashPath); base == "index.json" {
				// remove the index.json from the path
				// results in <some path>/src/catalogs/<repoPath>/index
				slashPath = path.Dir(slashPath)
				// remove the index folder from the path
				// results in <some path>/src/catalogs/<repoPath>
				slashPath = strings.TrimSuffix(slashPath, config.IndexDir)

				// remove the <some path>/src/catalogs from the path to arrive at <repoPath>
				repoPath := strings.TrimPrefix(slashPath, fmt.Sprintf("%s/%s/", dstDir, config.CatalogsDir))
				// get the repo namespace and id (where ID is a SHA or tag)
				// example: foo.com/foo/bar/<id>
				regRepoNs, id := path.Split(path.Dir(repoPath))
				regRepoNs = path.Clean(regRepoNs)
				// reconstitute the path into a valid docker ref
				var img string
				if strings.Contains(id, ":") {
					// Digest.
					img = fmt.Sprintf("%s@%s", regRepoNs, id)
				} else {
					// Tag.
					img = fmt.Sprintf("%s:%s", regRepoNs, id)
				}
				ctlgRef := image.TypedImage{}
				ctlgRef.Type = imagesource.DestinationRegistry
				sourceRef, err := image.ParseReference(img)
				// since we can't really tell if the "img" reference originated from an actual docker
				// reference or from an OCI file path that approximates a docker reference, ParseReference
				// might not lowercase the name and namespace values which is required by the
				// docker reference spec (see https://github.com/distribution/distribution/blob/main/reference/reference.go).
				// Therefore we lower case name and namespace here to make sure it's done.
				sourceRef.Ref.Name = strings.ToLower(sourceRef.Ref.Name)
				sourceRef.Ref.Namespace = strings.ToLower(sourceRef.Ref.Namespace)

				if err != nil {
					return fmt.Errorf("error parsing index dir path %q as image %q: %v", fpath, img, err)
				}
				ctlgRef.Ref = sourceRef.Ref
				// Update registry so the existing catalog image can be pulled.
				ctlgRef.Ref.Registry = mirrorRef.Ref.Registry
				ctlgRef.Ref.Namespace = path.Join(o.UserNamespace, ctlgRef.Ref.Namespace)
				ctlgRef = ctlgRef.SetDefaults()
				// Unset the ID when passing to the image builder.
				// Tags are needed here since the digest will be recalculated.
				ctlgRef.Ref.ID = ""

				catalogsByImage[ctlgRef] = slashPath

				// Add to mapping for ICSP generation
				refs.Add(sourceRef, ctlgRef.TypedImageReference, v1alpha2.TypeOperatorCatalog)
			}
			return nil
		}); err != nil {
			return nil, err
		}

		// update the catalogs in the OCI layout directory and push them to their destination
		if err := o.processCatalogRefs(ctx, catalogsByImage); err != nil {
			return nil, err
		}
	} else {
		if err := filepath.Walk(dstDir, func(fpath string, info fs.FileInfo, err error) error {
			if err != nil || info == nil {
				return err
			}
			slashPath := filepath.ToSlash(fpath)

			if filepath.Base(fpath) == config.LayoutsDir {

				// results in <some path>/src/catalogs/<repoPath>/layout
				slashPath = path.Dir(slashPath)

				// remove the <some path>/src/catalogs from the path to arrive at <repoPath>
				repoPath := strings.TrimPrefix(slashPath, fmt.Sprintf("%s/%s/", dstDir, config.CatalogsDir))
				// get the repo namespace and id (where ID is a SHA or tag)
				// example: foo.com/foo/bar/<id>
				regRepoNs, id := path.Split(repoPath)
				regRepoNs = path.Clean(regRepoNs)
				// reconstitute the path into a valid docker ref
				var img string
				if strings.Contains(id, ":") {
					// Digest.
					img = fmt.Sprintf("%s@%s", regRepoNs, id)
				} else {
					// Tag.
					img = fmt.Sprintf("%s:%s", regRepoNs, id)
				}

				ctlgRef := image.TypedImage{}
				ctlgRef.Type = imagesource.DestinationRegistry
				originRef, err := image.ParseReference(img)
				// since we can't really tell if the "img" reference originated from an actual docker
				// reference or from an OCI file path that approximates a docker reference, ParseReference
				// might not lowercase the name and namespace values which is required by the
				// docker reference spec (see https://github.com/distribution/distribution/blob/main/reference/reference.go).
				// Therefore we lower case name and namespace here to make sure it's done.
				originRef.Ref.Name = strings.ToLower(originRef.Ref.Name)
				originRef.Ref.Namespace = strings.ToLower(originRef.Ref.Namespace)

				if err != nil {
					return fmt.Errorf("error parsing index dir path %q as image %q: %v", fpath, img, err)
				}
				ctlgRef.Ref = originRef.Ref
				// Update registry so the existing catalog image can be pulled.
				ctlgRef.Ref.Registry = mirrorRef.Ref.Registry
				ctlgRef.Ref.Namespace = path.Join(o.UserNamespace, ctlgRef.Ref.Namespace)
				ctlgRef = ctlgRef.SetDefaults()
				// Unset the ID when passing to the image builder.
				// Tags are needed here since the digest will be recalculated.
				ctlgRef.Ref.ID = ""

				catalogOCIDir := fpath
				isValid, err := isValidOCILayout(catalogOCIDir)
				if err != nil {
					klog.Warningf("unable to verify that the catalog layout is valid for image %s: %v", ctlgRef.Ref.String(), err)
				}
				if isValid && err == nil {
					// Add to mapping for ICSP generation
					if strings.Contains(fpath, "sha256:") {
						randomNumber := strconv.Itoa(rand.Int())
						catalogOCIDir = tempFolder + randomNumber
						err := os.Mkdir(catalogOCIDir, 0755)
						if err != nil {
							klog.Warningf("unable to create temp folder facilitating mirroring of catalog image %s: %v", fpath, err)
						}
						defer os.RemoveAll(catalogOCIDir)
						err = copy.Copy(fpath, catalogOCIDir)
						if err != nil {
							klog.Warningf("unable to copy catalog to temp folder while mirroring of catalog image %s: %v", fpath, err)
						}
					}
					_, err = o.copyImage(ctx, "oci://"+catalogOCIDir, "docker://"+ctlgRef.Ref.String(), o.remoteRegFuncs)
					if err != nil {
						return fmt.Errorf("error copying image %s to %s: %v", fpath, "docker://"+ctlgRef.Ref.String(), err)
					}

				} else {
					layoutPath, err := layout.FromPath(fpath)
					if err != nil {
						return fmt.Errorf("error loading local image %s from %s: %v", "docker://"+ctlgRef.Ref.String(), fpath, err)
					}

					var destInsecure bool
					if o.DestPlainHTTP || o.DestSkipTLS {
						destInsecure = true
					}

					// Check push permissions before trying to resolve for Quay compatibility
					nameOpts := getNameOpts(destInsecure)
					remoteOpts := getRemoteOpts(ctx, destInsecure)

					imgBuilder := builder.NewImageBuilder(nameOpts, remoteOpts)
					update := func(cfg *v1.ConfigFile) {}
					err = imgBuilder.Run(ctx, ctlgRef.Ref.String(), layoutPath, update, []v1.Layer{}...)
					if err != nil {
						return fmt.Errorf("error copying image %s from %s: %v", "docker://"+ctlgRef.Ref.String(), fpath, err)
					}
				}
				// Add to mapping for ICSP generation
				refs.Add(originRef, ctlgRef.TypedImageReference, v1alpha2.TypeOperatorCatalog)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	// use the resolver to obtain the digests of the newly pushed images
	resolver, err := containerdregistry.NewResolver("", o.DestSkipTLS, o.DestPlainHTTP, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating image resolver: %v", err)
	}

	// Resolve the image's digest for ICSP creation.
	for source, dest := range refs {
		_, desc, err := resolver.Resolve(ctx, dest.Ref.Exact())
		if err != nil {
			return nil, fmt.Errorf("error retrieving digest for catalog image %q: %v", dest.Ref.Exact(), err)
		}
		dest.Ref.ID = desc.Digest.String()
		refs[source] = dest
	}

	return refs, nil
}

/*
processCatalogRefs uses the image builder to update a given image using the data provided in catalogRefs.

# Arguments

• ctx: cancellation context

• catalogsByImage: key is catalog destination reference, value is <some path>/src/catalogs/<repoPath>

# Returns

• error: non-nil if error occurs, nil otherwise
*/
func (o *MirrorOptions) processCatalogRefs(ctx context.Context, catalogsByImage map[image.TypedImage]string) error {
	for ctlgRef, artifactDir := range catalogsByImage {
		// Always build the catalog image with the new declarative config catalog
		// using the original catalog as the base image
		var layoutPath layout.Path
		refExact := ctlgRef.Ref.Exact()

		var destInsecure bool
		if o.DestPlainHTTP || o.DestSkipTLS {
			destInsecure = true
		}

		// Check push permissions before trying to resolve for Quay compatibility
		nameOpts := getNameOpts(destInsecure)
		remoteOpts := getRemoteOpts(ctx, destInsecure)
		imgBuilder := builder.NewImageBuilder(nameOpts, remoteOpts)

		klog.Infof("Rendering catalog image %q with file-based catalog ", refExact)

		layersToAdd := []v1.Layer{}
		layersToDelete := []v1.Layer{}
		withCacheRegeneration := true
		_, err := os.Stat(filepath.Join(artifactDir, config.OPMCacheLocationPlaceholder))
		if errors.Is(err, os.ErrNotExist) {
			withCacheRegeneration = false
		} else if err != nil {
			return fmt.Errorf("unable to determine location of cache for image %s. Cache generation failed: %v", ctlgRef, err)
		}

		configLayerToAdd, err := builder.LayerFromPathWithUidGid("/configs", filepath.Join(artifactDir, config.IndexDir), 0, 0)
		if err != nil {
			return fmt.Errorf("error creating add layer: %v", err)
		}
		layersToAdd = append(layersToAdd, configLayerToAdd)

		// Since we are defining the FBC as index.json,
		// remove anything that may currently exist
		deletedConfigLayer, err := deleteLayer("/.wh.configs")
		if err != nil {
			return fmt.Errorf("error creating deleted layer: %v", err)
		}
		layersToDelete = append(layersToDelete, deletedConfigLayer)

		if withCacheRegeneration {

			opmCmdPath := ""
			if opmBinary := os.Getenv("OPM_BINARY"); opmBinary != "" {
				opmCmdPath = opmBinary
			} else {
				opmCmdPath = filepath.Join(artifactDir, config.OpmBinDir, "opm")
			}
			_, err = os.Stat(opmCmdPath)
			if err != nil {
				return fmt.Errorf("cannot find opm in the extracted catalog %v for %s on %s: %v", ctlgRef, runtime.GOOS, runtime.GOARCH, err)
			}

			absConfigPath, err := filepath.Abs(filepath.Join(artifactDir, config.IndexDir))
			if err != nil {
				return fmt.Errorf("error getting absolute path for catalog's index %v: %v", filepath.Join(artifactDir, config.IndexDir), err)
			}
			absCachePath, err := filepath.Abs(filepath.Join(artifactDir, config.TmpDir))
			if err != nil {
				return fmt.Errorf("error getting absolute path for catalog's cache %v: %v", filepath.Join(artifactDir, config.TmpDir), err)
			}
			cmd := exec.Command(opmCmdPath, "serve", absConfigPath, "--cache-dir", absCachePath, "--cache-only")
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("error regenerating the cache for %v: %v", ctlgRef, err)
			}
			// Fix OCPBUGS-17546:
			// Add the cache under /cache in a new layer (instead of white-out /tmp/cache, which resulted in crashLoopBackoff only on some clusters)
			cacheLayerToAdd, err := builder.LayerFromPathWithUidGid("/cache", filepath.Join(artifactDir, config.TmpDir), cacheFolderUID, cacheFolderGID)
			if err != nil {
				return fmt.Errorf("error creating add layer: %v", err)
			}
			layersToAdd = append(layersToAdd, cacheLayerToAdd)
		}

		// Deleted layers must be added first in the slice
		// so that the /configs and /tmp directories are deleted
		// and then added back from the layers rebuilt from the new FBC.
		layers := []v1.Layer{}
		layers = append(layers, layersToDelete...)
		layers = append(layers, layersToAdd...)

		layoutDir := filepath.Join(artifactDir, config.LayoutsDir)
		layoutPath, err = imgBuilder.CreateLayout("", layoutDir)
		if err != nil {
			return fmt.Errorf("error creating OCI layout: %v", err)
		}

		update := func(cfg *v1.ConfigFile) {
			labels := map[string]string{
				containertools.ConfigsLocationLabel: "/configs",
			}
			cfg.Config.Labels = labels
			// Although it was prefered to keep the entrypoint and command as it was
			// we couldnt reuse /tmp/cache as the cache directory (OCPBUGS-17546)
			if withCacheRegeneration {
				cfg.Config.Cmd = []string{"serve", "/configs", "--cache-dir=/cache"}
			} else { // this means that no cache was found in the original catalog (old catalog with opm < 1.25)
				cfg.Config.Cmd = []string{"serve", "/configs"}
			}
		}
		if err := imgBuilder.Run(ctx, refExact, layoutPath, update, layers...); err != nil {
			return fmt.Errorf("error building catalog layers: %v", err)
		}
	}
	return nil
}

// extractOPMAndCache is usually called after rendering catalog's declarative config.
// it uses crane modules to pull the catalog image, select the manifest that corresponds to the
// current platform architecture. It then extracts from that image any files that are suffixed `*opm` for later
// use upon rebuilding the catalog: This is because the opm binary can be called `opm` but also
// `darwin-amd64-opm` etc.
func extractOPMAndCache(ctx context.Context, srcRef image.TypedImageReference, ctlgSrcDir string, insecure bool) error {
	var img v1.Image
	var err error
	refExact := srcRef.Ref.Exact()
	if srcRef.OCIFBCPath == "" {
		remoteOpts := getCraneOpts(ctx, insecure)
		img, err = crane.Pull(refExact, remoteOpts...)
		if err != nil {
			return fmt.Errorf("unable to pull image from %s: %v", refExact, err)
		}
	} else {
		img, err = getPlatformImageFromOCIIndex(v1alpha2.TrimProtocol(srcRef.OCIFBCPath), runtime.GOARCH, runtime.GOOS)
		if err != nil {
			return err
		}
	}
	// if we get here and no image was found bail out
	if img == nil {
		return fmt.Errorf("unable to obtain image for %v", srcRef)
	}
	cachePath, err := getCachePath(img)
	if err != nil {
		if errors.Is(err, NoCacheArgsError) {
			return nil
		} else {
			return err
		}
	}

	cacheLocationFileName := filepath.Join(ctlgSrcDir, config.OPMCacheLocationPlaceholder)

	baseDir := filepath.Dir(cacheLocationFileName)
	err = os.MkdirAll(baseDir, 0755)
	if err != nil {
		return err
	}

	cfl, err := os.Create(cacheLocationFileName)
	if err != nil {
		return err
	}
	defer cfl.Close()

	_, err = cfl.Write([]byte(cachePath))
	if err != nil {
		return err
	}
	// cachePath exists, opm binary will be needed to regenerate it
	opmBinaryFileName, err := copyOPMBinary(img, ctlgSrcDir)
	if err != nil {
		return err
	}
	// check for the extracted opm file (it should exist if we found something)
	_, err = os.Stat(opmBinaryFileName)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("opm binary not found after extracting opm from catalog image %v", srcRef)
	}
	err = os.Chmod(opmBinaryFileName, 0744)
	if err != nil {
		return fmt.Errorf("error changing permissions to the extracted opm binary while preparing to run opm to regenerate cache: %v", err)
	}

	return nil
}

// getPlatformImageFromOCIIndex takes an oci local image located in `fbcPath` and finds the image that
// corresponds to the current platform and OS within the manifestList or imageIndex
func getPlatformImageFromOCIIndex(fbcPath string, architecture string, os string) (v1.Image, error) {
	var img v1.Image

	// obtain the path to where the OCI image reference resides
	layoutPath := layout.Path(fbcPath)

	// get its index.json and obtain its manifest
	rootIndex, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, err
	}
	rootIndexManifest, err := rootIndex.IndexManifest()
	if err != nil {
		return nil, err
	}

	// attempt to find the first image reference in the layout that corresponds to the platform...
	// for a manifest list only search one level deep.

loop:
	for _, descriptor := range rootIndexManifest.Manifests {

		if descriptor.MediaType.IsIndex() {
			// follow the descriptor using its digest to get the referenced index and its manifest
			childIndex, err := rootIndex.ImageIndex(descriptor.Digest)
			if err != nil {
				return nil, err
			}
			childIndexManifest, err := childIndex.IndexManifest()
			if err != nil {
				return nil, err
			}

			// at this point, find and extract the child index that corresponds to this machine's
			// architecture
			for _, childDescriptor := range childIndexManifest.Manifests {
				if childDescriptor.MediaType.IsImage() && childDescriptor.Platform.Architecture == architecture && childDescriptor.Platform.OS == os {
					img, err = childIndex.Image(childDescriptor.Digest)
					if err != nil {
						return nil, err
					}
					// no further processing necessary
					break loop
				}
			}

		} else if descriptor.MediaType.IsImage() {
			// this is a direct reference to an image, so just store it for later
			img, err = rootIndex.Image(descriptor.Digest)
			if err != nil {
				return nil, err
			}
			// no further processing necessary
			break loop
		}
	}
	return img, nil
}

// getCachePath reads an image's config, and determines if the container command  had a --cache-dir argument
// and if so, returns the path that corresponds to that argument
func getCachePath(img v1.Image) (string, error) {
	cachePath := ""
	cfgf, err := img.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("unable to get config file for image %v: %v", img, err)
	}
	cmd := cfgf.Config.Cmd
	hasCacheArg := false
	for i, elmt := range cmd {
		if strings.Contains(elmt, "--cache-dir") {
			hasCacheArg = true
			subElmts := strings.Split(elmt, "=")
			if len(subElmts) == 1 { // the command might  be `serve --cache-dir /tmp/cache`
				cachePath = cmd[i+1]
			} else if len(subElmts) == 2 { // the command might be `serve --cache-dir=/tmp/cache`
				cachePath = subElmts[1]
			} else {
				return "", fmt.Errorf("unable to parse command line for image %v: %v", img, err)
			}
			break
		}
	}
	if !hasCacheArg {
		return "", NoCacheArgsError
	} else {
		return cachePath, nil
	}
}

func deleteLayer(old string) (v1.Layer, error) {
	deleteMap := map[string][]byte{}
	deleteMap[old] = []byte{}
	return crane.Layer(deleteMap)
}

func copyOPMBinary(img v1.Image, ctlgSrcDir string) (string, error) {
	targetOpm := filepath.Join(ctlgSrcDir, config.OpmBinDir, "opm")
	cfgf, err := img.ConfigFile()
	if err != nil {
		return "", err
	}
	opmBin := ""
	// Following https://docs.docker.com/engine/reference/builder/#understand-how-cmd-and-entrypoint-interact,
	// identifying the binary that will be run by the catalog:
	entrypoint := cfgf.Config.Entrypoint
	if len(entrypoint) > 0 {
		opmBin = strings.TrimPrefix(entrypoint[0], (string)(os.PathSeparator)) // /bin/opm becomes bin/opm in order to prepare for extraction from the image
	} else {
		cmd := cfgf.Config.Cmd
		if len(cmd) > 0 {
			opmBin = strings.TrimPrefix(cmd[0], (string)(os.PathSeparator))
		}
	}
	if opmBin == "" {
		return "", fmt.Errorf("unable to find the path to opm in the catalog image: both entrypoint and cmd were empty")
	}
	// Extract all catalog contents to a temporary folder
	err = extractCatalog(img, filepath.Join(ctlgSrcDir, config.CtlgExtractionDir), opmBin)
	if err != nil {
		return "", err
	}
	// The extractionDir now contains all the contents of the image, including
	// any symbolic links.
	// check that `opmBin` (as set in the image Entrypoint) can be found in the extractionDir
	opmBinPath := filepath.Join(ctlgSrcDir, config.CtlgExtractionDir, opmBin)
	_, err = os.Stat(opmBinPath)
	if err != nil {
		return "", err
	}

	// prepare folder for copying the binary for later use
	err = os.MkdirAll(filepath.Join(ctlgSrcDir, config.OpmBinDir), 0755)
	if err != nil {
		return "", err
	}

	// if `opmBin` has any symbolic links they can be evaluated easily
	realOpmBinPath, err := filepath.EvalSymlinks(opmBinPath)
	if err != nil {
		return "", err
	}

	// copy opm binary from the extractionDir to a separate folder (bin) under catalogSourceDir
	err = os.Rename(realOpmBinPath, targetOpm)
	if err != nil {
		return "", err
	}
	// in order to avoid using too much space (case where several operator catalogs are included in the imagesetconfig)
	// we clean up the extracted catalog right after having copied the opmBinary to its target location
	err = os.RemoveAll(filepath.Join(ctlgSrcDir, config.CtlgExtractionDir))
	if err != nil {
		return "", err
	}
	return targetOpm, nil
}

// `extractCatalog` function extracts the `img` contents into `destFolder`.
// All layers of the image are flattened.
// All contents (files, folders and symLinks are extracted)
// Errors related to creation of symbolic links are not considered as blockers
// unless they relate to the `opmBin` path.
func extractCatalog(img v1.Image, destFolder string, opmBin string) error {
	// `symLinks` is a map of symbolic link (key) to the file it points to (value)
	symLinks := make(map[string]string)
	tr := tar.NewReader(mutate.Extract(img))
	err := os.MkdirAll(destFolder, 0755)
	if err != nil {
		return err
	}
	for {
		header, err := tr.Next()

		// break the infinite loop when EOF
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		if header == nil {
			continue
		}

		// descriptor points to the file being read
		descriptor := filepath.Join(destFolder, header.Name)

		// check the file type
		// if its a dir and it doesn't exist create it
		if header.Typeflag == tar.TypeDir {
			if _, err := os.Stat(descriptor); err != nil {
				if err := os.MkdirAll(descriptor, 0755); err != nil {
					return err
				}
			}
		} else if header.Typeflag == tar.TypeReg {
			// if it's a file create it, making sure it's at least writable and executable by the user - should only be needed for debug cases
			f, err := os.OpenFile(descriptor, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode)|0755)
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		} else if header.Typeflag == tar.TypeSymlink {
			// A symLink cannot be created inside the loop:
			// the file/folder it points to (header.Linkname), or any folder on the path
			// to that file/folder might not have been untarred yet.
			// Storing the symbolic link to the map in order to create it later.
			// Examples:
			// descriptor=".../oc-mirror-workspace/src/catalogs/.../extracted/usr/bin/darwin-amd64-opm"; 		".../oc-mirror-workspace/src/catalogs/.../extracted/bin/registry/darwin-amd64-opm"
			// descriptor=".../oc-mirror-workspace/src/catalogs/.../extracted/usr/bin/opm"; 					".../oc-mirror-workspace/src/catalogs/.../extracted/bin/registry/opm"
			// descriptor=".../oc-mirror-workspace/src/catalogs/.../extracted/etc/alternatives/easy_install-3"; ".../oc-mirror-workspace/src/catalogs/.../extracted/usr/bin/easy_install-3.6"
			// descriptor=".../oc-mirror-workspace/src/catalogs/.../extracted/usr/bin/easy_install-3.6"; 		".../oc-mirror-workspace/src/catalogs/.../extracted/hostname"
			symLinks[descriptor] = filepath.Join(destFolder, header.Linkname)
		}

	}

	// symLinks map now contains all symbolic links from the image, all layers included.
	// The following loop will attempt to create each of these symbolic links.
	// Some attempts will fail.
	// Example : `hostname` is the target of several symbolic links in this map.
	// Creating symlinks pointing to `hostname` have been known to fail sometimes
	// because this file is not owned by the user and is read only.
	// Most of such problems have been solved by forcing the filemode to 0755 for all files untarred
	// (see previous loop).
	// For those attempts that failed, most will not be of any incidence to extracting the opm binary.
	// That is why we ignore these errors unless they correspond to a map entry relating to opmBin's path.
	// This technique is not flowless, but it reduces substancially breaking for symbolic links like `hostname`
	// that should not have any incidence on the extraction of the opm binary from the image.
	opmBinPathComponents := strings.Split(opmBin, string(filepath.Separator))
	for link, target := range symLinks {
		targetAbsPath, err := filepath.Abs(target)
		if err != nil { // this is an overkill, and should not happen
			for _, pathComp := range opmBinPathComponents {
				if strings.Contains(target, pathComp) {
					return err
				}
			}
		}
		linkAbsPath, err := filepath.Abs(link)
		if err != nil { // this is an overkill, and should not happen
			for _, pathComp := range opmBinPathComponents {
				if strings.Contains(link, pathComp) {
					return err
				}
			}
		}
		err = os.Symlink(targetAbsPath, linkAbsPath)
		if err != nil { // For `opmBin` = `/bin/opm`,
			// only return the error if the symlink contains `/bin` or `/opm`
			for _, pathComp := range opmBinPathComponents {
				if strings.Contains(link, pathComp) {
					return err
				}
			}
		}
	}

	return nil
}

func isValidOCILayout(folder string) (bool, error) {
	_, err := os.Stat(folder)
	if err != nil {
		return false, err
	}
	indexFile := filepath.Join(folder, "index.json")
	_, err = os.Stat(indexFile)
	if err != nil {
		return false, err
	}
	indexData, err := os.ReadFile(indexFile)
	if err != nil {
		return false, err
	}
	if strings.Contains(string(indexData), "application/vnd.oci") {
		return true, nil
	} else {
		return false, nil
	}
}
