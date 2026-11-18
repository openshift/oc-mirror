package delete

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/pkg/retry"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"

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

	// resolveManifest, when set, reports the manifest digest currently stored at a
	// destination reference. Tests use it in place of a registry. Production leaves
	// it nil and reads the destination (or, for a cache task, the local cache).
	resolveManifest func(ctx context.Context, ref string, cache bool) (digest.Digest, error)
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

// processDeleteItems processes the list of images and returns delete items.
// Identical destination references are kept once, including a signature tag shared
// by several images. Different tags are kept even when their origins share a digest:
// the destination registry is what decides whether those tags are one manifest.
func (o DeleteImages) processDeleteItems(ctx context.Context, images []v2alpha1.CopyImageSchema) []v2alpha1.DeleteItem {
	seen := make(map[string]struct{}, len(images))
	items := make([]v2alpha1.DeleteItem, 0, len(images)*2)
	for _, img := range images {
		if _, ok := seen[img.Destination]; ok {
			o.Log.Debug("duplicate image found %s", img.Destination)
			continue
		}
		seen[img.Destination] = struct{}{}
		items = append(items, v2alpha1.DeleteItem{
			ImageName:      img.Origin,
			ImageReference: img.Destination,
			Type:           img.Type,
		})

		if img.Type.IsOperatorCatalog() || img.Type == v2alpha1.TypeCincinnatiGraph {
			continue
		}

		for _, sig := range o.sigDeleteItems(ctx, img) {
			if _, ok := seen[sig.ImageReference]; ok {
				o.Log.Debug("duplicate signature found %s", sig.ImageReference)
				continue
			}
			seen[sig.ImageReference] = struct{}{}
			items = append(items, sig)
		}
	}
	return items
}

// DeleteRegistryImages - deletes both remote and local registries
func (o DeleteImages) DeleteRegistryImages(deleteImageList v2alpha1.DeleteImageList) error {
	o.Log.Debug("deleting images from remote registry")
	tasks, allErrs := o.deleteTasks(deleteImageList.Items)
	tasks = dedupeExactDestinations(tasks)
	if o.deletingFromRegistry() {
		tasks = o.collapseSharedManifests(tasks)
	}

	o.Opts.Stdout = io.Discard
	if o.deletingFromRegistry() {
		if _, err := o.Batch.Worker(context.Background(), collectorSchemaFromTasks(tasks), o.Opts); err != nil {
			o.Log.Warn("error during registry deletion: %v", err)
			allErrs = append(allErrs, err)
		}
	}

	return errors.Join(allErrs...)
}

// deleteTask is one deletion handed to the batch worker.
type deleteTask struct {
	schema v2alpha1.CopyImageSchema
	// cache is set for the local cache deletion added by --force-cache-delete.
	cache bool
}

