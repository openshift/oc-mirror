package bundle

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
)

func readMeta(r string) (*Imagesets, error) {
	metaPath := r + metadata
	jsonFile, err := os.Open(metaPath)
	// if we os.Open returns an error then handle it
	if err != nil {
		logrus.Errorln(err)
	}

	logrus.Infoln("Successfully Opened %v", metaPath)
	// defer the closing of our jsonFile so that we can parse it later on
	defer jsonFile.Close()

	// read our opened xmlFile as a byte array.
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var imagesets Imagesets

	json.Unmarshal(byteValue, &imagesets)
	return &imagesets, err

}

/*
func writeMeta() {
	// append current object to the end of the metadata file

}
*/
