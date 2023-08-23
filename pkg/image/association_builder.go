package image

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ctrsimgmanifest "github.com/containers/image/v5/manifest"
	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
)

type ErrInvalidImage struct {
	image string
	cause error
}

func (e *ErrInvalidImage) Error() string {
	message := fmt.Sprintf("image %q is invalid or does not exist", e.image)
	if e.cause != nil {
		message = fmt.Sprintf("%s: %v", message, e.cause)
	}
	return message
}

type ErrInvalidComponent struct {
	image string
	tag   string
}

func (e *ErrInvalidComponent) Error() string {
	return fmt.Sprintf("image %q has invalid component %q", e.image, e.tag)
}

// AssociateLocalImageLayers traverses a V2 directory and gathers all child manifests and layer digest information
// for mirrored images
func AssociateLocalImageLayers(rootDir string, imgMappings TypedImageMapping) (AssociationSet, utilerrors.Aggregate) {
	errs := []error{}
	bundleAssociations := AssociationSet{}

	skipParse := func(ref string) bool {
		seen := bundleAssociations.SetContainsKey(ref)
		return seen
	}

	localRoot := filepath.Join(rootDir, config.V2Dir)
	for image, diskLoc := range imgMappings {
		if diskLoc.Type != imagesource.DestinationFile {
			errs = append(errs, fmt.Errorf("image destination for %q is not type file", image.Ref.Exact()))
			continue
		}
		dirRef := diskLoc.Ref.AsRepository().String()
		imagePath := filepath.Join(localRoot, dirRef)

		// Verify that the dirRef exists before proceeding
		if _, err := os.Stat(imagePath); err != nil {
			errs = append(errs, &ErrInvalidImage{image.String(), nil})
			continue
		}

		var tagOrID string
		if diskLoc.Ref.Tag != "" {
			tagOrID = diskLoc.Ref.Tag
		} else {
			tagOrID = diskLoc.Ref.ID
		}

		if tagOrID == "" {
			errs = append(errs, &ErrInvalidComponent{image.String(), tagOrID})
			continue
		}

		// TODO(estroz): parallelize
		associations, err := associateLocalImageLayers(image.Ref.String(), localRoot, dirRef, tagOrID, "oc-mirror", image.Category, skipParse)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, association := range associations {
			bundleAssociations.Add(image.Ref.String(), association)
		}
	}

	return bundleAssociations, utilerrors.NewAggregate(errs)
}

func associateLocalImageLayers(image, localRoot, dirRef, tagOrID, defaultTag string, typ v1alpha2.ImageType, skipParse func(string) bool) (associations []v1alpha2.Association, err error) {
	if skipParse(image) {
		return nil, nil
	}

	manifestPath := filepath.Join(localRoot, filepath.FromSlash(dirRef), "manifests", tagOrID)
	// TODO(estroz): this file mode checking block is likely only necessary
	// for the first recursion leaf since image manifest layers always contain id's,
	// so unroll this component into AssociateImageLayers.

	info, err := os.Lstat(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &ErrInvalidImage{image, nil}
	} else if err != nil {
		return nil, err
	}
	// Tags are always symlinks due to how `oc` libraries mirror manifest files.
	id, tag := tagOrID, tagOrID
	switch m := info.Mode(); {
	case m&fs.ModeSymlink != 0:
		// Tag is the file name, so follow the symlink to the layer ID-named file.
		dst, err := os.Readlink(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("error evaluating image tag symlink: %v", err)
		}
		id = filepath.Base(dst)
	case m.IsRegular():
		// Layer ID is the file name, and no tag exists.
		tag = defaultTag
		if defaultTag != "" {
			// If set, add a subset of the digest to randomize the
			// tag in the event multiple digests are pulled for the same
			// image.
			partial, err := getPartialDigest(id)
			if err != nil {
				return nil, fmt.Errorf("error calculating partial digest for %s: %v", id, err)
			}
			tag = defaultTag + partial
			manifestDir := filepath.Dir(manifestPath)
			symlink := filepath.Join(manifestDir, tag)
			if err := os.Symlink(info.Name(), symlink); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("expected symlink or regular file mode, got: %b", m)
	}
	manifestBytes, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		return nil, fmt.Errorf("error reading image manifest file: %v", err)
	}

	association := v1alpha2.Association{
		Name:       image,
		Path:       dirRef,
		ID:         id,
		TagSymlink: tag,
		Type:       typ,
	}
	switch mt := ctrsimgmanifest.GuessMIMEType(manifestBytes); mt {
	case "":
		return nil, errors.New("unparseable manifest mediaType")
	case imgspecv1.MediaTypeImageIndex, ctrsimgmanifest.DockerV2ListMediaType:
		list, err := ctrsimgmanifest.ListFromBlob(manifestBytes, mt)
		if err != nil {
			return nil, err
		}
		for _, instance := range list.Instances() {
			digestStr := instance.String()
			// Add manifest references so publish can recursively look up image layers
			// for the manifests of this list.
			association.ManifestDigests = append(association.ManifestDigests, digestStr)
			// Recurse on child manifests, which should be in the same directory
			// with the same file name as it's digest.
			childAssocs, err := associateLocalImageLayers(digestStr, localRoot, dirRef, digestStr, "", typ, skipParse)
			if err != nil {
				return nil, err
			}
			associations = append(associations, childAssocs...)
		}
	default:
		// Treat all others as image manifests.
		manifest, err := ctrsimgmanifest.FromBlob(manifestBytes, mt)
		if err != nil {
			return nil, err
		}
		for _, layerInfo := range manifest.LayerInfos() {
			association.LayerDigests = append(association.LayerDigests, layerInfo.Digest.String())
		}
		// The config is just another blob, so associate it opaquely.
		association.LayerDigests = append(association.LayerDigests, manifest.ConfigInfo().Digest.String())
	}

	associations = append(associations, association)

	return associations, nil
}

