package delete

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
	"github.com/openshift/oc-mirror/v2/internal/pkg/signature"
)

type DeleteImages struct {
	Log              clog.PluggableLoggerInterface
	Opts             mirror.CopyOptions
	Batch            batch.BatchInterface
	Blobs            archive.BlobsGatherer
	Config           v2alpha1.ImageSetConfiguration
	Manifest         manifest.ManifestInterface
	SigHandler       signature.SignatureInterface
	LocalStorageDisk string
	LocalStorageFQDN string
}

// WriteDeleteMetaData
func (o DeleteImages) WriteDeleteMetaData(ctx context.Context, images []v2alpha1.CopyImageSchema) error {
	o.Log.Info(emoji.PageFacingUp + " Generating delete file...")
	o.Log.Info("%s file created", o.Opts.Global.WorkingDir+deleteDir)

	// we write the image and related blobs in yaml format to file for further processing
	filename := filepath.Join(o.Opts.Global.WorkingDir, deleteImagesYaml)
	discYamlFile := filepath.Join(o.Opts.Global.WorkingDir, discYaml)
	// used for versioning and comparing
	if len(o.Opts.Global.DeleteID) > 0 {
		filename = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(deleteImagesYaml, ".", "-"+o.Opts.Global.DeleteID+"."))
		discYamlFile = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(discYaml, ".", "-"+o.Opts.Global.DeleteID+"."))
	}
	// create the delete folder
	err := os.MkdirAll(o.Opts.Global.WorkingDir+deleteDir, 0755)
	if err != nil {
		o.Log.Error("%v ", err)
	}

	items := o.processDeleteItems(ctx, images)

	// sort the items
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ImageReference < items[j].ImageReference
	})
	// marshal to yaml and write to file
	deleteImageList := v2alpha1.DeleteImageList{
		Kind:       "DeleteImageList",
		APIVersion: "mirror.openshift.io/v2alpha1",
		Items:      items,
	}
	ymlData, err := yaml.Marshal(deleteImageList)
	if err != nil {
		o.Log.Error(deleteImagesErrMsg, err)
	}
	err = os.WriteFile(filename, ymlData, 0644) //nolint:gosec
	if err != nil {
		o.Log.Error(deleteImagesErrMsg, err)
	}
	// finally copy the deleteimagesetconfig for reference
	disc := v2alpha1.DeleteImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeleteImageSetConfiguration",
			APIVersion: "mirror.openshift.io/v2alpha1",
		},
		DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
			Delete: v2alpha1.Delete{
				Platform:         o.Config.Mirror.Platform,
				Operators:        o.Config.Mirror.Operators,
				AdditionalImages: o.Config.Mirror.AdditionalImages,
				Helm:             o.Config.Mirror.Helm,
			},
		},
	}
	discYamlData, err := yaml.Marshal(disc)
	if err != nil {
		o.Log.Error("%v ", err)
	}
	err = os.WriteFile(discYamlFile, discYamlData, 0644) //nolint:gosec
	if err != nil {
		o.Log.Error(deleteImagesErrMsg, err)
	}
	return nil
}

// processDeleteItems processes the list of images and returns delete items
func (o DeleteImages) processDeleteItems(ctx context.Context, images []v2alpha1.CopyImageSchema) []v2alpha1.DeleteItem {
	duplicates := []string{}
	items := make([]v2alpha1.DeleteItem, 0, len(images)*2)
	for _, img := range images {
		unflatDest := img.Destination
		flatDest := unflatDest
		unflatField := ""
		if o.Opts.Global.MaxNestedPaths > 0 {
			var err error
			flatDest, err = image.WithMaxNestedPaths(unflatDest, o.Opts.Global.MaxNestedPaths)
			if err != nil {
				o.Log.Error("max-nested-paths flatten %q: %v", unflatDest, err)
				continue
			}
			if flatDest != unflatDest {
				unflatField = unflatDest
			}
		}
		if slices.Contains(duplicates, flatDest) {
			o.Log.Debug("duplicate image found %s", flatDest)
			continue
		}
		duplicates = append(duplicates, flatDest)
		item := v2alpha1.DeleteItem{
			ImageName:                 img.Origin,
			ImageReference:            flatDest,
			UnflattenedImageReference: unflatField,
			Type:                      img.Type,
		}
		items = append(items, item)

		if img.Type.IsOperatorCatalog() || img.Type == v2alpha1.TypeCincinnatiGraph {
			continue
		}

		imgFlat := img
		imgFlat.Destination = flatDest
		sigs := o.sigDeleteItems(ctx, imgFlat)
		if unflatField != "" {
			imgUnflat := img
			imgUnflat.Destination = unflatDest
			unflatSigs := o.sigDeleteItems(ctx, imgUnflat)
			for i := range sigs {
				if i < len(unflatSigs) {
					sigs[i].UnflattenedImageReference = unflatSigs[i].ImageReference
				}
			}
		}
		items = append(items, sigs...)
	}
	return items
}

