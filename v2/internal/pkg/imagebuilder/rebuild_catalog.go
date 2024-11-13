package imagebuilder

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"text/template"

	"github.com/containers/buildah"
	"github.com/containers/buildah/define"
	"github.com/containers/buildah/imagebuildah"
	"github.com/containers/common/libimage/manifests"
	"github.com/containers/common/pkg/config"
	cp "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	filecopy "github.com/otiai10/copy"
)

const (
	dockerProtocol      = "docker://"
	MirrorToMirror      = "mirrorToMirror"
	rebuiltErrorLogFile = "rebuild-error.log"
)

type CatalogBuilder struct {
	CatalogBuilderInterface
	Logger   log.PluggableLoggerInterface
	CopyOpts mirror.CopyOptions
}

func NewCatalogBuilder(logger log.PluggableLoggerInterface, opts mirror.CopyOptions) CatalogBuilderInterface {

	return &CatalogBuilder{
		Logger:   logger,
		CopyOpts: opts,
	}
}

// RebuildCatalogs - uses buildah library that reads a containerfile and builds mult-arch manifestlist
// NB - due to the unshare (reexec) for buildah no unit tests have been implemented
// The final goal is to implement integration tests for this functionality
func (o CatalogBuilder) RebuildCatalog(ctx context.Context, catalogCopyRefs v2alpha1.CopyImageSchema, configPath string) (v2alpha1.CopyImageSchema, error) {

	containerTemplate := `
FROM {{ .Catalog }} AS builder
USER 0
RUN rm -rf /configs
COPY ./configs /configs
USER 1001
RUN rm -fr /tmp/cache/* && /bin/opm serve /configs --cache-only --cache-dir=/tmp/cache
`
	filteredDir := filepath.Dir(configPath)

	o.Logger.Info("🔂 rebuilding catalog (pulling catalog image) %s", catalogCopyRefs.Source)
	contents := bytes.NewBufferString("")
	tmpl, err := template.New("Containerfile").Parse(containerTemplate)
	if err != nil {
		return catalogCopyRefs, err
	}

	err = tmpl.Execute(contents, map[string]interface{}{
		"Catalog": catalogCopyRefs.Origin,
	})
	if err != nil {
		return catalogCopyRefs, err
	}

	// write the Containerfile content to a file
	containerfilePath := filepath.Join(filteredDir, "Containerfile")

	err = os.WriteFile(containerfilePath, contents.Bytes(), 0755)
	if err != nil {
		return catalogCopyRefs, err
	}

	var srcCache string

	destRef, err := image.ParseRef(catalogCopyRefs.Destination)
	if err != nil {
		return catalogCopyRefs, err
	}

	switch o.CopyOpts.Mode {
	case mirror.MirrorToDisk:
		srcCache = destRef.SetTag(filepath.Base(filteredDir)).ReferenceWithTransport
	case mirror.MirrorToMirror:
		srcCache = strings.Replace(catalogCopyRefs.Destination, o.CopyOpts.Destination, dockerProtocol+o.CopyOpts.LocalStorageFQDN, 1)
		destRef, err := image.ParseRef(srcCache)
		if err != nil {
			return catalogCopyRefs, err
		}
		srcCache = destRef.SetTag(filepath.Base(filteredDir)).ReferenceWithTransport
		o.CopyOpts.DestImage.TlsVerify = false
	case mirror.DiskToMirror:
		srcCache = catalogCopyRefs.Source
		o.CopyOpts.SrcImage.TlsVerify = false
	}

	updatedDest := strings.TrimPrefix(srcCache, dockerProtocol)

	srcSysCtx, err := o.CopyOpts.SrcImage.NewSystemContext()
	if err != nil {
		return catalogCopyRefs, err
	}

	file, err := createErrorLog(filteredDir)
	if err != nil {
		o.Logger.Error("error when creating the rebuild error log %s", err.Error())
	}
	defer file.Close()

	buildOptions, err := getStandardBuildOptions(updatedDest, srcSysCtx, file)
	if err != nil {
		return catalogCopyRefs, err
	}

	o.Logger.Trace("containerfile %s", contents.String())

	buildStoreOptions, err := storage.DefaultStoreOptions()
	if err != nil {
		return catalogCopyRefs, err
	}

	buildStore, err := storage.GetStore(buildStoreOptions)
	if err != nil {
		return catalogCopyRefs, err
	}
	defer buildStore.Shutdown(false)

	os.MkdirAll("configs", 0644)
	filecopy.Copy(configPath, "./configs")
	defer os.RemoveAll("configs")

	id, ref, err := imagebuildah.BuildDockerfiles(ctx, buildStore, buildOptions, []string{containerfilePath}...)
	if err == nil && buildOptions.Manifest != "" {
		o.Logger.Info("✅ successfully created catalog")
		o.Logger.Debug("  manifest list id : %s", id)
		o.Logger.Debug("  image reference  : %s", ref.String())
	}
	if err != nil {
		return catalogCopyRefs, err
	}

	retries := uint(3)

	destSysContext, err := o.CopyOpts.DestImage.NewSystemContext()
	if err != nil {
		return catalogCopyRefs, err
	}
	manifestPushOptions := manifests.PushOptions{
		Store:              buildStore,
		SystemContext:      destSysContext,
		ImageListSelection: cp.CopyAllImages,
		RemoveSignatures:   true,
		ManifestType:       buildah.OCIv1ImageManifest,
		MaxRetries:         &retries,
	}

	destImageRef, err := alltransports.ParseImageName(srcCache)
	if err != nil {
		return catalogCopyRefs, err
	}

	_, list, err := manifests.LoadFromImage(buildStore, id)
	if err != nil {
		return catalogCopyRefs, err
	}

	o.Logger.Debug("local cache destination (rebuilt-catalog) %s", srcCache)
	o.Logger.Debug("destination image reference %v", destImageRef)
	o.Logger.Debug("pushing manifest list to remote registry")
	// push the manifest list to local cache
	_, digest, err := list.Push(ctx, destImageRef, manifestPushOptions)
	if err != nil {
		return catalogCopyRefs, err
	}

	digestOnly := digest.Encoded()

	err = os.WriteFile(filepath.Join(filteredDir, "digest"), []byte(digestOnly), 0755)
	if err != nil {
		return catalogCopyRefs, err
	}

	_, err = buildStore.DeleteImage(id, true)
	if err != nil {
		return catalogCopyRefs, err
	}

	o.Logger.Info("✅ successfully pushed catalog manifest list")
	o.Logger.Debug("  digest           : %s", digest)

	if o.CopyOpts.Mode == MirrorToMirror {
		catalogCopyRefs = v2alpha1.CopyImageSchema{
			Origin:      catalogCopyRefs.Origin,
			Source:      srcCache,
			Destination: catalogCopyRefs.Destination,
			Type:        v2alpha1.TypeOperatorCatalog,
			RebuiltTag:  filepath.Base(filteredDir),
		}
	}

	o.Logger.Info("✅ completed rebuild catalog %s", catalogCopyRefs.Origin)
	return catalogCopyRefs, nil
}

