package mirror

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

type AdditionalOptions struct {
	*MirrorOptions
}

func NewAdditionalOptions(mo *MirrorOptions) *AdditionalOptions {
	opts := &AdditionalOptions{MirrorOptions: mo}
	return opts
}

// Plan provides an image mapping with source and destination for provided AdditionalImages
func (o *AdditionalOptions) Plan(ctx context.Context, imageList []v1alpha2.Image, scn scenario) (image.TypedImageMapping, error) {
	mmappings := make(image.TypedImageMapping, len(imageList))
	// Instead of returning an error, just log it.
	isSkipErr := func(err error) bool {
		return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
	}
	for _, img := range imageList {
		theRef := img.Name
		if img.Source != "" {
			theRef = img.Source
		}
		if !image.IsDocker(theRef) && !image.IsOCI(theRef) && !image.IsFile(theRef) {
			img.Name = "docker://" + theRef
		}
		// Get source image information
		srcRef, err := image.ParseReference(theRef) //docker:// or oci://fbc
		if err != nil {
			if !isSkipErr(err) {
				return mmappings, fmt.Errorf("error parsing source image %s: %v", theRef, err)
			}
			klog.Warning(fmt.Errorf("error parsing source image %s: %v", theRef, err))
			continue
		}

		// if tag is empty set to latest
		if srcRef.Ref.Tag == "" {
			srcRef.Ref.Tag = "latest"
		}
		ref := theRef
		if image.IsDocker(theRef) && srcRef.Ref.Registry == "" {
			srcRef.Ref = srcRef.Ref.DockerClientDefaults() // set default registry to docker.io
			ref = srcRef.Ref.Exact()                       // image repo, the tag and the digest
		}
		if !image.IsImagePinned(ref) {
			digest := srcRef.Ref.ID
			if digest == "" {
				digest, err = getImgDigest(theRef)
				klog.Infof("digest is :%s", digest)
				if err != nil {
					if !isSkipErr(err) {
						return mmappings, err
					}
					klog.Warning(err)
					continue
				}
				srcRef.Ref.ID = digest
			}
		}

		// Temporary :if source is oci, set the src image type to file (oc doesnt understand otherwise)
		if srcRef.Type == image.DestinationOCI {
			srcRef.Type = imagesource.DestinationFile
		}
		// Set destination image information as file by default
		dstRef := srcRef
		if scn == MirrorToDiskScenario {
			dstRef.Type = imagesource.DestinationFile
			// The registry component is not included in the final path.
			dstRef.Ref.Registry = ""
		} else {

			dstRef, err = image.ParseReference(img.Name)

			if err != nil {
				if !isSkipErr(err) {
					return mmappings, fmt.Errorf("error parsing source image - setting dstRef %s: %v", img.Name, err)
				}
				klog.Warning(fmt.Errorf("error parsing source image - setting dstRef %s: %v", img.Name, err))
				continue
			}
			//make name just the name of the image (not full path of source)
			// this is used when creating the oc mapping object
			// and later used when copying to oc-mirror-workspace
			// srcRef.Ref.Name = dstRef.Ref.Name

			if dstRef.Ref.ID == "" {
				dstRef.Ref.ID = srcRef.Ref.ID
			}
			if dstRef.Ref.Tag == "" {
				dstRef.Ref.Tag = srcRef.Ref.Tag
			}
		}

		mmappings.Add(srcRef, dstRef, v1alpha2.TypeGeneric)
	}

	return mmappings, nil
}

func getImgDigest(imageName string) (string, error) {
	if strings.HasPrefix(imageName, "file") {
		imageName = strings.Replace(imageName, "file", "dir", 1) // containers/image uses dir:// as a prefix for on disk dockerv2 images. file:// not recognized
	}
	if !strings.Contains(imageName, "://") { //imageName doesnt start with the transport prefix
		//assume a remote image
		imageName = dockerProtocol + imageName
	}
	_, _, _, _, digest := parseImageName(imageName)
	if digest != "" {
		return digest, nil
	}
	imgRef, err := alltransports.ParseImageName(imageName)
	if err != nil {
		return "", fmt.Errorf("unable to parse reference %s: %v", imageName, err)

	}
	ctx := context.TODO()
	imgsrc, err := imgRef.NewImageSource(ctx, nil)
	defer func() {
		if imgsrc != nil {
			err = imgsrc.Close()
			if err != nil {
				klog.V(3).Infof("%s is not closed", imgsrc)
			}
		}
	}()
	if err != nil {
		return "", fmt.Errorf(" unable to create ImageSource for %s: %v", imageName, err)

	}
	manifestBlob, _, err := imgsrc.GetManifest(ctx, nil)
	if err != nil {
		return "", fmt.Errorf(" unable to get Manifest for %s: %v", imageName, err)

	}
	dgst, err := manifest.Digest(manifestBlob)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshall manifest of image : %w", err)
	} else {

		return dgst.String(), nil
	}

}

// Plan provides an image mapping with source and destination for provided AdditionalImages
func (o *AdditionalOptions) PlanToMirror(ctx context.Context, imageList []v1alpha2.Image, destRepo string, namespace string) (image.TypedImageMapping, error) {
	mmappings := make(image.TypedImageMapping, len(imageList))
	resolver, err := containerdregistry.NewResolver("", o.SourceSkipTLS, o.SourcePlainHTTP, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating image resolver: %v", err)
	}
	for _, img := range imageList {

		if img.Name == "" {
			klog.Warningf("invalid additional image %s: reference empty", img.Name)
			continue
		}
		img.Name = trimProtocol(img.Name)

		// Instead of returning an error, just log it.
		isSkipErr := func(err error) bool {
			return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
		}

		srcImage, err := image.ResolveToPin(ctx, resolver, img.Name)
		if err != nil {
			if !isSkipErr(err) {
				return mmappings, err
			}
			klog.Warning(err)
			continue
		}
		pinnedRef, err := imagesource.ParseReference(srcImage)
		if err != nil {
			return mmappings, fmt.Errorf("error parsing pinned image %s: %v", srcImage, err)
		}
		srcRef, err := imagesource.ParseReference(img.Name)
		if err != nil {
			return mmappings, fmt.Errorf("error parsing source image %s: %v", img.Name, err)
		}
		srcRef.Type = imagesource.DestinationFile
		// The registry component is not included in the final path.
		srcRef.Ref.Registry = ""
		srcRef.Ref.ID = pinnedRef.Ref.ID

		dstRef := pinnedRef
		dstRef.Ref.Registry = destRepo
		dstRef.Ref.Namespace = namespace
		dstRef.Ref.Tag = srcRef.Ref.Tag
		mmappings.Add(srcRef, dstRef, v1alpha2.TypeGeneric)
	}

	return mmappings, nil
}

/*ctx := context.TODO()
	imgsrc, err := regFuncs.newImageSource(ctx, nil, imgRef)
	defer func() {
		if imgsrc != nil {
			err = imgsrc.Close()
			if err != nil {
				klog.V(3).Infof("%s is not closed", imgsrc)
			}
		}
	}()
	if err != nil {
		finalError = fmt.Errorf("%w: unable to create ImageSource for %s: %v", finalError, mirroredImage, err)
		continue
	}
	_, _, err = regFuncs.getManifest(ctx, nil, imgsrc)
	if err != nil {
		finalError = fmt.Errorf("%w: unable to get Manifest for %s: %v", finalError, mirroredImage, err)
		continue
	} else {
		return trimProtocol(mirroredImage), nil
	}
}*/
