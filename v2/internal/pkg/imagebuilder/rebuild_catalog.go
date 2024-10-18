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
	filecopy "github.com/otiai10/copy"
)

const (
	dockerProtocol = "docker://"
	MirrorToMirror = "mirrorToMirror"
)

// RebuildCatalogs - uses buildah library that reads a containerfile and builds mult-arch manifestlist
// NB - due to the unshare (reexec) for buildah no unit tests have been implemented
// The final goal is to implement integration tests for this functionality
func (o *ImageBuilder) RebuildCatalogs(ctx context.Context, catalogSchema v2alpha1.CollectorSchema) ([]v2alpha1.CopyImageSchema, []v2alpha1.Image, error) {

	// set variables
	catalogs := catalogSchema.CatalogToFBCMap
	excludeCatalogs := []v2alpha1.Image{}
	result := []v2alpha1.CopyImageSchema{}
	containerTemplate := `
FROM {{ .Catalog }} AS builder
USER 0
RUN rm -rf /configs
COPY ./configs /configs
USER 1001
RUN rm -fr /tmp/cache/*
RUN /bin/opm serve /configs --cache-only --cache-dir=/tmp/cache

FROM {{ .Catalog }} 
USER 0
RUN rm -rf /configs
COPY ./configs /configs
USER 1001
RUN rm -fr /tmp/cache/*
COPY --from=builder /tmp/cache /tmp/cache
`

	for _, v := range catalogs {
		o.Logger.Info("🔂 rebuilding catalog (pulling catalog image) %s", v.OperatorFilter.Catalog)
		contents := bytes.NewBufferString("")
		tmpl, err := template.New("Containerfile").Parse(containerTemplate)
		if err != nil {
			return result, excludeCatalogs, err
		}
		err = tmpl.Execute(contents, map[string]interface{}{
			"Catalog": v.OperatorFilter.Catalog,
		})
		if err != nil {
			return result, excludeCatalogs, err
		}

		// write the Containerfile content to a file
		containerfilePath := filepath.Join("tmp", "Containerfile")
		os.MkdirAll("tmp", 0755)
		defer os.RemoveAll("tmp")

		err = os.WriteFile(containerfilePath, contents.Bytes(), 0755)
		if err != nil {
			return result, excludeCatalogs, err
		}

		var localDest, remoteDest string

		imgSpec, err := image.ParseRef(v.OperatorFilter.Catalog)
		if err != nil {
			return result, excludeCatalogs, err
		}

		switch {
		case len(v.OperatorFilter.TargetCatalog) > 0 && len(v.OperatorFilter.TargetTag) > 0:
			localDest = dockerProtocol + strings.Join([]string{o.LocalFQDN, v.OperatorFilter.TargetCatalog}, "/") + ":" + v.OperatorFilter.TargetTag
			remoteDest = strings.Join([]string{o.Destination, v.OperatorFilter.TargetCatalog}, "/") + ":" + v.OperatorFilter.TargetTag
		case len(v.OperatorFilter.TargetCatalog) > 0 && len(v.OperatorFilter.TargetTag) == 0:
			localDest = dockerProtocol + strings.Join([]string{o.LocalFQDN, v.OperatorFilter.TargetCatalog}, "/") + ":" + imgSpec.Tag
			remoteDest = strings.Join([]string{o.Destination, v.OperatorFilter.TargetCatalog}, "/") + ":" + imgSpec.Tag
		case len(v.OperatorFilter.TargetCatalog) == 0 && len(v.OperatorFilter.TargetTag) > 0:
			localDest = dockerProtocol + strings.Join([]string{o.LocalFQDN, imgSpec.PathComponent}, "/") + ":" + v.OperatorFilter.TargetTag
			remoteDest = strings.Join([]string{o.Destination, imgSpec.PathComponent}, "/") + ":" + v.OperatorFilter.TargetTag
		case len(v.OperatorFilter.TargetCatalog) == 0 && len(v.OperatorFilter.TargetTag) == 0:
			localDest = dockerProtocol + strings.Join([]string{o.LocalFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
			remoteDest = strings.Join([]string{o.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
		}

		// this is safe as we know that there is a docker:// prefix
		updatedDest := strings.Split(localDest, dockerProtocol)[1]

		buildOptions, err := getStandardBuildOptions(updatedDest, o.SrcTlsVerify)
		if err != nil {
			return result, excludeCatalogs, err
		}

		buildOptions.DefaultMountsFilePath = ""

		o.Logger.Trace("containerfile %s", string(contents.Bytes()))

		buildStoreOptions, err := storage.DefaultStoreOptions()
		if err != nil {
			return result, excludeCatalogs, err
		}

		buildStore, err := storage.GetStore(buildStoreOptions)
		if err != nil {
			return result, excludeCatalogs, err
		}
		defer buildStore.Shutdown(false)

		os.MkdirAll("configs", 0644)
		filecopy.Copy(v.FilteredConfigPath, "./configs")
		defer os.RemoveAll("configs")

		id, ref, err := imagebuildah.BuildDockerfiles(ctx, buildStore, buildOptions, []string{containerfilePath}...)
		if err == nil && buildOptions.Manifest != "" {
			o.Logger.Info("✅ successfully created catalog")
			o.Logger.Debug("  manifest list id : %s", id)
			o.Logger.Debug("  image reference  : %s", ref.String())
		}
		if err != nil {
			return result, excludeCatalogs, err
		}

		var retries *uint
		retries = new(uint)
		*retries = 3

		manifestPushOptions := manifests.PushOptions{
			Store:                  buildStore,
			SystemContext:          newSystemContext(o.DestTlsVerify),
			ImageListSelection:     cp.CopyAllImages,
			Instances:              nil,
			RemoveSignatures:       true,
			SignBy:                 "",
			ManifestType:           "application/vnd.oci.image.manifest.v1+json",
			AddCompression:         []string{},
			ForceCompressionFormat: false,
			MaxRetries:             retries,
		}

		destImageRef, err := alltransports.ParseImageName(localDest)
		if err != nil {
			return result, excludeCatalogs, err
		}

		_, list, err := manifests.LoadFromImage(buildStore, id)
		if err != nil {
			return result, excludeCatalogs, err
		}

		o.Logger.Debug("local cache destination (rebuilt-catalog) %s", localDest)
		o.Logger.Debug("destination image reference %v", destImageRef)
		o.Logger.Debug("pushing manifest list to remote registry")
		// push the manifest list to local cache
		_, digest, err := list.Push(ctx, destImageRef, manifestPushOptions)
		if err != nil {
			return result, excludeCatalogs, err
		}

		_, err = buildStore.DeleteImage(id, true)
		if err != nil {
			return result, excludeCatalogs, err
		}

		o.Logger.Info("✅ successfully pushed catalog manifest list")
		o.Logger.Debug("  digest           : %s", digest)

		excludeImg := v2alpha1.Image{
			Name: v.OperatorFilter.Catalog,
		}
		excludeCatalogs = append(excludeCatalogs, excludeImg)

		if o.Mode == MirrorToMirror {
			copyImage := v2alpha1.CopyImageSchema{
				Origin:      v.OperatorFilter.Catalog,
				Source:      localDest,
				Destination: remoteDest,
				Type:        v2alpha1.TypeOperatorCatalog,
			}
			result = append(result, copyImage)
		}
	}
	o.Logger.Info("✅ completed rebuild catalog/s")
	o.Logger.Debug("result %v", result)
	return result, excludeCatalogs, nil
}

func newSystemContext(tlsVerify bool) *types.SystemContext {
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "",
		DockerInsecureSkipTLSVerify: types.NewOptionalBool(!tlsVerify),
	}
	return ctx
}

func getStandardBuildOptions(destination string, tlsVerify bool) (define.BuildOptions, error) {
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

	var jobs *int
	jobs = new(int)
	*jobs = 4

	buildOptions := define.BuildOptions{
		AddCapabilities:         capabilitiesForRoot,
		AdditionalBuildContexts: nil,
		AdditionalTags:          nil,
		AllPlatforms:            false,
		Annotations:             nil,
		Architecture:            "",
		Args:                    nil,
		BlobDirectory:           "",
		BuildOutput:             "",
		CacheFrom:               nil,
		CacheTo:                 nil,
		CacheTTL:                0,
		CDIConfigDir:            "",
		CNIConfigDir:            "",
		CNIPluginPath:           "",
		CompatVolumes:           types.NewOptionalBool(false),
		ConfidentialWorkload:    define.ConfidentialWorkloadOptions{},
		CPPFlags:                nil,
		CommonBuildOpts:         nil,
		Compression:             imagebuildah.Uncompressed,
		ConfigureNetwork:        buildah.NetworkDisabled,
		ContextDirectory:        "",
		Devices:                 []string{},
		DropCapabilities:        nil,
		Err:                     io.Discard,
		Excludes:                nil,
		ForceRmIntermediateCtrs: false,
		From:                    "",
		GroupAdd:                nil,
		IDMappingOptions:        nil,
		IIDFile:                 "",
		IgnoreFile:              "",
		In:                      nil,
		Isolation:               buildah.IsolationOCIRootless,
		Jobs:                    jobs,
		Labels:                  []string{},
		LayerLabels:             []string{},
		Layers:                  false,
		LogFile:                 "none",
		LogRusage:               false,
		LogSplitByPlatform:      false,
		Manifest:                destination,
		MaxPullPushRetries:      2,
		NamespaceOptions:        nil,
		NoCache:                 true,
		OS:                      "linux",
		OSFeatures:              nil,
		OSVersion:               "",
		OciDecryptConfig:        nil,
		Out:                     io.Discard,
		Output:                  "",
		OutputFormat:            "application/vnd.oci.image.manifest.v1+json",
		Platforms:               platforms,
		PullPolicy:              define.PullAlways,
		Quiet:                   true,
		RemoveIntermediateCtrs:  false,
		ReportWriter:            io.Discard,
		Runtime:                 "crun",
		RuntimeArgs:             nil,
		RusageLogFile:           "",
		SBOMScanOptions:         nil,
		SignBy:                  "",
		SignaturePolicyPath:     "",
		SkipUnusedStages:        types.NewOptionalBool(false),
		Squash:                  false,
		SystemContext:           newSystemContext(tlsVerify),
		Target:                  "",
		Timestamp:               nil,
		TransientMounts:         nil,
		UnsetEnvs:               nil,
		UnsetLabels:             nil,
	}
	return buildOptions, nil
}
