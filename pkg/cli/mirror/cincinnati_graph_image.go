package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	archiver "github.com/mholt/archiver/v3"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/image/builder"
)

// This is a temporary solution until this data is distributed as container images
// https://github.com/openshift/enhancements/pull/310

const (
	// Base image to use when build graph image
	graphBaseImage = "registry.access.redhat.com/ubi8/ubi-micro:latest"
	// URL where graph archive is stored
	graphURL       = "https://api.openshift.com/api/upgrades_info/graph-data"
	outputFile     = "cincinnati-graph-data.tar.gz"
	graphDataDir   = "/var/lib/cincinnati-graph-data/"
	getDataTimeout = time.Minute * 60
)

// unpackRelease will unpack Cincinnati graph data if it exists in the archive
func (o *MirrorOptions) unpackRelease(dstDir string, filesInArchive map[string]string) (bool, error) {
	var found bool
	if err := unpack(config.GraphDataDir, dstDir, filesInArchive); err != nil {
		nferr := &ErrArchiveFileNotFound{}
		if errors.As(err, &nferr) || errors.Is(err, os.ErrNotExist) {
			klog.V(1).Info("No  graph data found in archive, skipping graph image build")
			return found, nil
		}
		return found, err
	}
	found = true
	return found, nil
}

// buildGraphImage builds and publishes an image containing the unpacked Cincinnati graph data
func (o *MirrorOptions) buildGraphImage(ctx context.Context, dstDir string) (image.TypedImageMapping, error) {
	refs := image.TypedImageMapping{}

	var destInsecure bool
	if o.DestPlainHTTP || o.DestSkipTLS {
		destInsecure = true
	}

	nameOpts := getNameOpts(destInsecure)
	remoteOpts := getRemoteOpts(ctx, destInsecure)
	var err error
	mirrorRef := imagesource.TypedImageReference{Type: imagesource.DestinationRegistry}
	mirrorRef.Ref, err = reference.Parse(o.ToMirror)
	if err != nil {
		return nil, err
	}

	// The UBI image has been pulled and is expected to be available
	// as a base for the graph image
	ubiImage, err := imagesource.ParseReference(graphBaseImage)
	if err != nil {
		return refs, fmt.Errorf("error parsing image %q: %v", graphBaseImage, err)
	}

	ubiImage.Ref.Registry = mirrorRef.Ref.Registry
	ubiImage.Ref.Namespace = path.Join(o.UserNamespace, ubiImage.Ref.Namespace)

	graphImage := ubiImage
	graphImage.Ref.Namespace = path.Join(o.UserNamespace, "openshift")
	graphImage.Ref.Name = "graph-image"

	imgBuilder := builder.NewImageBuilder(nameOpts, remoteOpts)
	layoutDir := filepath.Join(dstDir, "layout")

	// unpack graph data archive and build image
	graphToFile := filepath.Join(dstDir, config.GraphDataDir, outputFile)
	graphDataFolder := filepath.Join(dstDir, config.GraphDataDir, "/graph-data")

	err = archiver.Unarchive(graphToFile, graphDataFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tarball %v", err)
	}

	add, err := builder.LayerFromPath(graphDataDir, graphDataFolder)
	if err != nil {
		return refs, fmt.Errorf("error creating add layer: %v", err)
	}

	cpCmd := fmt.Sprintf("cp -rp %s/* /var/lib/cincinnati/graph-data", graphDataDir)

	update := func(cfg *v1.ConfigFile) {
		cfg.Config.Cmd = []string{"/bin/bash", "-c", cpCmd}
		cfg.Author = "oc-mirror"
	}
	layoutPath, err := imgBuilder.CreateLayout(ubiImage.Ref.Exact(), layoutDir)
	if err != nil {
		return refs, fmt.Errorf("error creating OCI layout: %v", err)
	}
	if err := imgBuilder.Run(ctx, graphImage.Ref.Exact(), layoutPath, nil, update, add); err != nil {
		return refs, nil
	}

	graphImgCvt := image.TypedImageReference{
		Ref:  graphImage.Ref,
		Type: graphImage.Type,
	}
	// Add to mapping for UpdateService manifest generation
	refs.Add(graphImgCvt, graphImgCvt, v1alpha2.TypeCincinnatiGraph)

	resolver, err := containerdregistry.NewResolver("", o.DestSkipTLS, o.DestPlainHTTP, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating image resolver: %v", err)
	}

	// Resolve the image's digest for UpdateService manifest creation
	for source, dest := range refs {
		_, desc, err := resolver.Resolve(ctx, dest.Ref.Exact())
		if err != nil {
			return nil, fmt.Errorf("error retrieving digest for graph image %q: %v", dest.Ref.Exact(), err)
		}
		dest.Ref.ID = desc.Digest.String()
		refs[source] = dest
	}

	return refs, nil
}

// downloadsGraphData will download the current Cincinnati graph data
func downloadGraphData(ctx context.Context, dir string) error {
	// TODO(jpower432): It would be helpful to validate
	// the source of this downloaded file before processing
	// it further
	graphArchive := filepath.Join(dir, outputFile)
	out, err := os.Create(filepath.Clean(graphArchive))
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequest("GET", graphURL, nil)
	if err != nil {
		return err
	}

	client := http.Client{}
	tls, err := getTLSConfig()
	if err != nil {
		return err
	}
	transport := &http.Transport{
		TLSClientConfig: tls,
		Proxy:           http.ProxyFromEnvironment,
	}
	client.Transport = transport
	timeoutCtx, cancel := context.WithTimeout(ctx, getDataTimeout)
	defer cancel()

	resp, err := client.Do(req.WithContext(timeoutCtx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}
