package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	imgmanifest "go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/transports/alltransports"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
)

func (o *ExecutorSchema) DryRun(ctx context.Context, allImages []v2alpha1.CopyImageSchema) error {
	// set up location of logs dir
	outDir := filepath.Join(o.Opts.Global.WorkingDir, dryRunOutDir)
	// clean up logs directory
	os.RemoveAll(outDir)

	// create logs directory
	err := o.MakeDir.makeDirAll(outDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}
	// creating file for storing list of cached images
	mappingTxtFilePath := filepath.Join(outDir, mappingFile)
	mappingTxtFile, err := os.Create(mappingTxtFilePath)
	if err != nil {
		return err
	}
	defer mappingTxtFile.Close()
	imagesAvailable := map[string]bool{}
	nbMissingImgs := 0
	var buff bytes.Buffer
	var missingImgsBuff bytes.Buffer

	for _, img := range allImages {
		buff.WriteString(img.Source + "=" + img.Destination + "\n")

		// Try to get manifest list sub-digests from source
		manifestDigests, err := o.getManifestListDigests(ctx, img.Source)
		if err != nil {
			o.Log.Debug("unable to get manifest list info for %s: %v", img.Source, err)
		} else if len(manifestDigests) > 0 {
			// This is a manifest list, write each sub-digest
			// Remove the digest suffix (if any) to get the base reference
			sourceBase, _, _ := strings.Cut(img.Source, "@")
			for _, digest := range manifestDigests {
				subSource := sourceBase + "@" + digest
				buff.WriteString(subSource + "=" + img.Destination + "\n")
			}
		}

		if o.Opts.IsMirrorToDisk() {
			exists, err := o.Mirror.Check(ctx, img.Destination, o.Opts, false)
			if err != nil {
				o.Log.Debug("unable to check existence of %s in local cache: %v", img.Destination, err)
			}
			if err != nil || !exists {
				missingImgsBuff.WriteString(img.Source + "=" + img.Destination + "\n")
				nbMissingImgs++
			}
		}
	}

	_, err = mappingTxtFile.Write(buff.Bytes())
	if err != nil {
		return err
	}
	if nbMissingImgs > 0 {
		// creating file for storing list of cached images
		missingImgsFilePath := filepath.Join(outDir, missingImgsFile)
		missingImgsTxtFile, err := os.Create(missingImgsFilePath)
		if err != nil {
			return err
		}
		defer missingImgsTxtFile.Close()
		_, err = missingImgsTxtFile.Write(missingImgsBuff.Bytes())
		if err != nil {
			return err
		}
		o.Log.Warn(emoji.Warning+"  %d/%d images necessary for mirroring are not available in the cache.", nbMissingImgs, len(allImages))
		o.Log.Warn("List of missing images in : %s.\nplease re-run the mirror to disk process", missingImgsFilePath)
	}

	if len(imagesAvailable) > 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", len(imagesAvailable))
	}
	o.Log.Info(emoji.PageFacingUp+" list of all images for mirroring in : %s", mappingTxtFilePath)
	return nil
}

// getManifestListDigests inspects the source image to check if it's a manifest list
// and returns the sub-manifest digests. Works with any transport supported by
// containers/image (docker://, oci:, etc.) via alltransports.ParseImageName.
// Returns a slice of digest strings (e.g., ["sha256:abc...", "sha256:def..."]) or nil if not a manifest list.
func (o *ExecutorSchema) getManifestListDigests(ctx context.Context, source string) ([]string, error) {
	srcRef, err := alltransports.ParseImageName(source)
	if err != nil {
		return nil, fmt.Errorf("error parsing image name %s: %w", source, err)
	}

	sysCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, fmt.Errorf("error creating system context: %w", err)
	}

	imgSrc, err := srcRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("error creating image source for %s: %w", source, err)
	}
	defer imgSrc.Close()

	manifestBytes, manifestType, err := imgSrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting manifest for %s: %w", source, err)
	}

	if !imgmanifest.MIMETypeIsMultiImage(manifestType) {
		return nil, nil
	}

	list, err := imgmanifest.ListFromBlob(manifestBytes, manifestType)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest list for %s: %w", source, err)
	}

	instances := list.Instances()
	digests := make([]string, 0, len(instances))
	for _, instance := range instances {
		digests = append(digests, instance.String())
	}
	return digests, nil
}
