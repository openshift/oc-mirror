package mirror

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/image/builder"
)

const (
	opmCachePrefix           = "/tmp/cache"
	opmBinarySuffix          = "opm"
	opmBinaryDir             = "usr/bin/registry"
	cacheLocationPlaceholder = "cacheLocation.txt"
	cacheFolderUID           = 1001
	cacheFolderGID           = 0
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
rebuildCatalogs will modify an OCI catalog in <some path>/src/catalogs/<repoPath>/layout with
the index.json files found in <some path>/src/catalogs/<repoPath>/index/index.json

# Arguments

• ctx: cancellation context

• dstDir: the path to where the config.SourceDir resides

# Returns

• image.TypedImageMapping: the source/destination mapping for the catalog

• error: non-nil if error occurs, nil otherwise
*/
func (o *MirrorOptions) rebuildCatalogs(ctx context.Context, dstDir string) (image.TypedImageMapping, error) {
	refs := image.TypedImageMapping{}
	var err error

	mirrorRef := imagesource.TypedImageReference{Type: imagesource.DestinationRegistry}
	mirrorRef.Ref, err = reference.Parse(o.ToMirror)
	if err != nil {
		return nil, err
	}

	dstDir = filepath.Clean(dstDir)
	catalogsByImage := map[image.TypedImage]string{}
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
		_, err := os.Stat(filepath.Join(artifactDir, cacheLocationPlaceholder))
		if errors.Is(err, os.ErrNotExist) {
			withCacheRegeneration = false
		} else if err != nil {
			return fmt.Errorf("unable to determine location of cache for image %s. Cache generation failed: %v", ctlgRef, err)
		}

		configLayerToAdd, err := builder.LayerFromPath("/configs", filepath.Join(artifactDir, config.IndexDir, "index.json"))
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
			// read the location of the cache
			dat, err := os.ReadFile(filepath.Join(artifactDir, cacheLocationPlaceholder))
			if err != nil {
				return fmt.Errorf("unable to determine location of cache for image %s. Cache generation failed: %v", ctlgRef, err)
			}
			cacheLocation := string(dat)
			cacheLocationElmts := strings.Split(strings.TrimPrefix(cacheLocation, string(os.PathSeparator)), string(os.PathSeparator))
			// white out layer /tmp
			deletedCacheLayer, err := deleteLayer("/.wh." + cacheLocationElmts[0])
			if err != nil {
				return fmt.Errorf("error creating deleted cache layer: %v", err)
			}
			layersToDelete = append(layersToDelete, deletedCacheLayer)

			opmCmdPath, err := findOpmCmd(artifactDir)
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
			cacheLayerToAdd, err := builder.LayerFromPathWithUidGid("/tmp/cache", filepath.Join(artifactDir, config.TmpDir), cacheFolderUID, cacheFolderGID)
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
			// we couldnt guarantie that the cache-dir was /tmp/cache, and therefore
			// we had to specify the command for the newly built catalog
			cfg.Config.Cmd = []string{"serve", "/configs", "--cache-dir=/tmp/cache"}
		}
		if err := imgBuilder.Run(ctx, refExact, layoutPath, update, layers...); err != nil {
			return fmt.Errorf("error building catalog layers: %v", err)
		}
	}
	return nil
}

// findOpmCmd attempts to find the opm binary within the extracted contents of the catalog container image.
// The exact location of the opm binary within the image is not certain: It depends among other things on
// the version of the catalog, on the decisions by maintainers of the catalog, on the platform...
func findOpmCmd(artifactDir string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("error finding current working directory while preparing to run opm to regenerate cache: %v", err)
	}
	runningOS := runtime.GOOS
	runningArch := runtime.GOARCH
	opmBin := "opm"

	if runningOS != "linux" {
		opmBin = strings.Join([]string{runningOS, runningArch, opmBin}, "-")
	}
	opmCmdPath := ""
	binaryDir := filepath.Join(wd, artifactDir, config.OpmBinDir)
	err = filepath.Walk(binaryDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// files that match the opmBin name with size 0 are the result of extracted symbolic links where the link has broken during extraction
		if info.Name() == opmBin && info.Size() > 0 {
			opmCmdPath = path
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error finding the extracted opm binary %s while preparing to run opm to regenerate cache: %v", opmCmdPath, err)
	}
	_, err = os.Stat(opmCmdPath)
	if err != nil {
		return "", fmt.Errorf("error finding the extracted opm binary %s while preparing to run opm to regenerate cache: %v", opmCmdPath, err)
	}
	err = os.Chmod(opmCmdPath, 0744)
	if err != nil {
		return "", fmt.Errorf("error changing permissions to the extracted opm binary while preparing to run opm to regenerate cache: %v", err)
	}
	return opmCmdPath, nil
}

// extractOPMAndCache is usually called after rendering catalog's declarative config.
// it uses crane modules to pull the catalog image, select the manifest that corresponds to the
// current platform. It then extracts from that image any files that are suffixed `*opm` for later
// use upon rebuilding the catalog: This is because the opm binary can be called `opm` but also
// `darwin-amd64-opm` etc.
func extractOPMAndCache(ctx context.Context, srcRef image.TypedImageReference, outDir string, insecure bool) error {
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
		img, err = getPtfImageFromOCIIndex(v1alpha2.TrimProtocol(srcRef.OCIFBCPath), runtime.GOARCH, runtime.GOOS)
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

	cacheLocationFileName := filepath.Join(outDir, cacheLocationPlaceholder)

	baseDir := filepath.Dir(cacheLocationFileName)
	err = os.MkdirAll(baseDir, 0755)
	if err != nil {
		return err
	}

	cfl, err := os.Create(cacheLocationFileName)
	if err == nil {
		defer cfl.Close()
	} else {
		return err
	}

	_, err = cfl.Write([]byte(cachePath))
	if err != nil {
		return err
	}
	cfl.Close()
	// cachePath exists, opm binary will be needed to regenerate it
	tr := tar.NewReader(mutate.Extract(img))
	opmBinaryExtracted := ""
	for {
		header, err := tr.Next()

		// break the infinite loop when EOF
		if errors.Is(err, io.EOF) {
			break
		}

		// skip the file if it is a directory or if file name does not end with `opm`
		if !strings.HasSuffix(header.Name, opmBinarySuffix) || header.FileInfo().IsDir() {
			continue
		}

		var buf bytes.Buffer
		_, err = buf.ReadFrom(tr)
		if err != nil {
			return err
		}

		targetFileName := filepath.Join(outDir, config.OpmBinDir, header.Name)
		bytes := buf.Bytes()

		baseDir := filepath.Dir(targetFileName)
		err = os.MkdirAll(baseDir, 0755)
		if err != nil {
			return err
		}

		f, err := os.Create(targetFileName)
		if err == nil {
			defer f.Close()
		} else {
			return err
		}

		_, err = f.Write(bytes)
		if err != nil {
			return err
		}
		opmBinaryExtracted = targetFileName

		// check for the extracted opm file (it should exist if we found something)
		_, err = os.Stat(opmBinaryExtracted)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("opm binary not found after extracting %q from catalog image %v", opmBinaryExtracted, srcRef)
		}
	}

	return nil
}

// getPtfImageFromOCIIndex takes an oci local image located in `fbcPath` and finds the image that
// corresponds to the current platform and OS within the manifestList or imageIndex
func getPtfImageFromOCIIndex(fbcPath string, architecture string, os string) (v1.Image, error) {
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
