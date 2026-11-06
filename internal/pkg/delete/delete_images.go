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

	"go.podman.io/common/pkg/retry"
	imagemanifest "go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
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
		if slices.Contains(duplicates, img.Destination) {
			o.Log.Debug("duplicate image found %s", img.Destination)
			continue
		}
		duplicates = append(duplicates, img.Destination)
		item := v2alpha1.DeleteItem{
			ImageName:      img.Origin,
			ImageReference: img.Destination,
			Type:           img.Type,
		}
		items = append(items, item)

		if img.Type.IsOperatorCatalog() || img.Type == v2alpha1.TypeCincinnatiGraph {
			continue
		}

		sigs := o.sigDeleteItems(ctx, img)
		items = append(items, sigs...)
	}
	return items
}

// DeleteRegistryImages - deletes both remote and local registries
func (o DeleteImages) DeleteRegistryImages(deleteImageList v2alpha1.DeleteImageList) error {
	o.Log.Debug("deleting images from remote registry")
	// all destination and cache deletions are derived (and validated) before any of them is
	// deduplicated or handed over to the batch worker
	tasks := make([]deleteTask, 0, len(deleteImageList.Items)*2)

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
			if imgSpecRef.Name != expectedPath {
				allErrs = append(allErrs, fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (expected release content at %s, got %s)", o.Opts.Global.DeleteDestination, expectedPath, imgSpecRef.Name))
				continue
			}
		case v2alpha1.TypeOCPRelease:
			expectedPath := deleteDestBase + "/" + releaseImagePathComponents
			if imgSpecRef.Name != expectedPath {
				allErrs = append(allErrs, fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (expected release at %s, got %s)", o.Opts.Global.DeleteDestination, expectedPath, imgSpecRef.Name))
				continue
			}
		default:
			if !strings.HasPrefix(imgSpecRef.Name, deleteDestBase+"/") {
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
		tasks = append(tasks, deleteTask{schema: cis, repository: imgSpecRef.Name, byDigest: imgSpecRef.Digest != ""})

		if o.Opts.Global.ForceCacheDelete {
			cacheDestination := img.ImageReference
			// OCPBUGS-105878 - tag+digest images are cached under the digest key, not the
			// human tag at the destination; rebuild it from the validated reference.
			if originSpec.IsImageByTagAndDigest() {
				cacheDestination = fmt.Sprintf("%s%s:%s", imgSpecRef.Transport, imgSpecRef.Name, originSpec.CacheTag())
			}
			cacheDestination = strings.ReplaceAll(cacheDestination, o.Opts.Global.DeleteDestination, consts.DockerProtocol+o.LocalStorageFQDN)
			cis := v2alpha1.CopyImageSchema{
				Origin:      img.ImageName,
				Destination: cacheDestination,
				Type:        img.Type,
			}
			o.Log.Debug("deleting images local cache %v", cis.Destination)
			tasks = append(tasks, deleteTask{schema: cis, repository: imgSpecRef.Name, cache: true})
		}
	}

	o.Opts.Stdout = io.Discard
	if !o.Opts.Global.DeleteGenerate && len(o.Opts.Global.DeleteDestination) > 0 {
		ctx := context.Background()
		var err error
		tasks, err = o.preflightPinnedRepositories(ctx, tasks)
		if err != nil {
			allErrs = append(allErrs, err)
			return errors.Join(allErrs...)
		}
		if _, err := o.Batch.Worker(ctx, deleteCollectorSchema(tasks), o.Opts); err != nil {
			o.Log.Warn("error during registry deletion: %v", err)
			allErrs = append(allErrs, err)
		}
	}

	return errors.Join(allErrs...)
}

// deleteTask is one deletion handed over to the batch worker.
type deleteTask struct {
	schema v2alpha1.CopyImageSchema
	// repository is the destination repository (registry and path) of the delete item the
	// task was derived from; a cache task keeps the repository of its destination item.
	repository string
	// cache is set for the local cache deletions of --force-cache-delete.
	cache bool
	// byDigest is set for destination deletions addressed by digest.
	byDigest bool
}

// deleteCollectorSchema builds the batch worker input, counting every scheduled deletion.
func deleteCollectorSchema(tasks []deleteTask) v2alpha1.CollectorSchema {
	collectorSchema := v2alpha1.CollectorSchema{AllImages: make([]v2alpha1.CopyImageSchema, 0, len(tasks))}
	for _, task := range tasks {
		collectorSchema.AllImages = append(collectorSchema.AllImages, task.schema)
		switch {
		case task.schema.Type.IsRelease():
			collectorSchema.TotalReleaseImages++
		case task.schema.Type.IsOperator():
			collectorSchema.TotalOperatorImages++
		case task.schema.Type.IsAdditionalImage():
			collectorSchema.TotalAdditionalImages++
		case task.schema.Type.IsHelmImage():
			collectorSchema.TotalHelmImages++
		}
	}
	return collectorSchema
}

// preflightPinnedRepositories prepares the deletions of the repositories in which the delete
// plan addresses at least one image by digest (see resolveDeleteTargets), together with their
// local cache deletions. Deletions of any other repository are returned unchanged and cause no
// registry access.
//
// In such a repository a tag and a digest, or two tags, can address the same manifest. The
// registry deletes a manifest (and every tag pointing at it) whatever reference is used, so
// deleting both references would delete the manifest twice and fail. Before anything is
// deleted, every tag reference of these repositories is therefore resolved, once, against the
// registry it belongs to, and replaced by the digest it points to: the reference that is
// deduplicated is the reference that is deleted, even if the tag is moved afterwards. A tag-only
// selection thus means the image the tag points to during this preflight. Deletions addressing
// the same manifest are scheduled once.
//
// If any tag cannot be resolved, nothing is deleted and an error is returned. Failures of the
// deletions themselves are reported by the batch worker, as for any other deletion.
func (o DeleteImages) preflightPinnedRepositories(ctx context.Context, tasks []deleteTask) ([]deleteTask, error) {
	pinned := pinnedRepositories(tasks)
	if pinned.Len() == 0 {
		return tasks, nil
	}
	resolver := &tagResolver{images: o, resolved: map[string]string{}}
	scheduled := sets.New[string]()
	prepared := make([]deleteTask, 0, len(tasks))
	var failures []error
	for _, task := range tasks {
		if !pinned.Has(task.repository) {
			prepared = append(prepared, task)
			continue
		}

		ref, err := resolver.digestReference(ctx, task.schema.Destination, task.cache)
		switch {
		case err != nil:
			failures = append(failures, err)
		case scheduled.Has(ref):
			o.Log.Info("%s addresses the same manifest as %s, which is already scheduled for deletion", task.schema.Destination, ref)
		default:
			scheduled.Insert(ref)
			task.schema.Destination = ref
			prepared = append(prepared, task)
		}
	}

	if len(failures) > 0 {
		return nil, fmt.Errorf("delete preflight failed, no images were deleted: %w", errors.Join(failures...))
	}
	return prepared, nil
}

// pinnedRepositories returns the repositories with at least one destination deletion addressed
// by digest.
func pinnedRepositories(tasks []deleteTask) sets.Set[string] {
	pinned := sets.New[string]()
	for _, task := range tasks {
		if task.byDigest {
			pinned.Insert(task.repository)
		}
	}
	return pinned
}

// preflightSystemContexts returns the system contexts used to read the destination registry,
// with the destination TLS settings, and the local cache, which is always served over plain HTTP
// (see mirror.delete).
func (o DeleteImages) preflightSystemContexts() (*types.SystemContext, *types.SystemContext, error) {
	if o.Manifest == nil {
		return nil, nil, errors.New("unable to resolve tag references: no manifest client configured")
	}
	if o.Opts.DestImage == nil {
		return nil, nil, errors.New("unable to resolve tag references: no destination image options configured")
	}
	destinationCtx, err := o.Opts.DestImage.NewSystemContext()
	if err != nil {
		return nil, nil, fmt.Errorf("destination system context: %w", err)
	}
	cacheCtx, err := o.Opts.DestImage.NewSystemContext()
	if err != nil {
		return nil, nil, fmt.Errorf("cache system context: %w", err)
	}
	cacheCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	return destinationCtx, cacheCtx, nil
}

// digestReferenceOf returns the reference of spec's repository addressed by dgst ("<algorithm>:<hex>").
func digestReferenceOf(spec image.ImageSpec, dgst string) string {
	return spec.Transport + spec.Name + "@" + dgst
}

// tagResolver resolves the tag references of the preflight to digest references, at most once
// per reference, so that a tag listed several times is frozen to a single digest. The system
// contexts are only created when a tag has to be resolved.
type tagResolver struct {
	images      DeleteImages
	resolved    map[string]string
	destination *types.SystemContext
	cache       *types.SystemContext
}

// digestReference returns ref addressed by digest. A tag reference is resolved, against the
// local cache when cache is set and against the destination registry otherwise, to the digest of
// the manifest it currently points to.
func (r *tagResolver) digestReference(ctx context.Context, ref string, cache bool) (string, error) {
	spec, err := image.ParseRef(ref)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", ref, err)
	}
	if spec.Digest != "" {
		return digestReferenceOf(spec, spec.Algorithm+":"+spec.Digest), nil
	}
	if digestRef, ok := r.resolved[ref]; ok {
		return digestRef, nil
	}

	sysCtx, err := r.systemContext(cache)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	manifestDigest, err := r.images.manifestDigest(ctx, sysCtx, ref)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	digestRef := digestReferenceOf(spec, manifestDigest.String())
	r.images.Log.Debug("%s resolved to %s", ref, digestRef)
	r.resolved[ref] = digestRef
	return digestRef, nil
}

// systemContext returns the system context to read the local cache or the destination registry.
func (r *tagResolver) systemContext(cache bool) (*types.SystemContext, error) {
	if r.destination == nil {
		destinationCtx, cacheCtx, err := r.images.preflightSystemContexts()
		if err != nil {
			return nil, err
		}
		r.destination, r.cache = destinationCtx, cacheCtx
	}
	if cache {
		return r.cache, nil
	}
	return r.destination, nil
}

// manifestDigest returns the digest of the manifest ref points to, computed the same way as the
// registry deletion does (go.podman.io/image docker deleteImage).
func (o DeleteImages) manifestDigest(ctx context.Context, sysCtx *types.SystemContext, ref string) (digest.Digest, error) {
	retryOpts := retry.Options{}
	if o.Opts.RetryOpts != nil {
		retryOpts = *o.Opts.RetryOpts
	}
	retryOpts.IsErrorRetryable = mirror.IsErrorRetryable

	var manifestDigest digest.Digest
	err := retry.IfNecessary(ctx, func() error {
		lookupCtx := ctx
		if o.Opts.Global != nil && o.Opts.Global.CommandTimeout > 0 {
			var cancel context.CancelFunc
			lookupCtx, cancel = context.WithTimeout(ctx, o.Opts.Global.CommandTimeout)
			defer cancel()
		}
		manifestBytes, _, err := o.Manifest.ImageManifest(lookupCtx, sysCtx, ref, nil)
		if err != nil {
			return err
		}
		manifestDigest, err = imagemanifest.Digest(manifestBytes)
		if err != nil {
			return fmt.Errorf("compute manifest digest: %w", err)
		}
		return nil
	}, &retryOpts)
	if err != nil {
		return "", fmt.Errorf("get manifest: %w", err)
	}
	return manifestDigest, nil
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