// DeleteRegistryImages - deletes both remote and local registries
func (o DeleteImages) DeleteRegistryImages(deleteImageList v2alpha1.DeleteImageList) error {
	o.Log.Debug("deleting images from remote registry")
	collectorSchema := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{}}

	increment := 1
	if o.Opts.Global.ForceCacheDelete {
		increment = 2
	}

	allErrs := []error{}
	for _, img := range deleteImageList.Items {
		// OCPBUGS-43489
		// Verify that the "delete" destination is set correctly
		// It does not hurt to check each entry :)
		// This will avoid the error "Image may not exist or is not stored with a v2 Schema in a v2 registry"
		// Reverts OCPBUGS-44448
		originSpec, err := image.ParseRef(img.ImageName)
		if err != nil {
			allErrs = append(allErrs, fmt.Errorf("parse image name %q: %w", img.ImageName, err))
			continue
		}
		imgSpecRef, err := image.ParseRef(img.ImageReference)
		if err != nil {
			allErrs = append(allErrs, fmt.Errorf("parse imag eref %q: %w", img.ImageReference, err))
			continue
		}
		// remove dockerProtocol
		name := strings.Split(o.Opts.Global.DeleteDestination, consts.DockerProtocol)
		// this should not occur - but just incase
		if len(name) < 2 {
			allErrs = append(allErrs, fmt.Errorf("delete destination is not well formed (%s) - missing dockerProtocol?", o.Opts.Global.DeleteDestination))
			continue
		}
		deleteDestBase := name[1]
		// Release content is always mirrored to standardized paths and Platform type does not support targetRepo/tag.
		// For these types, we use strict equality checking with the expected paths.
		// For other types that support targetRepo/targetTag, we only verify the image is under the correct
		// registry tree for flexibility needed with targetRepo/targetTag.
		switch img.Type {
		case v2alpha1.TypeOCPReleaseContent:
			expectedPath := deleteDestBase + "/" + releaseContentPathComponents
			if !matchesExpectedReleaseRepo(imgSpecRef.Name, deleteDestBase, releaseContentPathComponents) {
				allErrs = append(allErrs, fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (expected release content at %s, got %s)", o.Opts.Global.DeleteDestination, expectedPath, imgSpecRef.Name))
				continue
			}
		case v2alpha1.TypeOCPRelease:
			expectedPath := deleteDestBase + "/" + releaseImagePathComponents
			if !matchesExpectedReleaseRepo(imgSpecRef.Name, deleteDestBase, releaseImagePathComponents) {
				allErrs = append(allErrs, fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (expected release at %s, got %s)", o.Opts.Global.DeleteDestination, expectedPath, imgSpecRef.Name))
				continue
			}
		default:
			if !matchesUnderDeleteDestination(imgSpecRef.Name, deleteDestBase, o.Opts.Global.MaxNestedPaths, img.UnflattenedImageReference) {
				allErrs = append(allErrs, fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (expected prefix %s/, got %s)", o.Opts.Global.DeleteDestination, deleteDestBase, imgSpecRef.Name))
				continue
			}
		}
		cis := v2alpha1.CopyImageSchema{
			Origin:      img.ImageName,
			Destination: img.ImageReference,
			Type:        img.Type,
		}
		o.Log.Debug("deleting images %v", cis.Destination)
		collectorSchema.AllImages = append(collectorSchema.AllImages, cis)

		if o.Opts.Global.ForceCacheDelete {
			// Prefer unflattened ref: local cache stores pre-max-nested-paths paths.
			cacheRef := img.ImageReference
			if img.UnflattenedImageReference != "" {
				cacheRef = img.UnflattenedImageReference
			}
			cacheSpec, err := image.ParseRef(cacheRef)
			if err != nil {
				allErrs = append(allErrs, fmt.Errorf("parse cache image ref %q: %w", cacheRef, err))
				continue
			}
			cacheDestination := cacheRef
			// OCPBUGS-105878 - tag+digest images are cached under the digest key, not the
			// human tag at the destination; rebuild it from the validated reference.
			if originSpec.IsImageByTagAndDigest() {
				cacheDestination = fmt.Sprintf("%s%s:%s", cacheSpec.Transport, cacheSpec.Name, originSpec.CacheTag())
			}
			cacheDestination = strings.ReplaceAll(cacheDestination, o.Opts.Global.DeleteDestination, consts.DockerProtocol+o.LocalStorageFQDN)
			cis := v2alpha1.CopyImageSchema{
				Origin:      img.ImageName,
				Destination: cacheDestination,
				Type:        img.Type,
			}
			o.Log.Debug("deleting images local cache %v", cis.Destination)
			collectorSchema.AllImages = append(collectorSchema.AllImages, cis)
		}

		switch {
		case img.Type.IsRelease():
			collectorSchema.TotalReleaseImages += increment
		case img.Type.IsOperator():
			collectorSchema.TotalOperatorImages += increment
		case img.Type.IsAdditionalImage():
			collectorSchema.TotalAdditionalImages += increment
		case img.Type.IsHelmImage():
			collectorSchema.TotalHelmImages += increment
		}
	}

	o.Opts.Stdout = io.Discard
	if !o.Opts.Global.DeleteGenerate && len(o.Opts.Global.DeleteDestination) > 0 {
		if _, err := o.Batch.Worker(context.Background(), collectorSchema, o.Opts); err != nil {
			o.Log.Warn("error during registry deletion: %v", err)
			allErrs = append(allErrs, err)
		}
	}

	return errors.Join(allErrs...)
}

// matchesUnderDeleteDestination reports whether gotName is under deleteDestBase
// (unflattened prefix) or a valid --max-nested-paths flattening of a path under
// that destination. When unflattenedRef is set (from delete YAML), also accept
// gotName if it is a flattening of that unflattened reference.
func matchesUnderDeleteDestination(gotName, deleteDestBase string, maxNestedPaths int, unflattenedRef string) bool {
	if gotName == deleteDestBase || strings.HasPrefix(gotName, deleteDestBase+"/") {
		return true
	}
	if maxNestedPaths > 0 && matchesFlattenedUnderDeleteDest(gotName, deleteDestBase, maxNestedPaths) {
		return true
	}
	return matchesAnyFlatteningOfUnflat(gotName, deleteDestBase, unflattenedRef)
}

// matchesAnyFlatteningOfUnflat reports whether gotName equals unflattenedRef's
// repository name or any WithMaxNestedPaths flattening of that reference, and
// that the unflattened name is under deleteDestBase.
func matchesAnyFlatteningOfUnflat(gotName, deleteDestBase, unflattenedRef string) bool {
	if unflattenedRef == "" {
		return false
	}
	unflatSpec, err := image.ParseRef(unflattenedRef)
	if err != nil {
		return false
	}
	if unflatSpec.Name != deleteDestBase && !strings.HasPrefix(unflatSpec.Name, deleteDestBase+"/") {
		return false
	}
	if gotName == unflatSpec.Name {
		return true
	}
	return gotNameMatchesAnyMaxNestedFlattening(gotName, unflattenedRef)
}

// matchesFlattenedUnderDeleteDest accepts names produced when WithMaxNestedPaths
// absorbs trailing destination path components into a dash-joined segment
// (e.g. dest test/72942 + max=2 → test/72942-compliance-op).
func matchesFlattenedUnderDeleteDest(gotName, deleteDestBase string, maxNestedPaths int) bool {
	if maxNestedPaths <= 0 {
		return false
	}
	destSpec, gotSpec, ok := parsePathCheckNamePair(deleteDestBase, gotName)
	if !ok {
		return false
	}
	if domainsConflict(destSpec.Domain, gotSpec.Domain) {
		return false
	}
	// maxNestedPaths > 0, so keep is never negative.
	keep := maxNestedPaths - 1
	destComps := splitPathComponent(destSpec.PathComponent)
	if len(destComps) <= keep {
		return strings.HasPrefix(gotName, deleteDestBase+"/")
	}
	prefixName := joinRegistryName(gotSpec.Domain, flatDestPathPrefix(destComps, keep))
	return gotName == prefixName || strings.HasPrefix(gotName, prefixName+"-")
}

func parsePathCheckNamePair(deleteDestBase, gotName string) (destSpec, gotSpec image.ImageSpec, ok bool) {
	destSpec, err := image.ParseRef(consts.DockerProtocol + deleteDestBase + ":oc-mirror-path-check")
	if err != nil {
		return image.ImageSpec{}, image.ImageSpec{}, false
	}
	gotSpec, err = image.ParseRef(consts.DockerProtocol + gotName + ":oc-mirror-path-check")
	if err != nil {
		return image.ImageSpec{}, image.ImageSpec{}, false
	}
	return destSpec, gotSpec, true
}

func domainsConflict(destDomain, gotDomain string) bool {
	return destDomain != "" && gotDomain != "" && destDomain != gotDomain
}

func splitPathComponent(pathComponent string) []string {
	if pathComponent == "" {
		return nil
	}
	return strings.Split(pathComponent, "/")
}

func flatDestPathPrefix(destComps []string, keep int) string {
	kept := strings.Join(destComps[:keep], "/")
	joined := strings.Join(destComps[keep:], "-")
	if kept == "" {
		return joined
	}
	return kept + "/" + joined
}

func joinRegistryName(domain, pathPrefix string) string {
	if domain == "" {
		return pathPrefix
	}
	return domain + "/" + pathPrefix
}

// matchesExpectedReleaseRepo reports whether gotName is the canonical release
// repository under deleteDestBase, or a valid WithMaxNestedPaths flattening of it.
func matchesExpectedReleaseRepo(gotName, deleteDestBase, releasePathComponents string) bool {
	expected := deleteDestBase + "/" + releasePathComponents
	if gotName == expected {
		return true
	}
	// ParseRef requires a tag or digest; use a placeholder for path-only matching.
	ref := consts.DockerProtocol + expected + ":oc-mirror-path-check"
	return gotNameMatchesAnyMaxNestedFlattening(gotName, ref)
}

// gotNameMatchesAnyMaxNestedFlattening reports whether gotName equals the
// repository Name of imageRef after applying WithMaxNestedPaths for any valid n.
func gotNameMatchesAnyMaxNestedFlattening(gotName, imageRef string) bool {
	spec, err := image.ParseRef(imageRef)
	if err != nil {
		return false
	}
	comps := strings.Split(spec.PathComponent, "/")
	for n := 1; n < len(comps); n++ {
		flat, err := image.WithMaxNestedPaths(imageRef, n)
		if err != nil {
			continue
		}
		flatSpec, err := image.ParseRef(flat)
		if err != nil {
			continue
		}
		if gotName == flatSpec.Name {
			return true
		}
	}
	return false
}

// ReadDeleteMetaData - read the list of images to delete
// used to verify the delete yaml is well formed as well as being
// the base for both local cache delete and remote registry delete
func (o DeleteImages) ReadDeleteMetaData() (v2alpha1.DeleteImageList, error) {
	o.Log.Info(emoji.Eyes + " Reading delete file...")
	var fileName string

	if len(o.Opts.Global.DeleteYaml) == 0 {
		fileName = filepath.Join(o.Opts.Global.WorkingDir, deleteImagesYaml)
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			return v2alpha1.DeleteImageList{}, fmt.Errorf("delete yaml file %s does not exist (please perform a delete with --dry-run)", fileName)
		}
	} else {
		fileName = o.Opts.Global.DeleteYaml
	}

	// lets parse the file to get the images
	list, err := parser.ParseYamlFile[v2alpha1.DeleteImageList](fileName)
	if err != nil {
		return v2alpha1.DeleteImageList{}, fmt.Errorf("delete image list: %w", err)
	}
	return list, nil
}

func (o DeleteImages) sigDeleteItems(ctx context.Context, img v2alpha1.CopyImageSchema) []v2alpha1.DeleteItem {
	items := []v2alpha1.DeleteItem{}

	if !o.Opts.Global.DeleteSignatures {
		return items
	}

	sigs, err := o.SigHandler.GetSignatureTag(ctx, img.Source)
	if err != nil {
		item := o.getSignatureTagWithoutCache(img)
		if item != nil {
			items = append(items, *item)
		}

		return items
	}

	for _, sig := range sigs {
		item := o.sigDeleteItem(img, sig)
		if item != nil {
			items = append(items, *item)
		}
	}

	return items
}

// GetSignatureTagWithoutCache tries to generate a signature tag when the image was not cached (mirror to mirror workflow).
// it only returns the signature tag if the image was referenced by digest and it does not delete multi arch signatures
func (o DeleteImages) getSignatureTagWithoutCache(img v2alpha1.CopyImageSchema) *v2alpha1.DeleteItem {
	imgSpec, err := image.ParseRef(img.Origin)

	if err == nil && imgSpec.Digest != "" {
		digest := digest.NewDigestFromEncoded(digest.Algorithm(imgSpec.Algorithm), imgSpec.Digest)
		sig, err := signature.SigstoreAttachmentTag(digest)
		if err == nil {
			return o.sigDeleteItem(img, sig)
		}
	}

	return nil
}

func (o DeleteImages) sigDeleteItem(img v2alpha1.CopyImageSchema, sig string) *v2alpha1.DeleteItem {
	if sig == "" {
		return nil
	}

	imgOriginRef, err := image.ParseRef(img.Origin)
	if err != nil {
		return nil
	}
	imgDestRef, err := image.ParseRef(img.Destination)
	if err != nil {
		return nil
	}

	originSigRef := imgOriginRef.Name + ":" + sig
	destSigRef := imgDestRef.Transport + imgDestRef.Name + ":" + sig

	return &v2alpha1.DeleteItem{
		ImageName:      originSigRef,
		ImageReference: destSigRef,
		Type:           img.Type,
	}
}
