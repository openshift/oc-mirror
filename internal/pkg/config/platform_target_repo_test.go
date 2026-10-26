package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestValidatePlatformTargetRepo(t *testing.T) {
	for _, tc := range []struct {
		name, repo string
		valid      bool
	}{
		{"empty", "", true},
		{"single component", "ocp-release", true},
		{"nested repository", "openshift4/ocp-release", true},
		{"allowed separators", "team/release_images.v4/release--repo", true},
		{"maximum length", strings.Repeat("a", 255), true},
		{"uppercase", "UPPER/release", false},
		{"tag", "repo:tag", false},
		{"digest", "repo@sha256:abc", false},
		{"transport", "docker://repo", false},
		{"leading slash", "/repo", false},
		{"trailing slash", "repo/", false},
		{"empty component", "a//b", false},
		{"parent component", "a/../b", false},
		{"space", "a b", false},
		{"registry port", "registry:5000/repo", false},
		{"too long", strings.Repeat("a", 256), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePlatformTargetRepo(tc.repo)
			if !tc.valid {
				require.ErrorContains(t, err, "platform.targetRepo")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPlatformTargetRepoConfig(t *testing.T) {
	for _, kind := range []string{v2alpha1.ImageSetConfigurationKind, v2alpha1.DeleteImageSetConfigurationKind} {
		t.Run(kind, func(t *testing.T) {
			stanza := "mirror"
			if kind == v2alpha1.DeleteImageSetConfigurationKind {
				stanza = "delete"
			}
			for _, tc := range []struct {
				repo  string
				valid bool
			}{
				{"", true},
				{"openshift4/ocp-release", true},
				{"repo:tag", false},
			} {
				t.Run(tc.repo, func(t *testing.T) {
					data := fmt.Sprintf("kind: %s\napiVersion: mirror.openshift.io/v2alpha1\n%s:\n  platform:\n    targetRepo: %q\n", kind, stanza, tc.repo)
					path := filepath.Join(t.TempDir(), "isc.yaml")
					require.NoError(t, os.WriteFile(path, []byte(data), 0600))
					cfg, err := ReadConfig(path, kind)
					if !tc.valid {
						require.ErrorContains(t, err, "platform.targetRepo")
						return
					}
					require.NoError(t, err)
					var platform v2alpha1.Platform
					switch c := cfg.(type) {
					case v2alpha1.ImageSetConfiguration:
						platform = c.Mirror.Platform
					case v2alpha1.DeleteImageSetConfiguration:
						platform = c.Delete.Platform
					default:
						t.Fatalf("unexpected configuration type %T", cfg)
					}
					require.Equal(t, tc.repo, platform.TargetRepo)
				})
			}
		})
	}
}
