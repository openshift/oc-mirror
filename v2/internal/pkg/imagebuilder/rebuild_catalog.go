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
func (o *ImageBuilder) RebuildCatalogs(ctx context.Context, catalogSchema v2alpha1.CollectorSchema) ([]v2alpha1.CopyImageSchema, error) {

	// set variables
	catalogs := catalogSchema.CatalogToFBCMap
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
		filteredDir := filepath.Dir(v.FilteredConfigPath)

		o.Logger.Info("🔂 rebuilding catalog (pulling catalog image) %s", v.OperatorFilter.Catalog)
		contents := bytes.NewBufferString("")
		tmpl, err := template.New("Containerfile").Parse(containerTemplate)
		if err != nil {
			return result, err
		}
		err = tmpl.Execute(contents, map[string]interface{}{
			"Catalog": v.OperatorFilter.Catalog,
		})
		if err != nil {
			return result, err
		}

		// write the Containerfile content to a file
		containerfilePath := filepath.Join(filteredDir, "Containerfile")

		err = os.WriteFile(containerfilePath, contents.Bytes(), 0755)
		if err != nil {
			return result, err
		}

		imgSpec, err := image.ParseRef(v.OperatorFilter.Catalog)
		if err != nil {
			return result, err
		}

		srcCache := dockerProtocol + strings.Join([]string{o.LocalFQDN, imgSpec.PathComponent}, "/") + ":" + filepath.Base(filteredDir)

		// this is safe as we know that there is a docker:// prefix
		updatedDest := strings.Split(srcCache, dockerProtocol)[1]

		buildOptions, err := getStandardBuildOptions(updatedDest, o.SrcTlsVerify)
		if err != nil {
			return result, err
		}

		buildOptions.DefaultMountsFilePath = ""

		o.Logger.Trace("containerfile %s", string(contents.Bytes()))

		buildStoreOptions, err := storage.DefaultStoreOptions()
		if err != nil {
			return result, err
		}

		buildStore, err := storage.GetStore(buildStoreOptions)
		if err != nil {
			return result, err
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
			return result, err
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

		destImageRef, err := alltransports.ParseImageName(srcCache)
		if err != nil {
			return result, err
		}

		_, list, err := manifests.LoadFromImage(buildStore, id)
		if err != nil {
			return result, err
		}

		o.Logger.Debug("local cache destination (rebuilt-catalog) %s", srcCache)
		o.Logger.Debug("destination image reference %v", destImageRef)
		o.Logger.Debug("pushing manifest list to remote registry")
		// push the manifest list to local cache
		_, digest, err := list.Push(ctx, destImageRef, manifestPushOptions)
		if err != nil {
			return result, err
		}

		digestOnly := digest.String()
		if strings.Contains(digestOnly, ":") {
			digestOnly = strings.Split(digest.String(), ":")[1]
		}

		err = os.WriteFile(filepath.Join(filteredDir, "digest"), []byte(digestOnly), 0755)
		if err != nil {
			return result, err
		}

		_, err = buildStore.DeleteImage(id, true)
		if err != nil {
			return result, err
		}

		o.Logger.Info("✅ successfully pushed catalog manifest list")
		o.Logger.Debug("  digest           : %s", digest)

		if o.Mode == MirrorToMirror {

			var dest string
			for _, img := range catalogSchema.AllImages {
				if img.Type.IsOperatorCatalog() && strings.Split(img.Origin, dockerProtocol)[1] == v.OperatorFilter.Catalog && !strings.Contains(img.Destination, o.LocalFQDN) {
					if v.OperatorFilter.TargetTag != "" {
						dest = img.Destination
					} else {
						//TODO ALEX so far we are not considering digests only, it is needed to cover the digest only as well
						parts := strings.Split(img.Destination, ":")
						parts[len(parts)-1] = filepath.Base(filteredDir)

						dest = strings.Join(parts, ":")
					}

				}
			}

			copyImage := v2alpha1.CopyImageSchema{
				Origin:           v.OperatorFilter.Catalog,
				Source:           srcCache,
				Destination:      dest,
				Type:             v2alpha1.TypeOperatorCatalog,
				IsCatalogRebuilt: true,
			}
			result = append(result, copyImage) //TODO ALEX currently in mirrorToMirror both original and filtered are being mirrored, find a way to keep the original only in the cache for mirrorToMirror
		}
	}
	o.Logger.Info("✅ completed rebuild catalog/s")
	o.Logger.Debug("result %v", result)
	return result, nil
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
