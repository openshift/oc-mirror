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
	"time"

	"github.com/containers/buildah"
	"github.com/containers/storage"
	"github.com/opencontainers/go-digest"

	v5 "github.com/containers/image/v5/docker/reference"

	"github.com/RedHatGov/bundle/pkg/operator"
	"github.com/containerd/containerd/errdefs"
	"github.com/containers/buildah/define"
	"github.com/containers/buildah/imagebuildah"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/storage/pkg/unshare"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
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

		// Build and push a new image with the same namespace, name, and optionally tag
		// as the original image, but to the mirror.
		digest, can, err := buildCatalogImage(ctx, ctlgRef.Ref, dcDirToBuild)
		if err != nil {
			return nil, fmt.Errorf("error building catalog image %q: %v", ctlgRef.Ref.Exact(), err)
		}
		logrus.Info(digest)
		logrus.Info(can)

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

func buildCatalogImage(ctx context.Context, ref reference.DockerImageReference, dir string) (digest.Digest, v5.Canonical, error) {
	dockerfile := filepath.Join(dir, "index.Dockerfile")
	f, err := os.Create(dockerfile)
	if err != nil {
		return "", nil, err
	}
	if err := (action.GenerateDockerfile{
		BaseImage: operator.OPMImage,
		IndexDir:  ".",
		Writer:    f,
	}).Run(); err != nil {
		return "", nil, err
	}

	logrus.Infof("Building rendered catalog image: %s", ref.Exact())

	unshare.MaybeReexecUsingUserNamespace(false)

	buildStoreOptions, err := storage.DefaultStoreOptions(unshare.IsRootless(), unshare.GetRootlessUID())

	if err != nil {
		panic(err)
	}

	buildStore, err := storage.GetStore(buildStoreOptions)

	if err != nil {
		panic(err)
	}

	// refSlice is the image tag
	var refSlice []string
	refSlice = append(refSlice, ref.Exact())

	// timestamp for image metadata
	var timestamp *time.Time
	t := time.Unix(0, 0).UTC()
	timestamp = &t

	var platforms []struct{ OS, Arch, Variant string }

	options := define.BuildOptions{
		AdditionalTags:   refSlice,
		Timestamp:        timestamp,
		ContextDirectory: dir,
		Platforms:        platforms,
	}
	var containerfiles []string
	containerfiles = append(containerfiles, dockerfile)

	_, cannon, err := imagebuildah.BuildDockerfiles(ctx, buildStore, options, containerfiles...)
	if err != nil {
		logrus.Error(err)
		return "", nil, err
	}

	pushopts := buildah.PushOptions{
		Store: buildStore,
	}

	iname := cannon.String()

	dest, err := alltransports.ParseImageName(iname)
	// add the docker:// transport to see if they neglected it.
	if err != nil {
		destTransport := strings.Split(iname, ":")[0]
		if t := transports.Get(destTransport); t != nil {
			return "", nil, err
		}

		if strings.Contains(iname, "://") {
			return "", nil, err
		}

		iname = "docker://" + iname
		dest2, err2 := alltransports.ParseImageName(iname)
		if err2 != nil {
			return "", nil, err
		}
		dest = dest2
		logrus.Debugf("Assuming docker:// as the transport method for DESTINATION: %s", iname)
	}

	cannon, digest, err := buildah.Push(ctx, iname, dest, pushopts)
	if err != nil {
		logrus.Error(err)
	}
	logrus.Info(digest)
	logrus.Info(cannon)
	/*
		args := []string{
			"buildx", "build",
			"-t", ref.Exact(),
			"-f", dockerfile,
			"--platform", strings.Join(o.CatalogPlatforms, ","),
			"--push",
			dir,
		}
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		logrus.Debugf("command: %s", strings.Join(cmd.Args, " "))

		return cmd.Run()
	*/
	return digest, cannon, err
}
