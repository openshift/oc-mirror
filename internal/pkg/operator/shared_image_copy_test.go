package operator

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/customsort"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// OCPBUGS-132865: an image can be the bundle image of one bundle and a related image of
// another. Its copy task must keep the related image type, whatever the order in which the
// copy batch builders see the two requests, so that the batch worker does not skip it when
// the bundle it belongs to has a related image that failed to copy.

// sharedImageCopyBuilders returns the operator copy batch builders, one per workflow.
func sharedImageCopyBuilders(t *testing.T) map[string]func(map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	t.Helper()
	log := clog.New("error")

	m2d := setupFilterCollector_MirrorToDisk(t.TempDir(), log, &MockManifest{})
	m2m := setupFilterCollector_MirrorToDisk(t.TempDir(), log, &MockManifest{})
	m2m.Opts.Mode = mirror.MirrorToMirror
	m2m.Opts.Destination = "docker://localhost:5000/test"
	d2m := setupFilterCollector_DiskToMirror(t.TempDir(), log)
	del := setupFilterCollector_DiskToMirror(t.TempDir(), log)
	del.Opts.Function = string(mirror.DeleteMode)

	return map[string]func(map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error){
		"mirrorToDisk":   m2d.prepareM2DCopyBatch,
		"mirrorToMirror": m2m.dispatchImagesForM2M,
		"diskToMirror":   d2m.prepareD2MCopyBatch,
		"delete":         del.prepareD2MCopyBatch,
	}
}

// copiesOf returns the copy tasks created for the given image.
func copiesOf(copies []v2alpha1.CopyImageSchema, img string) []v2alpha1.CopyImageSchema {
	var found []v2alpha1.CopyImageSchema
	for _, c := range copies {
		if c.Origin == consts.DockerProtocol+img {
			found = append(found, c)
		}
	}
	return found
}