// AssociateRemoteImageLayers queries remote manifests and gathers all child manifests and layer digest information
// for mirrored images
func AssociateRemoteImageLayers(ctx context.Context, imgMappings TypedImageMapping, skipTlS, plainHTTP, skipVerification bool) (AssociationSet, utilerrors.Aggregate) {
	var insecure bool
	if skipTlS || plainHTTP {
		insecure = true
	}
	errs := []error{}
	bundleAssociations := AssociationSet{}

	skipParse := func(ref string) bool {
		seen := bundleAssociations.SetContainsKey(ref)
		return seen
	}

	resolver, err := containerdregistry.NewResolver("", skipTlS, plainHTTP, nil)
	if err != nil {
		err = fmt.Errorf("error creating image resolver: %v", err)
		return bundleAssociations, utilerrors.NewAggregate([]error{err})
	}

	regctx, err := NewContext(skipVerification)
	if err != nil {
		err = fmt.Errorf("error creating registry context: %v", err)
		return bundleAssociations, utilerrors.NewAggregate([]error{err})
	}

	for srcImg, dstImg := range imgMappings {
		if dstImg.Type != imagesource.DestinationRegistry {
			errs = append(errs, fmt.Errorf("image destination for %q is not type registry", srcImg.Ref.Exact()))
			continue
		}

		if srcImg.Ref.ID == "" {
			if srcImg.Ref.Tag == "" {
				errs = append(errs, &ErrInvalidComponent{srcImg.String(), srcImg.Ref.Tag})
				continue
			}
			imgWithID, err := ResolveToPin(ctx, resolver, srcImg.Ref.Exact())
			if err != nil {
				errs = append(errs, &ErrInvalidImage{srcImg.String(), err})
				continue
			}
			pinnedRef, err := imagesource.ParseReference(imgWithID)
			if err != nil {
				errs = append(errs, fmt.Errorf("error parsing source image %s: %v", imgWithID, err))
				continue
			}
			srcImg.Ref.ID = pinnedRef.Ref.ID
		}

		repo, err := regctx.RepositoryForRef(ctx, srcImg.Ref, insecure)
		if err != nil {
			errs = append(errs, &ErrInvalidImage{srcImg.Ref.Exact(), err})
			continue
		}

		ms, err := repo.Manifests(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("open blob: %v", err))
			continue
		}

		// TODO(estroz): parallelize
		associations, err := associateRemoteImageLayers(ctx, srcImg.String(), dstImg.String(), srcImg, ms, skipParse, insecure)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, association := range associations {
			bundleAssociations.Add(srcImg.String(), association)
		}
	}

	return bundleAssociations, utilerrors.NewAggregate(errs)
}

func associateRemoteImageLayers(ctx context.Context, srcImg, dstImg string, srcInfo TypedImage, ms distribution.ManifestService, skipParse func(string) bool, insecure bool) (associations []v1alpha2.Association, err error) {
	if skipParse(srcImg) {
		return nil, nil
	}

	dgst, err := digest.Parse(srcInfo.Ref.ID)
	if err != nil {
		return nil, err
	}
	mn, err := ms.Get(ctx, dgst, preferManifestList)
	if err != nil {
		return nil, &ErrInvalidImage{srcImg, err}
	}
	mt, payload, err := mn.Payload()
	if err != nil {
		return nil, err
	}

	association := v1alpha2.Association{
		Name:       srcImg,
		Path:       dstImg,
		ID:         srcInfo.Ref.ID,
		TagSymlink: srcInfo.Ref.Tag,
		Type:       srcInfo.Category,
	}
	switch mt {
	case "":
		return nil, errors.New("unparseable manifest mediaType")
	case imgspecv1.MediaTypeImageIndex, ctrsimgmanifest.DockerV2ListMediaType:
		list, err := ctrsimgmanifest.ListFromBlob(payload, mt)
		if err != nil {
			return nil, err
		}
		for _, instance := range list.Instances() {
			digestStr := instance.String()
			// Add manifest references so publish can recursively look up image layers
			// for the manifests of this list.
			association.ManifestDigests = append(association.ManifestDigests, digestStr)
			// Recurse on child manifests, which should be in the same directory
			// with the same file name as it's digest.
			childInfo := srcInfo
			childInfo.Ref.ID = digestStr
			childInfo.Ref.Tag = ""
			childAssocs, err := associateRemoteImageLayers(ctx, digestStr, dstImg, childInfo, ms, skipParse, insecure)
			if err != nil {
				return nil, err
			}
			associations = append(associations, childAssocs...)
		}
	default:
		// Treat all others as image manifests.
		manifest, err := ctrsimgmanifest.FromBlob(payload, mt)
		if err != nil {
			return nil, err
		}
		for _, layerInfo := range manifest.LayerInfos() {
			association.LayerDigests = append(association.LayerDigests, layerInfo.Digest.String())
		}
		// The config is just another blob, so associate it opaquely.
		association.LayerDigests = append(association.LayerDigests, manifest.ConfigInfo().Digest.String())
	}

	associations = append(associations, association)

	return associations, nil
}

// PreferManifestList specifically requests a manifest list first
var preferManifestList = distribution.WithManifestMediaTypes([]string{
	manifestlist.MediaTypeManifestList,
	schema2.MediaTypeManifest,
	imgspecv1.MediaTypeImageManifest,
	imgspecv1.MediaTypeImageIndex,
})
