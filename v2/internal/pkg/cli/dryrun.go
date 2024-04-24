package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
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
		if o.Opts.IsMirrorToDisk() {
			exists, err := o.Mirror.Check(ctx, img.Destination, o.Opts)
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
		o.Log.Warn("⚠️  %d/%d images necessary for mirroring are not available in the cache.", nbMissingImgs, len(allImages))
		o.Log.Warn("List of missing images in : %s.\nplease re-run the mirror to disk process", missingImgsFilePath)
	}

	if len(imagesAvailable) > 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", len(imagesAvailable))
	}
	o.Log.Info("📄 list of all images for mirroring in : %s", mappingTxtFilePath)
	return nil
}