func TestSharedBundleImageCopyType(t *testing.T) {
	const (
		unsharedBundle  = "quay.io/example/unshared-bundle@sha256:5555555555555555555555555555555555555555555555555555555555555555"
		unsharedRelated = "quay.io/example/unshared-operand@sha256:6666666666666666666666666666666666666666666666666666666666666666"
	)
	catalog := v2alpha1.RelatedImage{
		Name:       "redhat-operator-index",
		Image:      "registry.redhat.io/redhat/redhat-operator-index:v4.17",
		Type:       v2alpha1.TypeOperatorCatalog,
		RebuiltTag: "fbf7a9e933d930758fcf18e1c6e6deff3",
	}
	// A catalog and an unshared bundle, whose copy tasks must not change.
	controls := map[string][]v2alpha1.RelatedImage{
		"catalog":  {catalog},
		"c.v1.0.0": {{Image: unsharedBundle, Type: v2alpha1.TypeOperatorBundle}, {Name: "operand", Image: unsharedRelated, Type: v2alpha1.TypeOperatorRelatedImage}},
	}
	sharedRefs := map[string]string{
		"digest":         "quay.io/example/shared-bundle@sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"tag":            "quay.io/example/shared-bundle:v1.0.0",
		"tag and digest": "quay.io/example/shared-bundle:v1.0.0@sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}

	for refName, shared := range sharedRefs {
		asBundle := v2alpha1.RelatedImage{Image: shared, Type: v2alpha1.TypeOperatorBundle}
		asRelated := v2alpha1.RelatedImage{Name: "uses-shared", Image: shared, Type: v2alpha1.TypeOperatorRelatedImage}
		inputs := map[string]map[string][]v2alpha1.RelatedImage{
			"bundle image first":  {"a.v1.0.0": {asBundle, asRelated}},
			"related image first": {"b.v1.0.0": {asRelated, asBundle}},
			"separate bundles": {
				"a.v1.0.0": {asBundle},
				"b.v1.0.0": {asRelated},
			},
		}

		for mode, build := range sharedImageCopyBuilders(t) {
			// The copy task the builder creates when the image is requested only once.
			bundleOnly, err := build(map[string][]v2alpha1.RelatedImage{"a.v1.0.0": {asBundle}})
			require.NoError(t, err)
			require.Len(t, bundleOnly, 1)
			relatedOnly, err := build(map[string][]v2alpha1.RelatedImage{"b.v1.0.0": {asRelated}})
			require.NoError(t, err)
			require.Len(t, relatedOnly, 1)
			assert.Equal(t, v2alpha1.TypeOperatorBundle, bundleOnly[0].Type, "%s %s: bundle image requested once", mode, refName)
			assert.Equal(t, v2alpha1.TypeOperatorRelatedImage, relatedOnly[0].Type, "%s %s: related image requested once", mode, refName)
			expected := relatedOnly[0]
			// Only the type differs between the two roles: references and tags are the same.
			bundleAsRelated := bundleOnly[0]
			bundleAsRelated.Type = v2alpha1.TypeOperatorRelatedImage
			assert.Equal(t, expected, bundleAsRelated, "%s %s: bundle and related copy tasks differ beyond their type", mode, refName)

			controlCopies, err := build(controls)
			require.NoError(t, err)

			for inputName, input := range inputs {
				t.Run(fmt.Sprintf("%s/%s/%s", mode, refName, inputName), func(t *testing.T) {
					withControls := maps.Clone(input)
					maps.Copy(withControls, controls)
					// Go randomizes map iteration: repeat so that every order of the bundles is seen.
					for range 20 {
						copies, err := build(withControls)
						require.NoError(t, err)

						got := copiesOf(copies, shared)
						require.Len(t, got, 1, "the shared image must be copied exactly once")
						assert.Equal(t, expected, got[0], "the shared image must be copied as a related image with unchanged references")

						others := slices.DeleteFunc(slices.Clone(copies), func(c v2alpha1.CopyImageSchema) bool {
							return c.Origin == consts.DockerProtocol+shared
						})
						assert.ElementsMatch(t, controlCopies, others, "catalog and unshared images must not be affected")
					}
				})
			}
		}
	}

	t.Run("controls", func(t *testing.T) {
		builders := sharedImageCopyBuilders(t)
		expectedCatalogCopies := map[string]int{"mirrorToDisk": 1, "mirrorToMirror": 2, "diskToMirror": 1, "delete": 0}
		for mode, build := range builders {
			copies, err := build(controls)
			require.NoError(t, err)
			bundle := copiesOf(copies, unsharedBundle)
			require.Len(t, bundle, 1, mode)
			assert.Equal(t, v2alpha1.TypeOperatorBundle, bundle[0].Type, "%s: an unshared bundle image stays a bundle image", mode)
			related := copiesOf(copies, unsharedRelated)
			require.Len(t, related, 1, mode)
			assert.Equal(t, v2alpha1.TypeOperatorRelatedImage, related[0].Type, "%s: an unshared related image stays a related image", mode)
			catalogCopies := copiesOf(copies, catalog.Image)
			assert.Len(t, catalogCopies, expectedCatalogCopies[mode], "%s: catalog copy tasks", mode)
			for _, c := range catalogCopies {
				assert.Equal(t, v2alpha1.TypeOperatorCatalog, c.Type, mode)
			}
		}
	})
}

const (
	sharedTestA = "quay.io/example/a-bundle@sha256:1111111111111111111111111111111111111111111111111111111111111111"    // bundle image of a, related image of b
	sharedTestR = "quay.io/example/a-operand-r@sha256:2222222222222222222222222222222222222222222222222222222222222222" // related image of a and d, fails to copy
	sharedTestO = "quay.io/example/a-operand-o@sha256:3333333333333333333333333333333333333333333333333333333333333333" // related image of a
	sharedTestB = "quay.io/example/b-bundle@sha256:4444444444444444444444444444444444444444444444444444444444444444"    // bundle image of b
	sharedTestD = "quay.io/example/d-bundle@sha256:5555555555555555555555555555555555555555555555555555555555555555"    // bundle image of d, not used by other bundles
)

// failureRecordingLogger records the images the batch worker reports as failed. The worker
// logs a failure after adding it to the errors that later skip decisions use.
type failureRecordingLogger struct {
	clog.PluggableLoggerInterface
	mu       sync.Mutex
	failed   []string
	once     sync.Once
	recorded chan struct{} // closed when the failure of sharedTestR has been recorded
}

func (l *failureRecordingLogger) Error(msg string, val ...interface{}) {
	line := fmt.Sprintf(msg, val...)
	l.mu.Lock()
	l.failed = append(l.failed, line)
	l.mu.Unlock()
	if strings.Contains(line, sharedTestR) {
		l.once.Do(func() { close(l.recorded) })
	}
}

// failingMirror fails to copy sharedTestR. Images copied after it wait until the worker has
// recorded that failure, so that later skip decisions deterministically take it into account.
type failingMirror struct {
	mu       sync.Mutex
	copied   []string
	failSrc  string
	failed   atomic.Bool
	recorded chan struct{}
}

// imageFailures returns the per-image failure reports of the batch worker.
func (l *failureRecordingLogger) imageFailures() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(l.failed), func(line string) bool { return !strings.HasPrefix(line, "Failed to copy ") })
}

