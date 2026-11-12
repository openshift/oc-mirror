package operator

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/customsort"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/skipped"
)

// OCPBUGS-132865: a bundle image that another bundle lists as a related image is copied as a
// related image, so that it is not skipped with its own bundle, but its own bundle still
// requires it. When it lacks a requested platform, the batch worker must fail its copy like
// any other bundle image instead of skipping it or narrowing its platforms as an operand.

var sharedPlatformRequested = []string{"linux/amd64", "linux/arm64"}

// controllerImagesByBundle returns the controller images of each test bundle's CSV without
// reading bundle images: sharedTestR runs the operators of a and d, b has no listed controller.
type controllerImagesByBundle map[string]map[string]struct{}

func (c controllerImagesByBundle) ControllerImages(_ context.Context, bundleImage, _ string, _ mirror.CopyOptions) (map[string]struct{}, error) {
	return c[bundleImage], nil
}

// platformMirror fails a copy that requests a platform its source has no instance for, with
// the copy library's error, before copying anything. It records the platforms of each copy.
type platformMirror struct {
	mu      sync.Mutex
	missing map[string][]string // source -> requested platforms the source lacks
	copied  map[string][]string // source -> platforms of its successful copy
}

func (m *platformMirror) Run(_ context.Context, src, _ string, _ mirror.Mode, opts *mirror.CopyOptions) error {
	for _, platform := range opts.InstancePlatforms {
		if slices.Contains(m.missing[src], platform) {
			return fmt.Errorf("preparing instances for copy: no instances found for platform %s", platform)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.copied[src] = slices.Clone(opts.InstancePlatforms)
	return nil
}

func (m *platformMirror) Check(context.Context, string, *mirror.CopyOptions, bool) (bool, error) {
	return true, nil
}

type sharedPlatformRun struct {
	copies   []v2alpha1.CopyImageSchema
	sources  map[string]string // image -> source of its copy task
	mirror   *platformMirror
	copied   v2alpha1.CollectorSchema
	failures []string
	err      error
}

// runSharedPlatformCopy collects sharedImageCatalogs, builds the copy batch of the workflow
// and runs the batch worker with the operand resolver, requesting sharedPlatformRequested.
func runSharedPlatformCopy(t *testing.T, workflow string, aListsBundleImage bool, missing map[string][]string) sharedPlatformRun {
	t.Helper()
	log := clog.New("error")

	first, second := sharedImageCatalogs(aListsBundleImage)
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: map[string]map[string]struct{}{}, BundlesByImage: map[string]map[string]string{}}
	handler := CatalogHandler{Log: log}
	related := map[string][]v2alpha1.RelatedImage{}
	for _, dc := range []*declcfg.DeclarativeConfig{first, second} {
		ri, err := handler.getRelatedImagesFromCatalog(dc, copyImageSchemaMap)
		require.NoError(t, err)
		maps.Copy(related, ri)
	}
	copies, err := sharedImageCopyBuilders(t)[workflow](related)
	require.NoError(t, err)
	sort.Sort(customsort.ByTypePriority(copies))

	run := sharedPlatformRun{
		copies:  copies,
		sources: map[string]string{},
		mirror:  &platformMirror{missing: map[string][]string{}, copied: map[string][]string{}},
	}
	for _, c := range copies {
		run.sources[strings.TrimPrefix(c.Origin, consts.DockerProtocol)] = c.Source
	}
	for img, platforms := range missing {
		require.Contains(t, run.sources, img)
		run.mirror.missing[run.sources[img]] = platforms
	}

	opts := sharedImageCopyOptions(workflow)
	opts.InstancePlatforms = sharedPlatformRequested
	opts.Global.WorkingDir = t.TempDir()
	resolver := &OperandResolver{Opts: &opts, handler: controllerImagesByBundle{
		sharedTestA: {sharedTestR: {}},
		sharedTestD: {sharedTestR: {}},
		sharedTestB: {},
	}}
	logger := &failureRecordingLogger{PluggableLoggerInterface: log, recorded: make(chan struct{})}
	worker := batch.New(batch.ChannelConcurrentWorker, logger, t.TempDir(), run.mirror, uint(1), "test", resolver)
	run.copied, run.err = worker.Worker(context.Background(), v2alpha1.CollectorSchema{
		AllImages:           copies,
		CopyImageSchemaMap:  *copyImageSchemaMap,
		TotalOperatorImages: len(copies),
	}, opts)
	run.failures = logger.imageFailures()
	return run
}

// reportsFailure reports whether the batch worker reported a failed copy of the image.
func (r sharedPlatformRun) reportsFailure(img string) bool {
	return slices.ContainsFunc(r.failures, func(line string) bool {
		return strings.HasSuffix(line, " "+consts.DockerProtocol+img)
	})
}

func platformStrings(filters []v2alpha1.InstancePlatformFilter) []string {
	platforms := make([]string, 0, len(filters))
	for _, f := range filters {
		platforms = append(platforms, f.String())
	}
	return platforms
}

var sharedPlatformWorkflows = []string{mirror.MirrorToMirror, mirror.MirrorToDisk, mirror.DiskToMirror}

func TestSharedBundleImageKeepsPlatformRequirements(t *testing.T) {
	missingCases := map[string][]string{
		"no requested platform available":  {"linux/amd64", "linux/arm64"},
		"one requested platform available": {"linux/arm64"},
	}

	for _, aListsBundleImage := range []bool{false, true} {
		for _, workflow := range sharedPlatformWorkflows {
			for caseName, missing := range missingCases {
				t.Run(fmt.Sprintf("%s/%s/bundle image listed in related images=%v", workflow, caseName, aListsBundleImage), func(t *testing.T) {
					// sharedTestA is the bundle image of a and a related image of b; sharedTestD is
					// the bundle image of d only (control); sharedTestO is an operand of a.
					run := runSharedPlatformCopy(t, workflow, aListsBundleImage, map[string][]string{
						sharedTestA: missing,
						sharedTestD: missing,
						sharedTestO: missing,
					})
					require.Error(t, run.err, "the batch reports failed copies")

					shared := copiesOf(run.copies, sharedTestA)
					require.Len(t, shared, 1)
					require.Equal(t, v2alpha1.TypeOperatorRelatedImage, shared[0].Type, "the shared bundle image is copied as a related image")
					require.Equal(t, v2alpha1.TypeOperatorBundle, copiesOf(run.copies, sharedTestD)[0].Type)

					for _, required := range []string{sharedTestA, sharedTestD} {
						origin := consts.DockerProtocol + required
						assert.NotContains(t, run.mirror.copied, run.sources[required], "bundle image %s is not copied with fewer platforms", required)
						assert.True(t, run.reportsFailure(required), "the failed copy of bundle image %s is reported, got %v", required, run.failures)
						assert.NotContains(t, run.copied.OperandPlatforms, origin, "no operand platform decision is recorded for bundle image %s", required)
						assert.NotContains(t, run.copied.PlatformFilters, origin, "the platforms of bundle image %s are not narrowed", required)
						assert.False(t, slices.ContainsFunc(run.copied.AllImages, func(c v2alpha1.CopyImageSchema) bool { return c.Origin == origin }),
							"bundle image %s is not reported as copied", required)
					}

					if workflow == mirror.DiskToMirror {
						// Disk to mirror never decides: it replays the decisions of mirror to disk.
						assert.Empty(t, run.copied.OperandPlatforms)
						return
					}

					// The operand of a keeps its optional platform handling.
					operand := consts.DockerProtocol + sharedTestO
					available := slices.DeleteFunc(slices.Clone(sharedPlatformRequested), func(p string) bool { return slices.Contains(missing, p) })
					assert.False(t, run.reportsFailure(sharedTestO), "the operand is not reported as failed")
					require.Contains(t, run.copied.OperandPlatforms, operand, "the operand decision is recorded")
					assert.ElementsMatch(t, available, platformStrings(run.copied.OperandPlatforms[operand]), "the operand decision keeps its available platforms")
					if len(available) == 0 {
						assert.NotContains(t, run.mirror.copied, run.sources[sharedTestO], "the operand without a requested platform is skipped")
					} else {
						assert.Equal(t, available, run.mirror.copied[run.sources[sharedTestO]], "the operand is copied with its available platforms")
					}

					if workflow == mirror.MirrorToDisk {
						// Only the operand has a decision that mirror to disk could persist for disk to
						// mirror to replay; the bundle images have none.
						dir := t.TempDir()
						require.NoError(t, skipped.WriteOperandPlatforms(dir, run.copied.OperandPlatforms))
						decisions, err := skipped.ReadOperandPlatforms(dir)
						require.NoError(t, err)
						assert.Equal(t, []string{operand}, slices.Collect(maps.Keys(decisions)))
					}
				})
			}
		}
	}
}

// A controller missing a requested platform still fails, and the bundle image of a it shares
// with b is still copied for b with every requested platform.
func TestSharedBundleImageCopiedWhenAControllerLacksAPlatform(t *testing.T) {
	for _, aListsBundleImage := range []bool{false, true} {
		for _, workflow := range sharedPlatformWorkflows {
			t.Run(fmt.Sprintf("%s/bundle image listed in related images=%v", workflow, aListsBundleImage), func(t *testing.T) {
				run := runSharedPlatformCopy(t, workflow, aListsBundleImage, map[string][]string{sharedTestR: {"linux/arm64"}})
				require.Error(t, run.err, "the controller missing a platform fails the batch")

				controller := consts.DockerProtocol + sharedTestR
				assert.True(t, run.reportsFailure(sharedTestR), "the failed controller copy is reported, got %v", run.failures)
				assert.NotContains(t, run.mirror.copied, run.sources[sharedTestR])
				assert.NotContains(t, run.copied.OperandPlatforms, controller)

				assert.Equal(t, sharedPlatformRequested, run.mirror.copied[run.sources[sharedTestA]], "the image needed by b is copied with every requested platform")
				assert.Equal(t, sharedPlatformRequested, run.mirror.copied[run.sources[sharedTestB]], "the bundle image of b is copied")
				assert.Equal(t, sharedPlatformRequested, run.mirror.copied[run.sources[sharedTestO]], "the operand of a is copied")
				assert.Empty(t, run.copied.OperandPlatforms)
			})
		}
	}
}
