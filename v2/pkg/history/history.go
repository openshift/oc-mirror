package history

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	clog "github.com/openshift/oc-mirror/v2/pkg/log"
)

var log clog.PluggableLoggerInterface

type OSFileCreator struct{}
type history struct {
	historyDir  string
	before      time.Time
	fileCreator FileCreator
}

// TODO : @aguidirh remove fileCreator from arguments
func NewHistory(workingDir string, before time.Time, logg clog.PluggableLoggerInterface, fileCreator FileCreator) (History, error) {
	if logg == nil {
		log = clog.New("error")
	} else {
		log = logg
	}
	historyDir := filepath.Join(workingDir, historyPath)

	err := os.MkdirAll(historyDir, 0755)
	if err != nil {
		return history{}, err
	}
	return history{
		historyDir:  historyDir,
		before:      before,
		fileCreator: fileCreator,
	}, nil
}

func (o history) Read() (map[string]string, error) {
	historyMap := make(map[string]string)
	historyFile, err := o.getHistoryFile(o.before)
	//if err is of type EmptyHistoryError
	// then return the erorr and an empty historyMap
	if errors.Is(err, &EmptyHistoryError{}) {
		return historyMap, err
	} else if err != nil {
		return nil, err
	}

	file, err := os.Open(historyFile)
	if err != nil {
		log.Error("unable to open history file: %s", err.Error())
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		blob := scanner.Text()
		historyMap[blob] = ""
	}

	if err := scanner.Err(); err != nil {
		log.Error("unable to read history file: %s", err.Error())
		return nil, err
	}

	return historyMap, nil
}

func (o history) getHistoryFile(before time.Time) (string, error) {
	historyFilePath := ""
	historyFiles, err := os.ReadDir(o.historyDir)
	if err != nil {
		log.Error("unable to read history directory: %s", err.Error())
		return "", err
	}

	var latestFile fs.DirEntry
	var latestTime time.Time

	for _, historyFile := range historyFiles {
		if isHistoryFile(historyFile) {
			fileTime, err := getFileDate(historyFile)
			if err != nil {
				return "", err
			}

			if !before.IsZero() {
				if fileTime.After(latestTime) && fileTime.Before(before) {
					latestFile = historyFile
					latestTime = fileTime
				}
			} else {
				if fileTime.After(latestTime) {
					latestFile = historyFile
					latestTime = fileTime
				}
			}
		}
	}
	if latestFile != nil {
		historyFilePath = filepath.Join(o.historyDir, latestFile.Name())
	} else {
		return "", EmptyHistoryErrorf("no history metadata found under %s", filepath.Dir(o.historyDir))
	}
	return historyFilePath, err
}

func isHistoryFile(historyFile fs.DirEntry) bool {
	return !historyFile.IsDir() && strings.HasPrefix(historyFile.Name(), historyNamePrefix)
}

func getFileDate(historyFile fs.DirEntry) (time.Time, error) {
	fileDate := strings.TrimPrefix(historyFile.Name(), historyNamePrefix)
	dateTime, err := time.Parse(time.RFC3339, fileDate)
	if err != nil {
		log.Error("unable to parse time from filename %s: %s", historyFile.Name(), err.Error())
		return time.Time{}, err
	}
	return dateTime, err
}

func (o history) Append(blobsToAppend map[string]string) (map[string]string, error) {

	filename := o.newFileName()

	historyBlobs, err := o.Read()
	if err != nil && !errors.Is(err, &EmptyHistoryError{}) {
		return nil, err
	}

	for k, v := range blobsToAppend {
		historyBlobs[k] = v
	}

	file, err := o.fileCreator.Create(filename)
	if err != nil {
		return historyBlobs, err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for blob := range historyBlobs {
		_, err := writer.WriteString(blob + "\n")
		if err != nil {
			log.Error("unable to write to history file: %s", err.Error())
			return historyBlobs, err
		}
	}

	err = writer.Flush()
	if err != nil {
		log.Error("unable to flush history file: %s", err.Error())
		return historyBlobs, err
	}

	return historyBlobs, err

}

func (o history) newFileName() string {
	return filepath.Join(o.historyDir, historyNamePrefix+time.Now().UTC().Format(time.RFC3339))
}

func (OSFileCreator) Create(filename string) (io.WriteCloser, error) {
	file, err := os.Create(filename)
	if err != nil {
		log.Error("unable to create file: %s", err.Error())
	}
	return file, err
}
