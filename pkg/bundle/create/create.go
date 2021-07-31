package create

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/archive"
	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(configPath, outputDir, rootDir string, segSize int64) error {

	var lastRun *v1alpha1.PastMirror
	var newRuns []v1alpha1.PastMirror

	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}

	// Open metadata for writing
	newMeta := v1alpha1.NewMetadata()

	// Read in current metadata
	metadata, err := config.LoadMetadata(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}

	newMeta.MetadataSpec = metadata.MetadataSpec

	if len(metadata.PastMirrors) != 0 {
		lastRun = &metadata.PastMirrors[len(metadata.PastMirrors)-1]
	} else {
		lastRun = &v1alpha1.PastMirror{
			Sequence: 0,
			Uid:      uuid.New(),
		}
	}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}

	if len(cfg.Mirror.OCP.Channels) != 0 {
		currentRun, err := bundle.GetReleases(lastRun, cfg, rootDir)

		newRuns = append(newRuns, *currentRun)
		logrus.Debug(newRuns)

		if err != nil {
			return err
		}
	}

	/*if &config.Mirror.Operators != nil {
	//GetOperators(*config, rootDir)
	//}
	//if &config.Mirror.Samples != nil {
	//GetSamples(*config, rootDir)
	//}*/

	if len(cfg.Mirror.AdditionalImages) != 0 {
		if err := bundle.GetAdditional(cfg, rootDir); err != nil {
			return err
		}
	}

	newMeta.PastMirrors = append(newMeta.PastMirrors, newRuns...)

	if err := config.WriteMetadata(newMeta, rootDir); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	if err != nil {
		return err
	}

	// Create archiver
	arc := archive.NewArchiver()

	// Set ouput directory so bundles do not get write to the rootDir
	if outputDir == "." {
		outputDir, err = os.Getwd()

		if err != nil {
			return err
		}
	}

	// Change dir before archving to avoid issues with symlink paths
	os.Chdir(rootDir)

	// Create tar archive
	if err := archive.CreateSplitArchive(arc, outputDir, "bundle", segSize, "."); err != nil {
		return fmt.Errorf("failed to create archive: %v", err)
	}

	return nil
}

// CreateDiff performs all tasks in creating differential imagesets
//func CreateDiff(rootDir string) error {
//    return err
//}

//func downloadObjects() {
//
//}
