package mirror

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
)

func (o *MirrorOptions) rebuildCatalogs(ctx context.Context, dstDir string, filesInArchive map[string]string) (refs []imagesource.TypedImageReference, err error) {
	if err := unpack("catalogs", dstDir, filesInArchive); err != nil {
		nferr := &ErrArchiveFileNotFound{}
		if errors.As(err, &nferr) || errors.Is(err, os.ErrNotExist) {
			logrus.Debug("No catalogs found in archive, skipping catalog rebuild")
			return nil, nil
		}
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

	resolver, err := containerdregistry.NewResolver("", o.DestSkipTLS, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating image resolver: %v", err)
	}
	reg, err := containerdregistry.NewRegistry(
		containerdregistry.SkipTLS(o.DestSkipTLS),
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
		var srcImage string
		var layers []v1.Layer
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

			delete, err := deleteLayer("/configs/.wh.index.json")
			if err != nil {
				return refs, fmt.Errorf("error creating delete layer: %v", err)
			}
			add, err := addLayer(renderedPath, "/configs")
			if err != nil {
				return refs, fmt.Errorf("error creating add layer: %v", err)
			}
			layers = append(layers, delete, add)

			srcImage = ctlgRef.Ref.Exact()

		} else if errors.Is(rerr, errdefs.ErrNotFound) {

			logrus.Infof("Catalog image %q not found, using new file-based catalog", refExact)
			dcDirToBuild = dcDir

			add, err := addLayer(filepath.Join(dcDir, "index.json"), "/configs")
			if err != nil {
				return refs, fmt.Errorf("error creating add layer: %v", err)
			}
			layers = append(layers, add)

			srcImage = OPMImage

		} else {
			return nil, fmt.Errorf("error resolving existing catalog image %q: %v", refExact, rerr)
		}

		if len(o.BuildxPlatforms) == 0 {
			err = buildCatalogLayer(srcImage, ctlgRef.Ref.Exact(), layers...)
		} else {
			err = o.buildDockerBuildx(ctx, ctlgRef.Ref, dcDirToBuild)
		}
		if err != nil {
			return nil, err
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

func deleteLayer(old string) (v1.Layer, error) {
	deleteMap := map[string][]byte{}
	deleteMap[old] = []byte{}
	return crane.Layer(deleteMap)
}

func addLayer(new, targetPath string) (v1.Layer, error) {
	return layerFromFile(targetPath, new)
}

func buildCatalogLayer(ref, targetRef string, layer ...v1.Layer) error {
	logrus.Debugf("Pulling image %s for processing", ref)
	img, err := crane.Pull(ref)
	if err != nil {
		return err
	}

	// Add new layers to image
	newImg, err := mutate.AppendLayers(img, layer...)
	if err != nil {
		return err
	}

	// Update image config
	cfg, err := newImg.ConfigFile()
	if err != nil {
		return err
	}

	labels := map[string]string{
		containertools.ConfigsLocationLabel: "/configs",
	}
	cfg.Config.Labels = labels
	cfg.Config.Cmd = []string{"serve", "configs"}
	cfg.Config.Entrypoint = []string{"/bin/opm"}
	newerImg, err := mutate.Config(newImg, cfg.Config)
	if err != nil {
		return err
	}
	tag, err := name.NewTag(targetRef)
	if err != nil {
		return err
	}
	if err := crane.Push(newerImg, tag.String()); err != nil {
		return err
	}
	return err
}

func (o *MirrorOptions) buildDockerBuildx(ctx context.Context, ref reference.DockerImageReference, dir string) error {
	dockerfile := filepath.Join(dir, "index.Dockerfile")
	f, err := os.Create(dockerfile)
	if err != nil {
		return err
	}
	if err := (action.GenerateDockerfile{
		BaseImage: OPMImage,
		IndexDir:  ".",
		Writer:    f,
	}).Run(); err != nil {
		return err
	}

	exactRef := ref.Exact()

	args := []string{
		"build", "buildx",
		"-t", exactRef,
		"-f", dockerfile,
		"--platform", strings.Join(o.BuildxPlatforms, ","),
		"--push",
		dir,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := runDebug(cmd); err != nil {
		return err
	}

	return nil
}

// layerFromFile will write the contents of the path the target
// directory and build a v1.Layer
func layerFromFile(targetPath, path string) (v1.Layer, error) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)

	logrus.Debugf("Processing file %s", path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(path)

	hdr := &tar.Header{
		Name: filepath.Join(targetPath, filepath.ToSlash(base)),
		Mode: int64(info.Mode()),
	}

	if !info.IsDir() {
		hdr.Size = info.Size()
	}

	if info.Mode().IsDir() {
		hdr.Typeflag = tar.TypeDir
	} else if info.Mode().IsRegular() {
		hdr.Typeflag = tar.TypeReg
	} else {
		return nil, fmt.Errorf("not implemented archiving file type %s (%s)", info.Mode(), base)
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("failed to write tar header: %w", err)
	}
	if !info.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(tw, f); err != nil {
			return nil, fmt.Errorf("failed to read file into the tar: %w", err)
		}
		f.Close()
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish tar: %w", err)
	}
	return tarball.LayerFromReader(&b)
}

func runDebug(cmd *exec.Cmd) error {
	logrus.Debugf("command: %s", strings.Join(cmd.Args, " "))
	return cmd.Run()
}
