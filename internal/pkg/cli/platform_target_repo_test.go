package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/docker/reference"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func platformWithTargetRepo(field, repo string) v2alpha1.Platform {
	if field == "targetImageRepo" {
		return v2alpha1.Platform{TargetImageRepo: repo}
	}
	return v2alpha1.Platform{TargetComponentRepo: repo}
}

func TestPlatformTargetRepoNestingLimit(t *testing.T) {
	for _, field := range []string{"targetImageRepo", "targetComponentRepo"} {
		t.Run(field, func(t *testing.T) {
			for _, tc := range []struct {
				name, repo, destination string
				limit                   int
				disk, invalid           bool
			}{
				{"unlimited", "a/b/c", "docker://mirror.example.com/team", 0, false, false},
				{"fits", "a/b", "docker://mirror.example.com", 2, false, false},
				{"too deep", "a/b/c", "docker://mirror.example.com", 2, false, true},
				{"namespace counts", "a/b", "docker://mirror.example.com/team", 2, false, true},
				{"single path", "release", "docker://mirror.example.com", 1, false, false},
				{"default can flatten", "", "docker://mirror.example.com/team", 2, false, false},
				{"disk fits", "a/b", "file:///tmp/archive", 2, true, false},
				{"disk too deep", "a/b/c", "file:///tmp/archive", 2, true, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ex := ExecutorSchema{Opts: &mirror.CopyOptions{
						Destination: tc.destination,
						Mode:        mirror.MirrorToMirror,
						Global:      &mirror.GlobalOptions{MaxNestedPaths: tc.limit},
					}}
					if tc.disk {
						ex.Opts.Mode = mirror.MirrorToDisk
					}
					ex.Config.Mirror.Platform = platformWithTargetRepo(field, tc.repo)
					err := ex.validatePlatformDestination()
					if tc.invalid {
						require.ErrorContains(t, err, "exceeds --max-nested-paths")
						return
					}
					require.NoError(t, err)
				})
			}
		})
	}
}

func TestPlatformTargetRepoDestinationValidation(t *testing.T) {
	const registry = "mirror.example.com"
	for _, field := range []string{"targetImageRepo", "targetComponentRepo"} {
		t.Run(field, func(t *testing.T) {
			for _, tc := range []struct {
				name, destination string
				repoLength, limit int
				disk, invalid     bool
			}{
				{"complete name fits", "docker://" + registry, reference.NameTotalLengthMax - len(registry) - 1, 0, false, false},
				{"registry makes name too long", "docker://" + registry, reference.NameTotalLengthMax, 0, false, true},
				{"too long with nesting limit", "docker://" + registry, reference.NameTotalLengthMax, 1, false, true},
				{"namespace fits", "docker://" + registry + "/team", reference.NameTotalLengthMax - len(registry+"/team") - 1, 0, false, false},
				{"namespace makes name too long", "docker://" + registry + "/team", reference.NameTotalLengthMax - len(registry) - 1, 0, false, true},
				{"invalid namespace", "docker://" + registry + "/UPPER", 1, 0, false, true},
				{"cache name fits", "file:///tmp/archive", reference.NameTotalLengthMax - len("localhost:55000/"), 0, true, false},
				{"cache port makes name too long", "file:///tmp/archive", reference.NameTotalLengthMax - len("localhost/"), 0, true, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ex := ExecutorSchema{Opts: &mirror.CopyOptions{
						Destination: tc.destination, Mode: mirror.MirrorToMirror,
						Global: &mirror.GlobalOptions{MaxNestedPaths: tc.limit, Port: 55000},
					}}
					if tc.disk {
						ex.Opts.Mode = mirror.MirrorToDisk
					}
					ex.Config.Mirror.Platform = platformWithTargetRepo(field, strings.Repeat("a", tc.repoLength))
					err := ex.validatePlatformDestination()
					if tc.invalid {
						require.ErrorContains(t, err, "platform."+field)
						require.ErrorContains(t, err, "is invalid")
						return
					}
					require.NoError(t, err)
				})
			}
		})
	}
}

