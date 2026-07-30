package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

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
		return fmt.Errorf("failed to create mapping file: %w", err)
	}
	defer mappingTxtFile.Close()
	var buff bytes.Buffer
	var missingImgsBuff bytes.Buffer
	nbMissingImgs, nbAvailableImgs := o.processImages(ctx, allImages, &buff, &missingImgsBuff)

	_, err = mappingTxtFile.Write(buff.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write mapping file: %w", err)
	}
	if err := o.writeMissingImagesFile(outDir, &missingImgsBuff, nbMissingImgs, len(allImages)); err != nil {
		return err
	}

	if nbAvailableImgs > 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", nbAvailableImgs)
	}
	o.Log.Info(emoji.PageFacingUp+" list of all images for mirroring in : %s", mappingTxtFilePath)

	// Generate cluster resources in dry-run mode (skip for mirror-to-disk since target registry is unknown)
	if !o.Opts.IsMirrorToDisk() {
		if err := o.generateClusterResources(ctx, allImages); err != nil {
			// In dry-run mode, log cluster resource generation errors as warnings instead of failing
			o.Log.Warn("Cluster resources generation failed (dry-run mode): %v", err)
		}
	}

	return nil
}

// writeMissingImagesFile creates the missing.txt file if there are missing images
func (o *ExecutorSchema) writeMissingImagesFile(outDir string, missingImgsBuff *bytes.Buffer, nbMissingImgs, totalImgs int) error {
	if nbMissingImgs == 0 {
		return nil
	}

	missingImgsFilePath := filepath.Join(outDir, missingImgsFile)
	missingImgsTxtFile, err := os.Create(missingImgsFilePath)
	if err != nil {
		return fmt.Errorf("failed to create missing images file: %w", err)
	}
	defer missingImgsTxtFile.Close()

	_, err = missingImgsTxtFile.Write(missingImgsBuff.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write missing images file: %w", err)
	}

	o.Log.Warn(emoji.Warning+"  %d/%d images necessary for mirroring are not available in the cache.", nbMissingImgs, totalImgs)
	o.Log.Warn("List of missing images in : %s.\nplease re-run the mirror to disk process", missingImgsFilePath)
	return nil
}

// processImages processes all images and returns the count of missing and available images
func (o *ExecutorSchema) processImages(ctx context.Context, allImages []v2alpha1.CopyImageSchema, buff, missingImgsBuff *bytes.Buffer) (int, int) {
	nbMissingImgs := 0
	nbAvailableImgs := 0
	for _, img := range allImages {
		buff.WriteString(img.Source + "=" + img.Destination + "\n")
		if o.Opts.IsMirrorToDisk() {
			exists, err := o.Mirror.Check(ctx, img.Destination, o.Opts, false)
			if err != nil {
				o.Log.Debug("unable to check existence of %s in local cache: %v", img.Destination, err)
			}
			if err != nil || !exists {
				missingImgsBuff.WriteString(img.Source + "=" + img.Destination + "\n")
				nbMissingImgs++
			} else {
				nbAvailableImgs++
			}
		}
	}
	return nbMissingImgs, nbAvailableImgs
}
