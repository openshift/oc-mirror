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
			for _, field := range []string{"targetImageRepo", "targetComponentRepo"} {
				err := validatePlatformTargetRepo(field, tc.repo)
				if !tc.valid {
					require.ErrorContains(t, err, "platform."+field)
					continue
				}
				require.NoError(t, err)
			}
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
				imageRepo, componentRepo string
				errorFields              []string
			}{
				{"", "", nil},
				{"openshift4/ocp-release", "", nil},
				{"", "openshift4/components", nil},
				{"openshift4/ocp-release", "openshift4/components", nil},
				{"repo:tag", "", []string{"targetImageRepo"}},
				{"", "repo:tag", []string{"targetComponentRepo"}},
				{"repo:tag", "UPPER", []string{"targetImageRepo", "targetComponentRepo"}},
				{"openshift4/ocp-release", "repo:tag", []string{"targetComponentRepo"}},
				{"repo:tag", "openshift4/components", []string{"targetImageRepo"}},
			} {
				t.Run(tc.imageRepo+"/"+tc.componentRepo, func(t *testing.T) {
					data := fmt.Sprintf("kind: %s\napiVersion: mirror.openshift.io/v2alpha1\n%s:\n  platform:\n    targetImageRepo: %q\n    targetComponentRepo: %q\n", kind, stanza, tc.imageRepo, tc.componentRepo)
					path := filepath.Join(t.TempDir(), "isc.yaml")
					require.NoError(t, os.WriteFile(path, []byte(data), 0600))
					cfg, err := ReadConfig(path, kind)
					if len(tc.errorFields) > 0 {
						for _, field := range tc.errorFields {
							require.ErrorContains(t, err, "platform."+field)
						}
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
					require.Equal(t, tc.imageRepo, platform.TargetImageRepo)
					require.Equal(t, tc.componentRepo, platform.TargetComponentRepo)
				})
			}
		})
	}
}

func TestPlatformTargetRepoRejectsOldField(t *testing.T) {
	for _, kind := range []string{v2alpha1.ImageSetConfigurationKind, v2alpha1.DeleteImageSetConfigurationKind} {
		t.Run(kind, func(t *testing.T) {
			stanza := "mirror"
			if kind == v2alpha1.DeleteImageSetConfigurationKind {
				stanza = "delete"
			}
			data := fmt.Sprintf("kind: %s\napiVersion: mirror.openshift.io/v2alpha1\n%s:\n  platform:\n    targetRepo: releases\n", kind, stanza)
			path := filepath.Join(t.TempDir(), "isc.yaml")
			require.NoError(t, os.WriteFile(path, []byte(data), 0600))
			_, err := ReadConfig(path, kind)
			require.ErrorContains(t, err, "unknown field")
			require.ErrorContains(t, err, "targetRepo")
		})
	}
}