type targetRepoResources struct {
	MockClusterResources
	graph, payload string
}

func (r *targetRepoResources) UpdateServiceGenerator(graph, payload string) error {
	r.graph, r.payload = graph, payload
	return nil
}

func TestPlatformTargetRepoUpdateServiceNesting(t *testing.T) {
	resources := &targetRepoResources{}
	ex := ExecutorSchema{
		ClusterResources: resources,
	}
	ex.Config.Mirror.Platform.Graph = true
	images := []v2alpha1.CopyImageSchema{
		{Type: v2alpha1.TypeOCPReleaseContent, Destination: "docker://mirror.example.com/components:v1"},
		{Type: v2alpha1.TypeCincinnatiGraph, Destination: "docker://mirror.example.com/team/openshift-graph-image:latest"},
		{Type: v2alpha1.TypeOCPRelease, Destination: "docker://mirror.example.com/team/ocp-release:v1"},
	}
	require.NoError(t, ex.generateUpdateServiceResource(images))
	require.Equal(t, "docker://mirror.example.com/team/openshift-graph-image:latest", resources.graph)
	require.Equal(t, "docker://mirror.example.com/team/ocp-release:v1", resources.payload)
}

func TestUpdateServiceRequiresMirroredImages(t *testing.T) {
	graph := v2alpha1.CopyImageSchema{Type: v2alpha1.TypeCincinnatiGraph, Destination: "docker://mirror.example.com/openshift/graph-image:latest"}
	payload := v2alpha1.CopyImageSchema{Type: v2alpha1.TypeOCPRelease, Destination: "docker://mirror.example.com/releases:v1"}
	for _, tc := range []struct {
		name    string
		enabled bool
		images  []v2alpha1.CopyImageSchema
	}{
		{"graph disabled", false, nil},
		{"no images", true, nil},
		{"no payload", true, []v2alpha1.CopyImageSchema{graph}},
		{"no graph", true, []v2alpha1.CopyImageSchema{payload}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resources := &targetRepoResources{}
			ex := ExecutorSchema{ClusterResources: resources}
			ex.Config.Mirror.Platform.Graph = tc.enabled
			err := ex.generateUpdateServiceResource(tc.images)
			if tc.enabled {
				require.ErrorContains(t, err, "mirrored release and graph image destinations are required")
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, resources.graph)
			require.Empty(t, resources.payload)
		})
	}
}

type failedReleaseBatch struct{ err error }

func (b failedReleaseBatch) Worker(context.Context, v2alpha1.CollectorSchema, mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	return v2alpha1.CollectorSchema{}, b.err
}

func TestUpdateServicePreservesBatchError(t *testing.T) {
	for _, mode := range []string{mirror.MirrorToMirror, mirror.DiskToMirror} {
		t.Run(mode, func(t *testing.T) {
			batchErr := &CollectionError{ReleaseErr: errors.New("release copy failed")}
			collector := &Collector{}
			resources := &targetRepoResources{}
			ex := ExecutorSchema{
				Log: clog.New("error"),
				Opts: &mirror.CopyOptions{
					Mode:             mode,
					RemoveSignatures: true,
					Global:           &mirror.GlobalOptions{WorkingDir: t.TempDir()},
				},
				Release:          collector,
				Operator:         collector,
				AdditionalImages: collector,
				HelmCollector:    collector,
				Batch:            failedReleaseBatch{err: batchErr},
				MirrorUnArchiver: MockMirrorUnArchiver{},
				ClusterResources: resources,
			}
			ex.Config.Mirror.Platform.Graph = true
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			var err error
			if mode == mirror.MirrorToMirror {
				err = ex.RunMirrorToMirror(cmd, nil)
			} else {
				err = ex.RunDiskToMirror(cmd, nil)
			}
			require.Same(t, batchErr, err, "resource generation must preserve the mirroring error and its exit code")
			require.Empty(t, resources.graph)
			require.Empty(t, resources.payload)
		})
	}
}