func (m *failingMirror) Run(_ context.Context, src, _ string, _ mirror.Mode, _ *mirror.CopyOptions) error {
	if src == m.failSrc {
		m.failed.Store(true)
		return errors.New("copy failed")
	}
	if m.failed.Load() {
		select {
		case <-m.recorded:
		case <-time.After(30 * time.Second):
			return errors.New("timed out waiting for the copy failure to be recorded")
		}
	}
	m.mu.Lock()
	m.copied = append(m.copied, src)
	m.mu.Unlock()
	return nil
}

func (m *failingMirror) Check(context.Context, string, *mirror.CopyOptions, bool) (bool, error) {
	return true, nil
}

// sharedImageCatalogs returns two catalogs. In the first, bundle a has sharedTestA as its
// bundle image, listed in its related images or not, and bundle d does not share its bundle
// image; both have sharedTestR as a related image. In the second, bundle b needs sharedTestA
// as a related image.
func sharedImageCatalogs(aListsBundleImage bool) (*declcfg.DeclarativeConfig, *declcfg.DeclarativeConfig) {
	pkg := func(name string) property.Property {
		return property.Property{Type: property.TypePackage, Value: []byte(fmt.Sprintf(`{"packageName":%q,"version":"1.0.0"}`, name))}
	}
	channel := func(name string) declcfg.Channel {
		return declcfg.Channel{Schema: declcfg.SchemaChannel, Package: name, Name: "stable", Entries: []declcfg.ChannelEntry{{Name: name + ".v1.0.0"}}}
	}
	aRelated := []declcfg.RelatedImage{{Name: "r", Image: sharedTestR}, {Name: "o", Image: sharedTestO}}
	if aListsBundleImage {
		aRelated = append([]declcfg.RelatedImage{{Image: sharedTestA}}, aRelated...)
	}
	first := &declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{{Schema: declcfg.SchemaPackage, Name: "a", DefaultChannel: "stable"}, {Schema: declcfg.SchemaPackage, Name: "d", DefaultChannel: "stable"}},
		Channels: []declcfg.Channel{channel("a"), channel("d")},
		Bundles: []declcfg.Bundle{
			{Schema: declcfg.SchemaBundle, Name: "a.v1.0.0", Package: "a", Image: sharedTestA, Properties: []property.Property{pkg("a")}, RelatedImages: aRelated},
			{Schema: declcfg.SchemaBundle, Name: "d.v1.0.0", Package: "d", Image: sharedTestD, Properties: []property.Property{pkg("d")},
				RelatedImages: []declcfg.RelatedImage{{Image: sharedTestD}, {Name: "r", Image: sharedTestR}}},
		},
	}
	second := &declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{{Schema: declcfg.SchemaPackage, Name: "b", DefaultChannel: "stable"}},
		Channels: []declcfg.Channel{channel("b")},
		Bundles: []declcfg.Bundle{
			{Schema: declcfg.SchemaBundle, Name: "b.v1.0.0", Package: "b", Image: sharedTestB, Properties: []property.Property{pkg("b")},
				RelatedImages: []declcfg.RelatedImage{{Image: sharedTestB}, {Name: "uses-a-bundle", Image: sharedTestA}}},
		},
	}
	return first, second
}

func sharedImageCopyOptions(mode string) mirror.CopyOptions {
	global := &mirror.GlobalOptions{IsTerminal: false}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()
	return mirror.CopyOptions{
		Global: global, DeprecatedTLSVerify: deprecatedTLSVerifyOpt, SrcImage: srcOpts, DestImage: destOpts, RetryOpts: retryOpts,
		Destination: "oci:test", Mode: mode, Function: string(mirror.CopyMode),
	}
}

