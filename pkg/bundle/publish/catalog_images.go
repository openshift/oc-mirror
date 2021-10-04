package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/RedHatGov/bundle/pkg/operator"
	"github.com/containers/buildah"
	"github.com/containers/buildah/define"
	"github.com/containers/storage"
	"github.com/opencontainers/go-digest"

	v5 "github.com/containers/image/v5/docker/reference"
	is "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"

	"github.com/containerd/containerd/errdefs"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/storage/pkg/unshare"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
)

func (o *Options) rebuildCatalogs(ctx context.Context, dstDir string, filesInArchive map[string]string) (refs []imagesource.TypedImageReference, err error) {
	if err := unpack("catalogs", dstDir, filesInArchive); err != nil {
		return nil, err
	}

	mirrorRef := imagesource.TypedImageReference{Type: imagesource.DestinationRegistry}
	if mirrorRef.Ref, err = reference.Parse(o.ToMirror); err != nil {
		return nil, err
	}

	dstDir = filepath.Clean(dstDir)
	catalogsByImage := map[imagesource.TypedImageReference]string{}
	if err := filepath.Walk(dstDir, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return err
		}

		slashPath := filepath.ToSlash(fpath)
		if base := path.Base(slashPath); base == "index.json" {
			slashPath = strings.TrimPrefix(slashPath, fmt.Sprintf("%s/catalogs/", dstDir))
			regRepoNs, id := path.Split(path.Dir(slashPath))
			regRepoNs = path.Clean(regRepoNs)
			var img string
			if strings.Contains(id, ":") {
				// Digest.
				img = fmt.Sprintf("%s@%s", regRepoNs, id)
			} else {
				// Tag.
				img = fmt.Sprintf("%s:%s", regRepoNs, id)
			}
			ctlgRef := imagesource.TypedImageReference{Type: imagesource.DestinationRegistry}
			if ctlgRef.Ref, err = reference.Parse(img); err != nil {
				return fmt.Errorf("error parsing index dir path %q as image %q: %v", fpath, img, err)
			}
			// Update registry so the existing catalog image can be pulled.
			// QUESTION(estroz): is assuming an image is present in a repo with the same name valid?
			ctlgRef.Ref.Registry = mirrorRef.Ref.Registry
			catalogsByImage[ctlgRef] = filepath.Dir(fpath)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	resolver, err := containerdregistry.NewResolver("", o.SkipTLS, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating image resolver: %v", err)
	}
	reg, err := containerdregistry.NewRegistry(
		containerdregistry.SkipTLS(o.SkipTLS),
		containerdregistry.WithCacheDir(filepath.Join(dstDir, "cache")),
	)
	if err != nil {
		return nil, err
	}
	defer reg.Destroy()
	for ctlgRef, dcDir := range catalogsByImage {

		// An image for a particular catalog may not exist in the mirror registry yet,
		// ex. when publish is run for the first time for a catalog (full/headsonly).
		// If that is the case, then simply build the catalog image with the new
		// declarative config catalog; otherwise render the existing and new catalogs together.
		var dcDirToBuild string
		refExact := ctlgRef.Ref.Exact()
		if _, _, rerr := resolver.Resolve(ctx, refExact); rerr == nil {

			logrus.Infof("Catalog image %q found, rendering with new file-based catalog", refExact)

			dc, err := action.Render{
				// Order the old ctlgRef before dcDir so new packages/channels/bundles overwrite
				// existing counterparts.
				Refs:           []string{refExact, dcDir},
				AllowedRefMask: action.RefAll,
				Registry:       reg,
			}.Run(ctx)
			if err != nil {
				return nil, err
			}
			dcDirToBuild = filepath.Join(dcDir, "rendered")
			if err := os.MkdirAll(dcDirToBuild, os.ModePerm); err != nil {
				return nil, err
			}
			renderedPath := filepath.Join(dcDirToBuild, "index.json")
			f, err := os.Create(renderedPath)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			if err := declcfg.WriteJSON(*dc, f); err != nil {
				return nil, err
			}

		} else if errors.Is(rerr, errdefs.ErrNotFound) {

			logrus.Infof("Catalog image %q not found, using new file-based catalog", refExact)
			dcDirToBuild = dcDir

		} else {
			return nil, fmt.Errorf("error resolving existing catalog image %q: %v", refExact, rerr)
		}

		sctx := types.SystemContext{}

		// Build and push a new image with the same namespace, name, and optionally tag
		// as the original image, but to the mirror.
		_, err := buildCatalogImage(ctx, ctlgRef.Ref, sctx, dcDirToBuild)
		if err != nil {
			return nil, fmt.Errorf("error building catalog image %q: %v", ctlgRef.Ref.Exact(), err)
		}

		// Resolve the image's digest for ICSP creation.
		_, desc, err := resolver.Resolve(ctx, ctlgRef.Ref.Exact())
		if err != nil {
			return nil, fmt.Errorf("error retrieving digest for catalog image %q: %v", ctlgRef.Ref.Exact(), err)
		}
		ctlgRef.Ref.ID = desc.Digest.String()

		refs = append(refs, ctlgRef)
	}

	return refs, nil
}

func buildCatalogImage(ctx context.Context, ref reference.DockerImageReference, sctx types.SystemContext, dir string) (v5.Canonical, error) {
	// MaybeExecUsingUserNamespace ends up calling os.Exit() after the push. This catches the panic.
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
	containerfile := filepath.Join(dir, "index.Dockerfile")

	writeContainerfile(dir, containerfile)
	// Rootless buildah
	if buildah.InitReexec() {
		return nil, nil
	}
	// Rootless buildah
	unshare.MaybeReexecUsingUserNamespace(false)

	buildStoreOptions, err := storage.DefaultStoreOptionsAutoDetectUID()
	if err != nil {
		logrus.Errorf("%v", err)
		os.Exit(1)
	}

	buildStore, err := storage.GetStore(buildStoreOptions)
	if err != nil {
		return nil, err
	}
	/*  // This commented section is the failing bud config.
	    // We are getting an empty error from the imagebuildah package
		// that tanks the build whenever it processes any command after the
		// "FROM" command in the containerfile. I am leaving this here
		// for the intrepid developer that wants to make this work.
		// refSlice is the image tag
		var refSlice []string
		refSlice = append(refSlice, ref.Exact())

		options := define.BuildOptions{
			AdditionalTags:   refSlice,
			CommonBuildOpts:  &define.CommonBuildOptions{},
			ContextDirectory: dir,
			//AllPlatforms:     true,
			SystemContext: &sctx,
		}

		id, cannon, err := imagebuildah.BuildDockerfiles(ctx, buildStore, options, containerfile)
		if err != nil {
			logrus.Error(err)
			return nil, err
		}
		logrus.Info(id)
	*/
	// TODO: get rid of buildah commit for multi-arch builds
	sctx.ArchitectureChoice = "amd64"
	sctx.OSChoice = "linux"
	opts := buildah.BuilderOptions{
		FromImage:        operator.OPMImage,
		Isolation:        define.IsolationChroot,
		CommonBuildOpts:  &define.CommonBuildOptions{},
		ConfigureNetwork: define.NetworkDefault,
		// SystemContext has a lot of tweaks including insecure push.
		SystemContext: &sctx,
	}
	// Using a builder here because bud is not easily integrated.
	// This is an issue because we cannot commit multi-arch images. So,
	// we are stuck with amd64 until we get a better build option.
	// Boilerplate for this here:
	// https://github.com/containers/buildah/blob/main/docs/tutorials/04-include-in-your-build-tool.md#complete-code
	builder, err := buildah.NewBuilder(ctx, buildStore, opts)
	if err != nil {
		logrus.Error(err)
		panic(err)
	}
	builder.SetEntrypoint([]string{"/bin/opm"})
	builder.SetCmd([]string{"serve", "/configs"})
	err = builder.Add("/configs", false, buildah.AddAndCopyOptions{}, ".")
	if err != nil {
		logrus.Error(err)
		panic(err)
	}
	builder.SetLabel(containertools.ConfigsLocationLabel, "/configs")

	imageRef, err := is.Transport.ParseStoreReference(buildStore, ref.Exact())
	if err != nil {
		logrus.Error(err)
		panic(err)
	}
	imageId, cannon, _, err := builder.Commit(ctx, imageRef, buildah.CommitOptions{})
	if err != nil {
		logrus.Error(err)
		panic(err)
	}

	image := imageDeets{
		name: cannon,
		id:   imageId,
	}

	image.pushImage(ctx, buildStore, sctx)
	_, storeErr := buildStore.Shutdown(false)
	if err != nil {
		logrus.Errorf("%v", storeErr)
		os.Exit(1)
	}
	return cannon, err
}

func (image imageDeets) pushImage(ctx context.Context, store storage.Store, sctx types.SystemContext) (digest.Digest, v5.Canonical, error) {
	pushopts := buildah.PushOptions{
		Store:         store,
		SystemContext: &sctx,
	}

	name := image.name.Name()

	dest, err := alltransports.ParseImageName(name)
	// add the docker:// transport to see if they neglected it.
	if err != nil {
		destTransport := strings.Split(name, ":")[0]
		if t := transports.Get(destTransport); t != nil {
			return "", nil, err
		}

		if strings.Contains(name, "://") {
			return "", nil, err
		}

		imageId := "docker://" + name
		dest2, err := alltransports.ParseImageName(imageId)
		if err != nil {
			return "", nil, err
		}
		dest = dest2
		logrus.Debugf("Assuming docker:// as the transport method for DESTINATION: %s", imageId)
	}

	cannon, digest, err := buildah.Push(ctx, image.id, dest, pushopts)
	if err != nil {
		logrus.Error(err)
	}
	logrus.Debugf("Image pushed! %s\n", cannon)

	return digest, cannon, err

}

func writeContainerfile(dir string, cfile string) error {

	f, err := os.Create(cfile)
	if err != nil {
		logrus.Error(err)
		return err
	}
	if err := (action.GenerateDockerfile{
		BaseImage: operator.OPMImage,
		IndexDir:  ".",
		Writer:    f,
	}).Run(); err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Infoln("Building rendered catalog image")
	return nil
}

type imageDeets struct {
	name v5.Canonical
	id   string
}