// deleteTasks validates each planned reference and builds the remote deletion plus,
// when requested, its cache deletion. Invalid rows are reported and skipped.
func (o DeleteImages) deleteTasks(items []v2alpha1.DeleteItem) ([]deleteTask, []error) {
	tasks := make([]deleteTask, 0, len(items)*2)
	var allErrs []error
	for _, img := range items {
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
		if !strings.HasPrefix(imgSpecRef.Name, deleteDestBase+"/") {
			allErrs = append(allErrs, fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (expected prefix %s/, got %s)", o.Opts.Global.DeleteDestination, deleteDestBase, imgSpecRef.Name))
			continue
		}
		task := deleteTask{schema: v2alpha1.CopyImageSchema{
			Origin:      img.ImageName,
			Destination: img.ImageReference,
			Type:        img.Type,
		}}
		o.Log.Debug("deleting images %v", task.schema.Destination)
		tasks = append(tasks, task)

		if o.Opts.Global.ForceCacheDelete {
			cacheDestination := cacheDestinationFor(img, originSpec, imgSpecRef, o.Opts.Global.DeleteDestination, o.LocalStorageFQDN)
			o.Log.Debug("deleting images local cache %v", cacheDestination)
			tasks = append(tasks, deleteTask{schema: v2alpha1.CopyImageSchema{
				Origin:      img.ImageName,
				Destination: cacheDestination,
				Type:        img.Type,
			}, cache: true})
		}
	}
	return tasks, allErrs
}

// cacheDestinationFor is the local-cache reference for a validated delete item.
func cacheDestinationFor(img v2alpha1.DeleteItem, originSpec, imgSpecRef image.ImageSpec, deleteDestination, localStorageFQDN string) string {
	cacheDestination := img.ImageReference
	// OCPBUGS-105878 - tag+digest images are cached under the digest key, not the
	// human tag at the destination; rebuild it from the validated reference.
	if originSpec.IsImageByTagAndDigest() {
		cacheDestination = fmt.Sprintf("%s%s:%s", imgSpecRef.Transport, imgSpecRef.Name, originSpec.CacheTag())
	}
	cacheDestination = strings.ReplaceAll(cacheDestination, deleteDestination, consts.DockerProtocol+localStorageFQDN)
	// The cache keeps release images in the default repositories, even when
	// platform target repositories were used at the destination.
	if img.Type == v2alpha1.TypeOCPRelease {
		cacheDestination = consts.DockerProtocol + localStorageFQDN + "/" + releaseImagePathComponents + ":" + imgSpecRef.Tag
	}
	if img.Type == v2alpha1.TypeOCPReleaseContent {
		cacheDestination = consts.DockerProtocol + localStorageFQDN + "/" + releaseContentPathComponents + ":" + imgSpecRef.Tag
	}
	return cacheDestination
}

func dedupeExactDestinations(tasks []deleteTask) []deleteTask {
	seen := make(map[string]struct{}, len(tasks))
	out := make([]deleteTask, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := seen[task.schema.Destination]; ok {
			continue
		}
		seen[task.schema.Destination] = struct{}{}
		out = append(out, task)
	}
	return out
}

// collapseSharedManifests schedules one deletion for each manifest in a repository.
// Tag references are resolved against the registry that will be deleted. The origin
// digest is not used: it describes where the image was mirrored from, not what the
// destination tag points at now. A reference that already names a digest keeps that
// digest, so two digests that share a tag are both deleted. A reference that cannot
// be resolved is deleted as written, and the deletion reports the failure.
func (o DeleteImages) collapseSharedManifests(tasks []deleteTask) []deleteTask {
	scheduled := make(map[string]string, len(tasks))
	out := make([]deleteTask, 0, len(tasks))
	for _, task := range tasks {
		original := task.schema.Destination
		identity := o.manifestIdentity(&task)
		if previous, ok := scheduled[identity]; ok {
			o.Log.Info("%s addresses the same manifest as %s, which is already scheduled for deletion", original, previous)
			continue
		}
		scheduled[identity] = task.schema.Destination
		out = append(out, task)
	}
	return out
}

// manifestIdentity returns the registry, repository, and manifest digest for task.
// When the reference is a tag and the registry can be read, task.Destination is
// replaced by that digest so a later tag move cannot delete a different manifest.
func (o DeleteImages) manifestIdentity(task *deleteTask) string {
	spec, err := image.ParseRef(task.schema.Destination)
	if err != nil {
		return unresolvedIdentity(task)
	}
	dgst := spec.Algorithm + ":" + spec.Digest
	if spec.Digest == "" {
		resolved, err := o.destinationManifestDigest(task.schema.Destination, task.cache)
		if err != nil || resolved == "" {
			o.Log.Debug("leaving %s unchanged, manifest digest is unavailable: %v", task.schema.Destination, err)
			return unresolvedIdentity(task)
		}
		dgst = resolved.String()
		task.schema.Destination = spec.Transport + spec.Name + "@" + dgst
	}
	return taskLocation(task) + ":" + spec.Name + "@" + dgst
}

func unresolvedIdentity(task *deleteTask) string {
	return "unresolved:" + taskLocation(task) + ":" + task.schema.Destination
}

func taskLocation(task *deleteTask) string {
	if task.cache {
		return "cache"
	}
	return "remote"
}

// destinationManifestDigest reads the manifest digest stored at ref.
// Each lookup gets its own image timeout. One slow tag does not cancel the
// lookups that follow, and it does not share their deadline.
func (o DeleteImages) destinationManifestDigest(ref string, cache bool) (digest.Digest, error) {
	ctx, cancel := o.commandContext()
	defer cancel()
	if o.resolveManifest != nil {
		return o.resolveManifest(ctx, ref, cache)
	}
	if o.Opts.DestImage == nil {
		return "", fmt.Errorf("destination system context is not configured")
	}
	sysCtx, err := o.Opts.DestImage.NewSystemContext()
	if err != nil {
		return "", fmt.Errorf("destination system context: %w", err)
	}
	if cache || (o.LocalStorageFQDN != "" && strings.Contains(ref, o.LocalStorageFQDN)) {
		sysCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}
	var dgst digest.Digest
	err = retry.IfNecessary(ctx, func() error {
		var lookupErr error
		dgst, lookupErr = lookupManifestDigest(ctx, sysCtx, ref)
		return lookupErr
	}, mirror.RetryOptionsFrom(&o.Opts))
	if err != nil {
		return "", err
	}
	return dgst, nil
}

// lookupManifestDigest asks the named registry for ref's digest. It does not follow
// registries.conf pull mirrors: delete removes the manifest from this registry, so
// the digest has to be the one stored there.
func lookupManifestDigest(ctx context.Context, sysCtx *types.SystemContext, ref string) (digest.Digest, error) {
	if err := mirror.ReexecIfNecessaryForImages(ref); err != nil {
		return "", fmt.Errorf("prepare image lookup for %s: %w", ref, err)
	}
	srcRef, err := alltransports.ParseImageName(ref)
	if err != nil {
		return "", fmt.Errorf("parse destination %s: %w", ref, err)
	}
	dgst, err := docker.GetDigest(ctx, sysCtx, srcRef)
	if err != nil {
		return "", fmt.Errorf("read manifest digest %s: %w", ref, err)
	}
	return dgst, nil
}

func collectorSchemaFromTasks(tasks []deleteTask) v2alpha1.CollectorSchema {
	schema := v2alpha1.CollectorSchema{AllImages: make([]v2alpha1.CopyImageSchema, 0, len(tasks))}
	for _, task := range tasks {
		schema.AllImages = append(schema.AllImages, task.schema)
		switch {
		case task.schema.Type.IsRelease():
			schema.TotalReleaseImages++
		case task.schema.Type.IsOperator():
			schema.TotalOperatorImages++
		case task.schema.Type.IsAdditionalImage():
			schema.TotalAdditionalImages++
		case task.schema.Type.IsHelmImage():
			schema.TotalHelmImages++
		}
	}
	return schema
}

func (o DeleteImages) deletingFromRegistry() bool {
	return o.Opts.Global != nil && !o.Opts.Global.DeleteGenerate && len(o.Opts.Global.DeleteDestination) > 0
}

func (o DeleteImages) commandContext() (context.Context, context.CancelFunc) {
	if o.Opts.Global == nil {
		return context.Background(), func() {}
	}
	return o.Opts.Global.CommandTimeoutContext()
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