func TestPlatformTargetRepoSingleLabelRegistry(t *testing.T) {
	for _, field := range []string{"targetImageRepo", "targetComponentRepo"} {
		t.Run(field, func(t *testing.T) {
			for _, tc := range []struct {
				destination string
				limit       int
			}{
				{"docker://registry", 1},
				{"docker://registry/team", 2},
			} {
				t.Run(tc.destination, func(t *testing.T) {
					ex := ExecutorSchema{Opts: &mirror.CopyOptions{
						Destination: tc.destination, Mode: mirror.MirrorToMirror,
						Global: &mirror.GlobalOptions{MaxNestedPaths: tc.limit},
					}}
					ex.Config.Mirror.Platform = platformWithTargetRepo(field, "releases")
					require.NoError(t, ex.validatePlatformDestination())
					ex.Config.Mirror.Platform = platformWithTargetRepo(field, "team/releases")
					require.ErrorContains(t, ex.validatePlatformDestination(), "exceeds --max-nested-paths")
				})
			}
		})
	}
}

func TestPlatformTargetRepoGraphRepositoryConflict(t *testing.T) {
	for _, field := range []string{"targetImageRepo", "targetComponentRepo"} {
		t.Run(field, func(t *testing.T) {
			for _, tc := range []struct {
				name, repo string
				mode       string
				limit      int
				conflict   bool
			}{
				{"direct", "openshift/graph-image", mirror.MirrorToMirror, 0, true},
				{"archive", "openshift/graph-image", mirror.MirrorToDisk, 0, true},
				{"restore", "openshift/graph-image", mirror.DiskToMirror, 0, true},
				{"flattened graph", "openshift-graph-image", mirror.MirrorToMirror, 1, true},
				{"unflattened cache", "openshift-graph-image", mirror.MirrorToDisk, 1, false},
				{"separate repositories", "releases", mirror.MirrorToMirror, 1, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ex := ExecutorSchema{Opts: &mirror.CopyOptions{
						Destination: "docker://mirror.example.com", Mode: tc.mode,
						Global: &mirror.GlobalOptions{Port: 55000, MaxNestedPaths: tc.limit},
					}}
					ex.Config.Mirror.Platform = platformWithTargetRepo(field, tc.repo)
					ex.Config.Mirror.Platform.Graph = true
					if tc.conflict {
						require.ErrorContains(t, ex.validatePlatformDestination(), "conflicts with the Cincinnati graph repository")
					} else {
						require.NoError(t, ex.validatePlatformDestination())
					}
					ex.Config.Mirror.Platform.Graph = false
					require.NoError(t, ex.validatePlatformDestination())
				})
			}
		})
	}
}