func TestSharedBundleImageCopiedWhenItsBundleFails(t *testing.T) {
	log := clog.New("error")
	modes := map[string]string{
		"mirrorToDisk":   mirror.MirrorToDisk,
		"mirrorToMirror": mirror.MirrorToMirror,
		"diskToMirror":   mirror.DiskToMirror,
	}

	for _, aListsBundleImage := range []bool{false, true} {
		for name, mode := range modes {
			t.Run(fmt.Sprintf("%s/bundle image listed in related images=%v", name, aListsBundleImage), func(t *testing.T) {
				build := sharedImageCopyBuilders(t)[name]
				// Go randomizes map iteration: repeat so that every order of the bundles is seen.
				for range 20 {
					first, second := sharedImageCatalogs(aListsBundleImage)
					for _, dc := range []*declcfg.DeclarativeConfig{first, second} {
						_, err := declcfg.ConvertToModel(*dc)
						require.NoError(t, err, "the test catalogs must be valid")
					}

					// Collect each catalog into its own map and merge them, as the collector does.
					copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: map[string]map[string]struct{}{}, BundlesByImage: map[string]map[string]string{}}
					handler := CatalogHandler{Log: log}
					related := map[string][]v2alpha1.RelatedImage{}
					for _, dc := range []*declcfg.DeclarativeConfig{first, second} {
						ri, err := handler.getRelatedImagesFromCatalog(dc, copyImageSchemaMap)
						require.NoError(t, err)
						maps.Copy(related, ri)
					}
					require.Equal(t, v2alpha1.RelatedImage{Image: sharedTestA, Type: v2alpha1.TypeOperatorBundle}, related["a.v1.0.0"][0],
						"collection still reports the bundle image of a as a bundle image")

					copies, err := build(related)
					require.NoError(t, err)
					sort.Sort(customsort.ByTypePriority(copies))

					// Skip decisions are deterministic only if the copy task dispatched right after
					// the failing image is not a bundle image: it waits for the failure to be recorded.
					failing := slices.IndexFunc(copies, func(c v2alpha1.CopyImageSchema) bool {
						return c.Origin == consts.DockerProtocol+sharedTestR
					})
					require.GreaterOrEqual(t, failing, 0)
					require.Less(t, failing+1, len(copies))
					require.NotEqual(t, v2alpha1.TypeOperatorBundle, copies[failing+1].Type,
						"the copy task dispatched after the failing image must not be a bundle image")

					sources := map[string]string{}
					for _, c := range copies {
						sources[strings.TrimPrefix(c.Origin, consts.DockerProtocol)] = c.Source
					}

					recorded := make(chan struct{})
					logger := &failureRecordingLogger{PluggableLoggerInterface: log, recorded: recorded}
					mirrorMock := &failingMirror{failSrc: sources[sharedTestR], recorded: recorded}
					worker := batch.New(batch.ChannelConcurrentWorker, logger, t.TempDir(), mirrorMock, uint(1), "test", nil)
					_, err = worker.Worker(context.Background(), v2alpha1.CollectorSchema{
						AllImages:           copies,
						CopyImageSchemaMap:  *copyImageSchemaMap,
						TotalOperatorImages: len(copies),
					}, sharedImageCopyOptions(mode))
					require.Error(t, err, "the failed copy is reported")

					assert.Contains(t, mirrorMock.copied, sources[sharedTestA], "the image needed by b is copied")
					assert.Contains(t, mirrorMock.copied, sources[sharedTestB], "the bundle image of b is copied")
					assert.Contains(t, mirrorMock.copied, sources[sharedTestO], "the other related image of a is copied")
					assert.NotContains(t, mirrorMock.copied, sources[sharedTestD], "the unshared bundle image of d is skipped")
					assert.ElementsMatch(t, []string{
						"Failed to copy operatorRelatedImage " + consts.DockerProtocol + sharedTestR,
						"Failed to copy operatorBundle " + consts.DockerProtocol + sharedTestD,
					}, logger.imageFailures(), "only the failed related image and the skipped bundle image are reported")
				}
			})
		}
	}
}