func getStandardBuildOptions(destination string, sysCtx *types.SystemContext, rebuildErrLog *os.File) (define.BuildOptions, error) {
	// define platforms
	platforms := []struct{ OS, Arch, Variant string }{
		{"linux", "amd64", ""},
		{"linux", "arm64", ""},
		{"linux", "ppc64le", ""},
		{"linux", "s390x", ""},
	}
	conf, err := config.Default()
	if err != nil {
		return define.BuildOptions{}, err
	}
	capabilitiesForRoot, err := conf.Capabilities("root", nil, nil)
	if err != nil {
		return define.BuildOptions{}, err
	}

	jobs := 4

	buildOptions := define.BuildOptions{
		AddCapabilities:    capabilitiesForRoot,
		ConfigureNetwork:   buildah.NetworkDisabled,
		Err:                rebuildErrLog,
		Isolation:          buildah.IsolationOCIRootless,
		Jobs:               &jobs,
		LogFile:            "none",
		Manifest:           destination,
		MaxPullPushRetries: 2,
		NoCache:            true,
		Out:                io.Discard,
		OutputFormat:       buildah.OCIv1ImageManifest,
		Platforms:          platforms,
		PullPolicy:         define.PullAlways,
		Quiet:              true,
		ReportWriter:       io.Discard,
		Runtime:            "crun",
		SystemContext:      sysCtx,
	}
	return buildOptions, nil
}

func createErrorLog(filteredDir string) (*os.File, error) {
	rebuiltErrLogPath := filepath.Join(filteredDir, rebuiltErrorLogFile)

	file, err := os.Create(rebuiltErrLogPath)
	if err != nil {
		return nil, err
	}

	return file, err
}