func TestPlatformTargetRepoImageDestinationConflicts(t *testing.T) {
	const target = "docker://mirror.example.com/releases:v1-component"
	const payload = "source.example.com/release:v1-component"
	digest := "@sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, first, second string
		conflict            bool
	}{
		{"payload and component", payload, "source.example.com/components" + digest, true},
		{"different payloads", payload, "other.example.com/release:v1-component", true},
		{"duplicate origin", payload, payload, false},
		{"transport prefix", payload, "docker://" + payload, false},
		{"same digest", "source.example.com/components" + digest, "other.example.com/components" + digest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := ExecutorSchema{}
			ex.Config.Mirror.Platform.TargetImageRepo = "releases"
			ex.Config.Mirror.Platform.TargetComponentRepo = "releases"
			images := []v2alpha1.CopyImageSchema{
				{Origin: tc.first, Destination: target, Type: v2alpha1.TypeOCPRelease},
				{Origin: tc.second, Destination: target, Type: v2alpha1.TypeOCPReleaseContent},
			}
			err := ex.validatePlatformImageDestinations(images)
			if tc.conflict {
				require.ErrorContains(t, err, "conflicting destination")
				require.ErrorContains(t, err, tc.first)
				require.ErrorContains(t, err, tc.second)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPlatformTargetRepositoriesValidatedIndependently(t *testing.T) {
	for _, tc := range []struct {
		name, imageRepo, componentRepo, errorField string
		limit                                      int
		graph                                      bool
	}{
		{"separate repositories", "releases", "components", "", 2, true},
		{"shared repository", "releases", "releases", "", 2, true},
		{"component too deep", "releases", "team/components", "targetComponentRepo", 2, false},
		{"payload too deep", "team/releases", "components", "targetImageRepo", 2, false},
		{"component too long", "releases", strings.Repeat("a", reference.NameTotalLengthMax), "targetComponentRepo", 0, false},
		{"component graph conflict", "releases", "openshift/graph-image", "targetComponentRepo", 0, true},
		{"flattened component graph conflict", "releases", "openshift-graph-image", "targetComponentRepo", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := ExecutorSchema{Opts: &mirror.CopyOptions{
				Destination: "docker://mirror.example.com/team", Mode: mirror.MirrorToMirror,
				Global: &mirror.GlobalOptions{MaxNestedPaths: tc.limit},
			}}
			ex.Config.Mirror.Platform = v2alpha1.Platform{
				TargetImageRepo: tc.imageRepo, TargetComponentRepo: tc.componentRepo, Graph: tc.graph,
			}
			err := ex.validatePlatformDestination()
			if tc.errorField != "" {
				require.ErrorContains(t, err, "platform."+tc.errorField)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPlatformComponentOnlyDestinationConflict(t *testing.T) {
	ex := ExecutorSchema{}
	ex.Config.Mirror.Platform.TargetComponentRepo = "openshift/release-images"
	const destination = "docker://mirror.example.com/openshift/release-images:v1-component"
	images := []v2alpha1.CopyImageSchema{
		{Origin: "quay.io/release/payload:v1-component", Destination: destination, Type: v2alpha1.TypeOCPRelease},
		{Origin: "quay.io/release/components@sha256:" + strings.Repeat("a", 64), Destination: destination, Type: v2alpha1.TypeOCPReleaseContent},
	}
	require.ErrorContains(t, ex.validatePlatformImageDestinations(images), "conflicting destination")
}

func TestPlatformImageDestinationConflictConfigSources(t *testing.T) {
	const destination = "docker://mirror.example.com/releases:v1"
	for _, tc := range []struct {
		name                  string
		platform              v2alpha1.Platform
		firstType, secondType v2alpha1.ImageType
		expected, unexpected  []string
	}{
		{
			name:      "payload and additional image",
			platform:  v2alpha1.Platform{TargetImageRepo: "releases"},
			firstType: v2alpha1.TypeOCPRelease, secondType: v2alpha1.TypeGeneric,
			expected:   []string{`platform.targetImageRepo "releases"`, "additionalImages"},
			unexpected: []string{"platform.targetComponentRepo"},
		},
		{
			name:      "additional image and payload",
			platform:  v2alpha1.Platform{TargetImageRepo: "releases"},
			firstType: v2alpha1.TypeGeneric, secondType: v2alpha1.TypeOCPRelease,
			expected:   []string{"additionalImages", `platform.targetImageRepo "releases"`},
			unexpected: []string{"platform.targetComponentRepo"},
		},
		{
			name:      "component override and default payload",
			platform:  v2alpha1.Platform{TargetComponentRepo: "openshift/release-images"},
			firstType: v2alpha1.TypeOCPReleaseContent, secondType: v2alpha1.TypeOCPRelease,
			expected:   []string{`platform.targetComponentRepo "openshift/release-images"`, "platform release payload (default repository)"},
			unexpected: []string{"platform.targetImageRepo", "additionalImages"},
		},
		{
			name:      "payload override and default component",
			platform:  v2alpha1.Platform{TargetImageRepo: "openshift/release"},
			firstType: v2alpha1.TypeOCPRelease, secondType: v2alpha1.TypeOCPReleaseContent,
			expected:   []string{`platform.targetImageRepo "openshift/release"`, "platform release component (default repository)"},
			unexpected: []string{"platform.targetComponentRepo", "additionalImages"},
		},
		{
			name:      "unrelated additional images with a platform override configured",
			platform:  v2alpha1.Platform{TargetImageRepo: "payloads", TargetComponentRepo: "components"},
			firstType: v2alpha1.TypeGeneric, secondType: v2alpha1.TypeGeneric,
			expected:   []string{"additionalImages"},
			unexpected: []string{"platform.targetImageRepo", "platform.targetComponentRepo"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := ExecutorSchema{}
			ex.Config.Mirror.Platform = tc.platform
			images := []v2alpha1.CopyImageSchema{
				{Origin: "docker://source.example.com/first:v1", Destination: destination, Type: tc.firstType},
				{Origin: "source.example.com/second:v1", Destination: destination, Type: tc.secondType},
			}
			originalImages := append([]v2alpha1.CopyImageSchema(nil), images...)
			err := ex.validatePlatformImageDestinations(images)
			require.ErrorContains(t, err, "conflicting destination")
			require.ErrorContains(t, err, destination)
			require.ErrorContains(t, err, "source.example.com/first:v1")
			require.ErrorContains(t, err, "source.example.com/second:v1")
			for _, expected := range tc.expected {
				require.Contains(t, err.Error(), expected, "identify the actual configuration source")
			}
			for _, unexpected := range tc.unexpected {
				require.NotContains(t, err.Error(), unexpected, "do not blame unrelated configuration fields")
			}
			require.Equal(t, originalImages, images, "validation must not modify the collected images")
		})
	}
}

func TestImageDestinationConfigSource(t *testing.T) {
	ex := ExecutorSchema{}
	ex.Config.Mirror.Platform = v2alpha1.Platform{TargetImageRepo: "payloads", TargetComponentRepo: "components"}
	for _, tc := range []struct {
		imageType v2alpha1.ImageType
		want      string
	}{
		{v2alpha1.TypeOCPRelease, `platform.targetImageRepo "payloads"`},
		{v2alpha1.TypeOCPReleaseContent, `platform.targetComponentRepo "components"`},
		{v2alpha1.TypeKubeVirtContainer, `platform.targetComponentRepo "components"`},
		{v2alpha1.TypeCincinnatiGraph, "platform.graph"},
		{v2alpha1.TypeGeneric, "additionalImages"},
		{v2alpha1.TypeOperatorCatalog, "operators"},
		{v2alpha1.TypeOperatorBundle, "operators"},
		{v2alpha1.TypeOperatorRelatedImage, "operators"},
		{v2alpha1.TypeHelmImage, "helm"},
		{v2alpha1.TypeInvalid, "unknown image type"},
		{v2alpha1.ImageType(-1), "unknown image type -1"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			require.Equal(t, tc.want, ex.imageDestinationConfigSource(tc.imageType))
		})
	}
}

func TestSameImageDigestPreservesIdentityRules(t *testing.T) {
	const origin = "source.example.com/release:v1"
	digest := "@sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, first, second string
		same                bool
	}{
		{"same digest", origin + digest, "other.example.com/components" + digest, true},
		{"different digests", origin + digest, origin + "@sha256:" + strings.Repeat("b", 64), false},
		{"tag only", origin, origin, false},
		{"tag and pinned reference remain distinct", origin, origin + digest, false},
		{"pinned reference and tag remain distinct", origin + digest, origin, false},
		{"invalid first origin", "", origin + digest, false},
		{"invalid second origin", origin + digest, "invalid@sha256:abc", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.same, sameImageDigest(tc.first, tc.second))
		})
	}
}
