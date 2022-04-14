package mirror

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	operatorimage "github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/image/builder"
	"github.com/openshift/oc-mirror/pkg/operator"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

// unpackCatalog will unpack file-based catalogs if they exists
func (o *MirrorOptions) unpackCatalog(dstDir string, filesInArchive map[string]string) (bool, error) {
	var found bool
	if err := unpack(CatalogsDir, dstDir, filesInArchive); err != nil {
		nferr := &ErrArchiveFileNotFound{}
		if errors.As(err, &nferr) || errors.Is(err, os.ErrNotExist) {
			klog.V(4).Info("No catalogs found in archive, skipping catalog rebuild")
			return found, nil
		}
		return found, err
	}
	found = true
	return found, nil
}

func (o *MirrorOptions) rebuildCatalogs(ctx context.Context, dstDir string) (image.TypedImageMapping, error) {
	refs := image.TypedImageMapping{}
	var err error

	mirrorRef := imagesource.TypedImageReference{Type: imagesource.DestinationRegistry}
	mirrorRef.Ref, err = reference.Parse(o.ToMirror)
	if err != nil {
		return nil, err
	}

	dstDir = filepath.Clean(dstDir)
	catalogsByImage := map[imagesource.TypedImageReference]string{}
	if err := filepath.Walk(dstDir, func(fpath string, info fs.FileInfo, err error) error {

		// Skip the layouts dir because we only need
		// to process the parent directory one time
		if filepath.Base(fpath) == LayoutsDir {
			return filepath.SkipDir
		}

		if err != nil || info == nil {
			return err
		}

		// From the index path determine the artifacts (index and layout) directory.
		// Using that path determine the corresponding catalog image for processing.
		slashPath := filepath.ToSlash(fpath)
		if base := path.Base(slashPath); base == "index.json" {
			slashPath = path.Dir(slashPath)
			slashPath = strings.TrimSuffix(slashPath, IndexDir)

			repoPath := strings.TrimPrefix(slashPath, fmt.Sprintf("%s/%s/", dstDir, CatalogsDir))
			regRepoNs, id := path.Split(path.Dir(repoPath))
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
			sourceRef, err := imagesource.ParseReference(img)
			if err != nil {
				return fmt.Errorf("error parsing index dir path %q as image %q: %v", fpath, img, err)
			}
			ctlgRef.Ref = sourceRef.Ref
			// Update registry so the existing catalog image can be pulled.
			ctlgRef.Ref.Registry = mirrorRef.Ref.Registry
			ctlgRef.Ref.Namespace = path.Join(o.UserNamespace, ctlgRef.Ref.Namespace)

			catalogsByImage[ctlgRef] = slashPath

			// Add to mapping for ICSP generation
			refs.Add(sourceRef, ctlgRef, v1alpha2.TypeOperatorCatalog)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	resolver, err := containerdregistry.NewResolver("", o.DestSkipTLS, o.DestPlainHTTP, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating image resolver: %v", err)
	}
	reg, err := containerdregistry.NewRegistry(
		containerdregistry.SkipTLSVerify(o.DestSkipTLS),
		containerdregistry.WithPlainHTTP(o.DestPlainHTTP),
		containerdregistry.WithCacheDir(filepath.Join(dstDir, "cache")),
	)
	if err != nil {
		return nil, err
	}
	defer reg.Destroy()

	if err := o.processCatalogRefs(ctx, catalogsByImage, reg, resolver); err != nil {
		return nil, err
	}

	// Resolve the image's digest for ICSP creation.
	for source, dest := range refs {
		_, desc, err := resolver.Resolve(ctx, dest.Ref.Exact())
		if err != nil {
			return nil, fmt.Errorf("error retrieving digest for catalog image %q: %v", dest.Ref.Exact(), err)
		}
		dest.Ref.ID = desc.Digest.String()
		refs[source] = dest
	}

	return refs, nil
}

func (o *MirrorOptions) processCatalogRefs(ctx context.Context, catalogsByImage map[imagesource.TypedImageReference]string, reg operatorimage.Registry, resolver remotes.Resolver) error {
	for ctlgRef, artifactDir := range catalogsByImage {
		// An image for a particular catalog may not exist in the mirror registry yet,
		// ex. when publish is run for the first time for a catalog (full/headsonly).
		// If that is the case, then simply build the catalog image with the new
		// declarative config catalog; otherwise render the existing and new catalogs together.
		var layers []v1.Layer
		var layoutPath layout.Path
		refExact := ctlgRef.Ref.Exact()

		var destInsecure bool
		if o.DestPlainHTTP || o.DestSkipTLS {
			destInsecure = true
		}

		// Check push permissions before trying to resolve for Quay compatibility
		nameOpts := getNameOpts(destInsecure)
		remoteOpts := getRemoteOpts(ctx, destInsecure)
		ref, err := name.ParseReference(refExact, nameOpts...)
		if err != nil {
			return err
		}
		err = remote.CheckPushPermission(ref, authn.DefaultKeychain, createRT(destInsecure))
		if err != nil {
			return err
		}
		imgBuilder := &builder.ImageBuilder{
			NameOpts:   nameOpts,
			RemoteOpts: remoteOpts,
		}

		switch _, _, rerr := resolver.Resolve(ctx, refExact); {
		case errors.Is(rerr, errdefs.ErrNotFound):
			klog.Infof("Catalog image %q found, building from file with file-based catalog ", refExact)

			add, err := builder.LayerFromPath("/configs", filepath.Join(artifactDir, IndexDir, "index.json"))
			if err != nil {
				return fmt.Errorf("error creating add layer: %v", err)
			}

			// Since we are defining the FBC as index.json, remove
			// any .yaml files from the initial image to ensure they are not processed instead
			deleted, err := deleteLayer("/configs/.wh.index.yaml")
			if err != nil {
				return fmt.Errorf("error creating deleted layer: %v", err)
			}
			layers = append(layers, add, deleted)

			layoutDir := filepath.Join(artifactDir, LayoutsDir)
			layoutPath, err = imgBuilder.CreateLayout("", layoutDir)
			if err != nil {
				return fmt.Errorf("error creating OCI layout: %v", err)
			}
		case rerr == nil:
			klog.Infof("Catalog image %q found, rendering with file-based catalog", refExact)

			dcDir := filepath.Join(artifactDir, IndexDir)
			dc, err := action.Render{
				// Order the old ctlgRef before dcDir so new packages/channels/bundles overwrite
				// existing counterparts.
				Refs:           []string{refExact, dcDir},
				AllowedRefMask: action.RefAll,
				Registry:       reg,
			}.Run(ctx)
			if err != nil {
				return err
			}
			// Remove any duplicate objects
			merger := &operator.TwoWayStrategy{}
			if err := merger.Merge(dc); err != nil {
				return err
			}
			dcDirToBuild := filepath.Join(dcDir, "rendered")
			if err := os.MkdirAll(dcDirToBuild, os.ModePerm); err != nil {
				return err
			}
			renderedPath := filepath.Join(dcDirToBuild, "index.json")
			f, err := os.Create(renderedPath)
			if err != nil {
				return err
			}
			defer f.Close()
			if err := declcfg.WriteJSON(*dc, f); err != nil {
				return err
			}

			add, err := builder.LayerFromPath("/configs", renderedPath)
			if err != nil {
				return fmt.Errorf("error creating add layer: %v", err)
			}
			layers = append(layers, add)

			layoutDir := filepath.Join(dcDir, "layout")
			layoutPath, err = imgBuilder.CreateLayout(refExact, layoutDir)
			if err != nil {
				return fmt.Errorf("error creating OCI layout: %v", err)
			}
		default:
			return fmt.Errorf("error resolving existing catalog image %q: %v", refExact, rerr)
		}

		update := func(cfg *v1.ConfigFile) {
			labels := map[string]string{
				containertools.ConfigsLocationLabel: "/configs",
			}
			cfg.Config.Labels = labels
			cfg.Config.Cmd = []string{"serve", "/configs"}
			cfg.Config.Entrypoint = []string{"/bin/opm"}
		}
		if err := imgBuilder.Run(ctx, refExact, layoutPath, update, layers...); err != nil {
			return fmt.Errorf("error building catalog layers: %v", err)
		}
	}
	return nil
}

func deleteLayer(old string) (v1.Layer, error) {
	deleteMap := map[string][]byte{}
	deleteMap[old] = []byte{}
	return crane.Layer(deleteMap)
}
