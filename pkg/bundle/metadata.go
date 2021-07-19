package bundle

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
)

func ReadMeta(rootDir string) (Imagesets, error) {
	metaPath := rootDir + metadata
	if _, err := os.Stat(metaPath); err == nil {

		jsonFile, err := os.Open(metaPath)
		if err != nil {
			logrus.Errorln(err)
		}

		logrus.Infof("Successfully Opened %v", metaPath)
		// defer the closing of our jsonFile so that we can parse it later on
		defer jsonFile.Close()

		byteValue, _ := ioutil.ReadAll(jsonFile)

		var imagesets Imagesets

		json.Unmarshal(byteValue, &imagesets)
		return imagesets, err
	} else {
		empty := Imagesets{}
		return empty, err
	}
}

/*
func SetupMetadata(rootDir string) (string, error) {

	if _, err := os.Stat(filepath.Join(rootDir, "src/publish/.")); os.IsNotExist(err) {
		logrus.Infof("Metadata not found. Creating new metadata")
	} else {

		metafile, err := os.OpenFile(filepath.Join(rootDir, "src/publish/openshift_bundle.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			logrus.Fatal(errors.Wrap(err, "failed to open metadata file"))
		}

		return func() {
			metafile.Close()
		}
	}
}
*/
/*
func writeMeta() {
	// append current object to the end of the metadata file

}
*/
