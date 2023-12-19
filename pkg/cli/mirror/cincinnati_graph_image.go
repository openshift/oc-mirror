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
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	archiver "github.com/mholt/archiver/v3"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/verify/util"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/image/builder"
	corev1 "k8s.io/api/core/v1"
)

// This is a temporary solution until this data is distributed as container images
// https://github.com/openshift/enhancements/pull/310

const (
	// Base image to use when build graph image
	graphBaseImage = "registry.access.redhat.com/ubi8/ubi-micro:latest"
	// URL where graph archive is stored
	graphURL           = "https://api.openshift.com/api/upgrades_info/graph-data"
	outputFile         = "cincinnati-graph-data.tar.gz"
	graphDataDir       = "/var/lib/cincinnati-graph-data/"
	graphDataMountPath = "/var/lib/cincinnati/graph-data"
	getDataTimeout     = time.Minute * 60
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
func (o *MirrorOptions) buildGraphImage(ctx context.Context, srcSignatureDir string, dstDir string) (image.TypedImageMapping, error) {
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

	// Copy the signature to graph data directory
	err = copySignatureForUpdateGraph(srcSignatureDir, graphDataFolder)
	if err != nil {
		return refs, fmt.Errorf("error copying signatures to Cincinnati graph data directory: %v", err)
	}
	add, err := builder.LayerFromPath(graphDataDir, graphDataFolder)
	if err != nil {
		return refs, fmt.Errorf("error creating add layer: %v", err)
	}

	cpCmd := fmt.Sprintf("cp -rp %s/* %s", graphDataDir, graphDataMountPath)

	update := func(cfg *v1.ConfigFile) {
		cfg.Config.Cmd = []string{"/bin/bash", "-c", cpCmd}
		cfg.Author = "oc-mirror"
	}
	layoutPath, err := imgBuilder.CreateLayout(ubiImage.Ref.Exact(), layoutDir)
	if err != nil {
		return refs, fmt.Errorf("error creating OCI layout: %v", err)
	}
	if err := imgBuilder.Run(ctx, graphImage.Ref.Exact(), layoutPath, update, add); err != nil {
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
		klog.Errorf("call to Cincinatti API returned with status HTTP %s", resp.Status)
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	// TODO(jpower432): It would be helpful to validate
	// the source of this downloaded file before processing
	// it further
	graphArchive := filepath.Join(dir, outputFile)
	out, err := os.Create(filepath.Clean(graphArchive))
	if err != nil {
		return err
	}
	defer out.Close()
	bytesWritten, err := io.Copy(out, resp.Body)
	klog.V(5).Infof("HTTP body for request to Cincinnati API is %d bytes, and was written to %s", bytesWritten, graphArchive)
	return err
}

func copySignatureForUpdateGraph(srcSigDir string, dstGraphDataDir string) error {

	//Go through the files in srcSignaturePath and parse the json files.
	files, err := os.ReadDir(srcSigDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		//Read the file
		signatureRawBytes, err := os.ReadFile(filepath.Join(srcSigDir, file.Name()))
		if err != nil {
			return err
		}
		//Parse the file
		cmObj, err := util.ReadConfigMap(signatureRawBytes)
		if err != nil {
			continue
		}
		if cmObj == nil {
			continue
		}
		//Copy the signature to graph data directory
		err = copySignatureToGraphDataDir(dstGraphDataDir, cmObj)
		if err != nil {
			return fmt.Errorf("error copying signatures to Cincinnati graph data directory: %v", err)
		}
	}
	return nil
}

// copySignatureToGraphDataDir() will copy the signatures to cincinnati graph data directory
// with signatures/{algorithm}/{digest}/signature-{number} schema.
func copySignatureToGraphDataDir(graphDataSignatureDir string, cmObj *corev1.ConfigMap) error {

	sigDirPath := filepath.Join(graphDataSignatureDir, "signatures")

	//iterate through the cmObj.BinaryData map
	for key, value := range cmObj.BinaryData {
		//example key:value -> key = sha256-73946971c03b43a0dc6f7b0946b26a177c2f3c9d37105441315b4e3359373a55-1
		v := strings.Split(key, "-")
		if len(v) == 0 {
			return fmt.Errorf("invalid signature key %s", key)
		}
		algo := v[0]
		digest := v[1]
		signatureNumber := v[2]

		//create the signature directory
		sigDirPath := filepath.Join(sigDirPath, algo, digest)
		err := os.MkdirAll(sigDirPath, 0755)
		if err != nil {
			return fmt.Errorf("error creating directory %s: %s", sigDirPath, err)
		}

		// Write the signature file
		sigFilePath := filepath.Join(sigDirPath, fmt.Sprintf("signature-%s", signatureNumber))
		err = os.WriteFile(sigFilePath, []byte(value), 0644)
		if err != nil {
			return fmt.Errorf("error writing to the %s: %s", sigFilePath, err)
		}
	}
	return nil
}
