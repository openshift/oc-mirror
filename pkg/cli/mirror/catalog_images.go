package mirror

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/image/builder"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

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
imageInfo contains file path and other metadata about an image
*/
type imageInfo struct {
	imageDetails      []imageDetails // one or more details about an image (one per platform)
	fullPathToRepoDir string         // path to the repo directory <some path>/src/catalogs/<repoPath>, where <repoPath> is a docker ref in filepath form
}

/*
imageDetails captures catalog information for a given platform
*/
type imageDetails struct {
	/*
		a path to where an index.json file resides
			single arch: <some path>/src/catalogs/<repoPath>/index/index.json
			multi arch image: <some path>/src/catalogs/<repoPath>/multi/<platform>/index/index.json
	*/
	indexJsonPath string
	platform      OperatorCatalogPlatform // platform associated with this image
	hash          v1.Hash                 // the hash of this image pulled from the OCI layout directory
}

/*
catalogsByImage keeps track of the file system paths for a given catalog.

• key is catalog destination reference (with a tag and no digest),

• value contains file system paths associated with this catalog as well as other metadata
*/
type catalogsByImage map[image.TypedImage]imageInfo

/*
rebuildCatalogs will modify an OCI catalog in <some path>/src/catalogs/<repoPath>/layout with
the index.json files found in one or more of these locations:

	single architecture image:    <some path>/src/catalogs/<repoPath>/index/index.json
	multi architecture image(s):  <some path>/src/catalogs/<repoPath>/multi/<platform>/index/index.json

# Arguments

• ctx: cancellation context

• dstDir: the path to where the config.SourceDir resides

# Returns

• image.TypedImageMapping: the source/destination mapping for the catalog (this contains all images for single or multiple architectures)

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

	// pathHasMultiRecursive walks "upward" through the path looking for the "multi" dir (if present)
	var pathHasMultiRecursive func(pathIn string, isMulti bool) bool
	pathHasMultiRecursive = func(pathIn string, isMulti bool) bool {
		// paranoid check to bail out if we've already determined this is multi
		if isMulti {
			return isMulti
		}

		base := filepath.Base(pathIn)
		dir := filepath.Dir(pathIn)

		// terminal state: found match
		if base == config.MultiDir {
			return true
		}

		// terminal state: at root of path
		if dir == "" || dir == "." {
			return false
		}

		return pathHasMultiRecursive(dir, isMulti)
	}

	dstDir = filepath.Clean(dstDir)
	// catalogsByImage is used to capture information about an image
	catalogsByImage := catalogsByImage{}
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
			// the index.json file lives in different locations depending on what kind of image it is:
			//   single architecture image:    <some path>/src/catalogs/<repoPath>/index/index.json
			//   multi architecture image(s):  <some path>/src/catalogs/<repoPath>/multi/<platform>/index/index.json

			// remove the index.json from the path
			// results in one of these:
			//   <some path>/src/catalogs/<repoPath>/index
			//   <some path>/src/catalogs/<repoPath>/multi/<platform>/index
			slashPath = path.Dir(slashPath)

			// paranoid check to make sure we're in the right spot
			if strings.HasSuffix(slashPath, config.IndexDir) {
				// remove the index folder from the path
				// results in one of these:
				//   <some path>/src/catalogs/<repoPath>
				//   <some path>/src/catalogs/<repoPath>/multi/<platform>
				slashPath = strings.TrimSuffix(slashPath, config.IndexDir)
			} else {
				// if the index.json is not within a index folder, we've discovered the wrong file... move on to next item
				return nil
			}

			// detect if this is a multi architecture path
			isMulti := pathHasMultiRecursive(slashPath, false)
			// depending on if its single or multi architecture process the path so we end up with
			// <some path>/src/catalogs/<repoPath>
			var fullPathToRepoDir string
			// platformString is the portion of the path that represents the platform associated with this index.json file
			// If this is a single architecture image then this will be empty string.
			// If this is part of a multi architecture image, then this will be non empty value
			var platformString string
			if isMulti { // multi arch branch
				// capture the platform
				platformString = path.Base(slashPath)
				// capture the parent directory of the platform
				possibleMultiDir := path.Base(path.Dir(slashPath))
				if possibleMultiDir != config.MultiDir {
					// we identified that the path had a multi dir, but its not in the expected spot... move on to next item
					return nil
				}
				// now that we know this path contains a multi directory, strip off the last two directories
				fullPathToRepoDir = path.Clean(filepath.Join(slashPath, "..", ".."))
			} else {
				// single arch branch
				fullPathToRepoDir = path.Clean(slashPath)
			}

			// remove the <some path>/src/catalogs from the path to arrive at <repoPath>
			repoPath := strings.TrimPrefix(fullPathToRepoDir, fmt.Sprintf("%s/%s/", dstDir, config.CatalogsDir))

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
			sourceRef, err := image.ParseReference(img)
			if err != nil {
				return fmt.Errorf("error parsing index dir path %q as image %q: %v", fpath, img, err)
			}

			destRef := image.TypedImage{}
			destRef.Type = imagesource.DestinationRegistry
			destRef.Ref = sourceRef.Ref
			// Update registry so the existing catalog image can be pulled.
			destRef.Ref.Registry = mirrorRef.Ref.Registry
			destRef.Ref.Namespace = path.Join(o.UserNamespace, destRef.Ref.Namespace)
			destRef = destRef.SetDefaults()
			// Unset the ID when passing to the image builder.
			// Tags are needed here since the digest will be recalculated.
			destRef.Ref.ID = ""

			// associate an imageInfo struct with the destination reference
			var info imageInfo
			if existingImageInfo, ok := catalogsByImage[destRef]; ok {
				info = existingImageInfo
			} else {
				info = imageInfo{}
			}
			// reconstruct the platform from string and use that platform to lookup the matching platform image and get its hash
			platform := *NewOperatorCatalogPlatform(platformString)
			hash, err := getDigestFromOCILayout(ctx, layout.Path(filepath.Join(fullPathToRepoDir, config.LayoutsDir)), platform)
			if err != nil {
				return err
			}
			info.fullPathToRepoDir = fullPathToRepoDir
			info.imageDetails = append(info.imageDetails, imageDetails{
				indexJsonPath: fpath,
				platform:      platform,
				hash:          *hash,
			})

			// update the map with the updated info
			catalogsByImage[destRef] = info

			// Add to mapping for ICSP generation
			refs.Add(sourceRef, destRef.TypedImageReference, v1alpha2.TypeOperatorCatalog)
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

• catalogRefs: key is catalog destination reference, value is <some path>/src/catalogs/<repoPath>

# Returns

• error: non-nil if error occurs, nil otherwise
*/
func (o *MirrorOptions) processCatalogRefs(ctx context.Context, catalogRefs catalogsByImage) error {
	for destCtlgRef, catalogInfo := range catalogRefs {
		// each platform needs to be processed separately
		for _, imageDetail := range catalogInfo.imageDetails {

			// Always build the catalog image with the new declarative config catalog
			// using the original catalog as the base image
			refExact := destCtlgRef.Ref.Exact()

			var destInsecure bool
			if o.DestPlainHTTP || o.DestSkipTLS {
				destInsecure = true
			}

			// Check push permissions before trying to resolve for Quay compatibility
			nameOpts := getNameOpts(destInsecure)
			remoteOpts := getRemoteOpts(ctx, destInsecure)
			imgBuilder := builder.NewImageBuilder(nameOpts, remoteOpts)

			klog.Infof("Rendering catalog image %q with file-based catalog ", refExact)

			add, err := builder.LayerFromPath("/configs", imageDetail.indexJsonPath)
			if err != nil {
				return fmt.Errorf("error creating add layer: %v", err)
			}

			// Since we are defining the FBC as index.json,
			// remove anything that may currently exist
			deleted, err := deleteLayer("/.wh.configs")
			if err != nil {
				return fmt.Errorf("error creating deleted layer: %v", err)
			}

			// Delete must be first in the slice
			// so that the /configs directory is deleted
			// and then add back with the new FBC.
			layers := []v1.Layer{deleted, add}

			layoutDir := filepath.Join(catalogInfo.fullPathToRepoDir, config.LayoutsDir)
			layoutPath, err := imgBuilder.CreateLayout("", layoutDir)
			if err != nil {
				return fmt.Errorf("error creating OCI layout: %v", err)
			}

			update := func(cfg *v1.ConfigFile) {
				labels := map[string]string{
					containertools.ConfigsLocationLabel: "/configs",
				}
				cfg.Config.Labels = labels
				cfg.Config.Cmd = []string{"serve", "/configs"}
				cfg.Config.Entrypoint = []string{"/bin/opm"}
			}
			// Use the hash here so that the image builder only updates that image
			if err := imgBuilder.Run(ctx, refExact, layoutPath, match.Digests(imageDetail.hash), update, layers...); err != nil {
				return fmt.Errorf("error building catalog layers: %v", err)
			}
		}
	}
	return nil
}

func deleteLayer(old string) (v1.Layer, error) {
	deleteMap := map[string][]byte{}
	deleteMap[old] = []byte{}
	return crane.Layer(deleteMap)
}
