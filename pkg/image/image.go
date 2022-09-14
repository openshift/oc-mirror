package image

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	oci "github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	libgoref "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

var (
	DestinationOCI imagesource.DestinationType = "oci"
)

// GetVersionsFromImage gets the set of versions after stripping a dash-suffix,
// effectively stripping out timestamps. Example: tag "v4.11-1648566121" becomes version "v4.11"
func GetVersionsFromImage(catalog string) (map[string]int, error) {
	versionTags, err := GetTagsFromImage(catalog)
	if err != nil {
		return nil, err
	}
	versions := make(map[string]int)
	for _, vt := range versionTags {
		v := strings.Split(vt, "-")
		versions[v[0]] += 1
	}
	return versions, nil
}

// GetTagsFromImage gets the tags for an image
func GetTagsFromImage(image string) ([]string, error) {
	repo, err := name.NewRepository(image)
	if err != nil {
		return nil, err
	}
	tags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	return tags, err
}

/*
ParseReference is a wrapper function of imagesource.ParseReference

	It provides support for oci: prefixes
*/
func ParseReference(ref string) (imagesource.TypedImageReference, error) {
	if !strings.HasPrefix(ref, "oci:") {
		return imagesource.ParseReference(ref)
	}

	dstType := DestinationOCI
	ref = strings.TrimPrefix(ref, "oci:")
	ref = strings.TrimPrefix(ref, "//") //it could be that there is none
	ref = strings.TrimPrefix(ref, "/")  // case of full path

	dst, err := libgoref.Parse(ref)
	if err != nil {
		return imagesource.TypedImageReference{Ref: dst, Type: dstType}, fmt.Errorf("%q is not a valid image reference: %v", ref, err)
	}
	return imagesource.TypedImageReference{Ref: dst, Type: dstType}, nil
}

/* GetConfigDirFromOCI verifies the ref is for an image of type OCI
 * It then unpacks the layers, searches for the configs folder within
 * the unpacked content and returns it
 */
func GetConfigDirFromOCICatalog(ctx context.Context, ref string) (string, error) {

	ociImgSrc, path, err := getOCIImgSrcFromPath(ctx, ref)
	if err != nil {
		return "", err
	}
	defer ociImgSrc.Close()

	manifest, err := getManifest(ctx, ociImgSrc)
	if err != nil {
		return "", err
	}

	untarredLayer := ""
	for _, layer := range manifest.LayerInfos() {
		if !layer.EmptyLayer {
			tmpDir := "/tmp/oc-mirror/"
			err := os.MkdirAll(tmpDir, 0755)
			if err != nil {
				return "", err
			}
			layerSha := layer.Digest.String()
			layerDirName := layerSha[7:]
			untarLocation := filepath.Join(tmpDir, layerDirName)
			layerPath := filepath.Join(path, "blobs/sha256/", layerDirName)

			reader, err := os.Open(layerPath)
			if err != nil {
				return "", err
			}
			untarredLayer, err = extractLayerWithConfigs(reader, untarLocation)
			if err != nil {
				return "", err
			}
			if untarredLayer != "" {
				return untarredLayer, nil
			}
		}
	}

	return "", fmt.Errorf("%s is not a valid OCI catalog", ref)
}

func getManifest(ctx context.Context, imgSrc types.ImageSource) (manifest.Manifest, error) {
	manifestBlob, manifestType, err := imgSrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, err
	}
	manifest, err := manifest.FromBlob(manifestBlob, manifestType)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}
func getOCIImgSrcFromPath(ctx context.Context, ref string) (types.ImageSource, string, error) {
	imgRef, err := ParseReference(ref)
	if err != nil {
		return nil, "", err
	}
	if imgRef.Type != DestinationOCI {
		return nil, "", fmt.Errorf("%s is not an OCI image", ref)
	}
	path := string(os.PathSeparator) + imgRef.Ref.Namespace + string(os.PathSeparator) + imgRef.Ref.Name
	ociImgRef, err := oci.ParseReference(path)
	if err != nil {
		return nil, "", err
	}
	imgsrc, err := ociImgRef.NewImageSource(ctx, nil)
	if err != nil {
		return nil, "", err
	}

	return imgsrc, path, nil

}

// Extract
func extractLayerWithConfigs(gzipStream io.Reader, file string) (string, error) {
	var layerWithConfigs string

	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		log.Fatal("ExtractTarGz: NewReader failed")
	}

	tarReader := tar.NewReader(uncompressedStream)

	for true {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return "", err
		}

		// #TODO : replace configs with value from config layer , label "operators.operatorframework.io.index.configs.v1"
		if strings.Contains(header.Name, "configs") {
			if len(layerWithConfigs) == 0 {
				layerWithConfigs = filepath.Join(file, header.Name)
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if header.Name != "./" {
					if err := os.MkdirAll(file+"/"+header.Name, 0755); err != nil {
						return "", err
					}
				}
			case tar.TypeReg:
				outFile, err := os.Create(file + "/" + header.Name)
				if err != nil {
					return "", err
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					return "", err
				}
				outFile.Close()

			default:
				fmt.Println(fmt.Errorf("ExtractTarGz: uknown type: %v in %s", header.Typeflag, header.Name))
			}

		}
	}
	return layerWithConfigs, nil
}

func CopyFromRemote(src, dst string) error {

	// srcRef, err := name.ParseReference(src)
	// if err != nil {
	// 	return fmt.Errorf("unable to parse source image %s: %v", src, err)
	// }

	// dstRef, err := name.ParseReference(dst)
	// if err != nil {
	// 	return fmt.Errorf("unable to parse source image %s: %v", dst, err)
	// }

	// srcImg, err := remote.Image(srcRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	// if err != nil {
	// 	return fmt.Errorf("unable to get image from source ref %s: %v", srcRef, err)
	// }

	// err = remote.Write(dstRef, srcImg, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	// if err != nil {
	// 	return fmt.Errorf("unable to copy image from source ref %s to %s: %v", srcRef, dst, err)
	// }
	systemCtx := &types.SystemContext{}
	policy, _ := signature.DefaultPolicy(systemCtx)
	policyCtx, _ := signature.NewPolicyContext(policy)

	copyOptions := &copy.Options{
		ReportWriter:     os.Stdout,
		RemoveSignatures: true,
	}
	srcRef, err := alltransports.ParseImageName(src)
	if err != nil {
		return fmt.Errorf("unable to parse source image %s: %v", src, err)
	}
	dstRef, err := alltransports.ParseImageName(dst)
	if err != nil {
		return fmt.Errorf("unable to parse destination image %s: %v", dst, err)
	}
	manifest, err := copy.Image(context.Background(),
		policyCtx,
		dstRef,
		srcRef,
		copyOptions,
	)
	fmt.Printf("%s", manifest)
	return err
}
