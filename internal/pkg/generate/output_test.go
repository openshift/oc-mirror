package generate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
)

func sampleContent() Content {
	return Content{
		Platform: v2alpha1.Platform{
			Channels: []v2alpha1.ReleaseChannel{{Name: "stable-4.16", Type: v2alpha1.TypeOCP, MinVersion: "4.16.1", MaxVersion: "4.16.3"}},
			//nolint:staticcheck // SA1019: Architectures is deprecated but we maintain backward compatibility
			Architectures: []string{"amd64"},
		},
		Operators: []v2alpha1.Operator{{
			Catalog:       "reg.example.com/redhat/redhat-operator-index:v4.16",
			Full:          true,
			IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{{Name: "foo"}}},
		}},
		AdditionalImages: []v2alpha1.AdditionalImage{{Name: "reg.example.com/ubi9/ubi:latest"}},
	}
}

func TestWriteFilesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths, err := WriteFiles(dir, sampleContent(), OutputBoth)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, ISCFileName), filepath.Join(dir, DISCFileName)}, paths)

	isc, err := config.ReadConfig(paths[0], v2alpha1.ImageSetConfigurationKind)
	require.NoError(t, err)
	assert.Equal(t, "reg.example.com/ubi9/ubi:latest",
		isc.(v2alpha1.ImageSetConfiguration).Mirror.AdditionalImages[0].Name)

	disc, err := config.ReadConfig(paths[1], v2alpha1.DeleteImageSetConfigurationKind)
	require.NoError(t, err)
	assert.Equal(t, "stable-4.16",
		disc.(v2alpha1.DeleteImageSetConfiguration).Delete.Platform.Channels[0].Name)
}

// Registry error text in a warning must not escape the YAML comment and corrupt the config.
func TestHeaderFlattensMultiLineWarnings(t *testing.T) {
	hdr := header([]string{"catalog check failed: line one\nkind: Evil\r\nline three"})
	lines := strings.Split(strings.TrimSuffix(hdr, "\n"), "\n")
	require.Len(t, lines, 2)
	for _, l := range lines {
		assert.True(t, strings.HasPrefix(l, "#"), "line is not a comment: %q", l)
	}
	assert.Contains(t, lines[1], "line one kind: Evil  line three")
}

func TestWriteFilesRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, ISCFileName)
	require.NoError(t, os.WriteFile(existing, []byte("mine"), 0o600))

	_, err := WriteFiles(dir, sampleContent(), OutputISC)
	assert.ErrorContains(t, err, "already exists")
	got, _ := os.ReadFile(existing)
	assert.Equal(t, "mine", string(got), "user file must be untouched")
}

func TestWriteFilesBothLeavesNoPartialOutput(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, DISCFileName)
	require.NoError(t, os.WriteFile(existing, []byte("mine"), 0o600))

	_, err := WriteFiles(dir, sampleContent(), OutputBoth)
	assert.ErrorContains(t, err, "already exists")
	_, statErr := os.Stat(filepath.Join(dir, ISCFileName))
	assert.True(t, os.IsNotExist(statErr), "ISC file must not be created when DISC file exists")
	got, _ := os.ReadFile(existing)
	assert.Equal(t, "mine", string(got), "user file must be untouched")
}

func TestWriteFilesEmptyContentStillParses(t *testing.T) {
	dir := t.TempDir()
	paths, err := WriteFiles(dir, Content{}, OutputBoth)
	require.NoError(t, err)
	_, err = config.ReadConfig(paths[0], v2alpha1.ImageSetConfigurationKind)
	assert.NoError(t, err)
	_, err = config.ReadConfig(paths[1], v2alpha1.DeleteImageSetConfigurationKind)
	assert.NoError(t, err)
}

func TestWriteFilesExistsWrapsErrExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ISCFileName), []byte("x"), 0o600))
	_, err := WriteFiles(dir, sampleContent(), OutputISC)
	assert.True(t, errors.Is(err, os.ErrExist))
}
