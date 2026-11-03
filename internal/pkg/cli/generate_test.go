package cli

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/generate"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// TestGenerateValidate checks accepted registry arguments and invalid flag combinations.
func TestGenerateValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    generateOptions
		args    []string
		host    string
		wantErr string
	}{
		{name: "ok", opts: generateOptions{outputType: "both"}, args: []string{"docker://reg.example.com:5000"}, host: "reg.example.com:5000"},
		{name: "missing arg", opts: generateOptions{outputType: "both"}, wantErr: "registry argument is required"},
		{name: "no protocol", opts: generateOptions{outputType: "both"}, args: []string{"reg.example.com"}, wantErr: "docker://"},
		{name: "path in arg", opts: generateOptions{outputType: "both"}, args: []string{"docker://reg.example.com/team"}, wantErr: "--prefix"},
		{name: "empty host", opts: generateOptions{outputType: "both"}, args: []string{"docker://"}, wantErr: "empty host"},
		{name: "bad type", opts: generateOptions{outputType: "xml"}, args: []string{"docker://r"}, wantErr: "--output-type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := tt.opts.validate(tt.args)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.host, host)
		})
	}
}

// TestGenerateRun checks the end-to-end flow against an in-process registry:
// the ISC lists the pushed image, and a second run never overwrites it.
func TestGenerateRun(t *testing.T) {
	t.Setenv("REGISTRY_AUTH_FILE", filepath.Join(t.TempDir(), "no-auth.json"))

	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	img, err := random.Image(64, 1)
	require.NoError(t, err)
	ref, err := name.ParseReference(host+"/ubi9/ubi:latest", name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))

	global := &mirror.GlobalOptions{}
	sharedFlags, shared := mirror.SharedImageFlags()
	depFlags, dep := mirror.DeprecatedTLSVerifyFlags()
	srcFlags, src := mirror.ImageSrcFlags(global, shared, dep, "src-", "screds")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.AddFlagSet(&sharedFlags)
	fs.AddFlagSet(&depFlags)
	fs.AddFlagSet(&srcFlags)
	require.NoError(t, fs.Parse([]string{"--src-tls-verify=false"}))

	outDir := t.TempDir()
	o := &generateOptions{
		outputType: string(generate.OutputISC),
		outputDir:  outDir,
		globalOpts: &mirror.CopyOptions{Global: global, DeprecatedTLSVerify: dep, SrcImage: src},
	}
	log := clog.New("trace")

	require.NoError(t, o.run(context.Background(), log, host))
	data, err := os.ReadFile(filepath.Join(outDir, generate.ISCFileName))
	require.NoError(t, err)
	assert.Contains(t, string(data), "additionalImages")
	assert.Contains(t, string(data), host+"/ubi9/ubi:latest")

	err = o.run(context.Background(), log, host)
	assert.ErrorContains(t, err, "exist")
	after, err := os.ReadFile(filepath.Join(outDir, generate.ISCFileName))
	require.NoError(t, err)
	assert.Equal(t, string(data), string(after))
}

// TestGenerateCommandArgs checks cobra rejects zero or several registry arguments before RunE.
func TestGenerateCommandArgs(t *testing.T) {
	for _, args := range [][]string{{}, {"docker://a", "docker://b"}} {
		cmd := NewGenerateCommand(clog.New("trace"), &mirror.CopyOptions{})
		cmd.SetArgs(args)
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		assert.ErrorContains(t, cmd.Execute(), "accepts 1 arg", "args %v", args)
	}
}

// TestGenerateErrorsHideIdentifiers checks the CLI boundary does not expose input values.
func TestGenerateErrorsHideIdentifiers(t *testing.T) {
	o := &generateOptions{outputType: "private-customer-value"}
	_, err := o.validate([]string{"docker://host"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-customer-value")
	global := &mirror.GlobalOptions{}
	_, shared := mirror.SharedImageFlags()
	_, dep := mirror.DeprecatedTLSVerifyFlags()
	_, src := mirror.ImageSrcFlags(global, shared, dep, "src-", "screds")
	o.globalOpts = &mirror.CopyOptions{Global: global, DeprecatedTLSVerify: dep, SrcImage: src}
	err = o.run(context.Background(), clog.New("trace"), "private-customer/invalid")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-customer")
}

// generateRecordingLogger captures CLI diagnostics while retaining the logger interface.
type generateRecordingLogger struct {
	clog.PluggableLoggerInterface
	messages []string
}

// Info records a formatted progress message.
func (l *generateRecordingLogger) Info(msg string, args ...interface{}) {
	l.messages = append(l.messages, fmt.Sprintf(msg, args...))
}

// Warn records a formatted warning count.
func (l *generateRecordingLogger) Warn(msg string, args ...interface{}) {
	l.messages = append(l.messages, fmt.Sprintf(msg, args...))
}

// TestGenerateOutputDiagnostics checks successful and failed writes hide output paths.
func TestGenerateOutputDiagnostics(t *testing.T) {
	t.Setenv("REGISTRY_AUTH_FILE", filepath.Join(t.TempDir(), "no-auth.json"))
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	global := &mirror.GlobalOptions{}
	_, shared := mirror.SharedImageFlags()
	_, dep := mirror.DeprecatedTLSVerifyFlags()
	flags, src := mirror.ImageSrcFlags(global, shared, dep, "src-", "screds")
	require.NoError(t, flags.Set("src-tls-verify", "false"))
	o := &generateOptions{outputType: "both", outputDir: t.TempDir(), globalOpts: &mirror.CopyOptions{Global: global, DeprecatedTLSVerify: dep, SrcImage: src}}
	log := &generateRecordingLogger{PluggableLoggerInterface: clog.New("trace")}
	host := strings.TrimPrefix(srv.URL, "http://")
	require.NoError(t, o.run(context.Background(), log, host))
	assert.NotContains(t, strings.Join(log.messages, "\n"), o.outputDir)
	assert.NotContains(t, strings.Join(log.messages, "\n"), host)
	err := o.run(context.Background(), log, host)
	require.ErrorIs(t, err, os.ErrExist)
	assert.NotContains(t, err.Error(), o.outputDir)
	o.outputDir = filepath.Join(o.outputDir, "missing-private-directory")
	err = o.run(context.Background(), log, host)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "missing-private-directory")
}

// TestGenerateCatalogLoggerSuppressesDetails checks all catalog log levels discard messages.
func TestGenerateCatalogLoggerSuppressesDetails(t *testing.T) {
	// A nil delegate would panic if any diagnostic were forwarded.
	log := generateCatalogLogger{}
	require.NotPanics(t, func() {
		log.Error("private %s", "value")
		log.Warn("private %s", "value")
		log.Info("private %s", "value")
		log.Debug("private %s", "value")
		log.Trace("private %s", "value")
	})
}
