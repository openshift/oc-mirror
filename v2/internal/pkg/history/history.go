package history

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

var log clog.PluggableLoggerInterface

type OSFileCreator struct{}
type history struct {
	historyDir  string
	before      time.Time
	fileCreator FileCreator
}

func NewHistory(workingDir string, before time.Time, logg clog.PluggableLoggerInterface, fileCreator FileCreator) (history, error) {
	if logg == nil {
		log = clog.New("error")
	} else {
		log = logg
	}
	historyDir := filepath.Join(workingDir, historyPath)

	err := os.MkdirAll(historyDir, 0766)
	if err != nil {
		return history{}, fmt.Errorf("error creating directories %w", err)
	}
	return history{
		historyDir:  historyDir,
		before:      before,
		fileCreator: fileCreator,
	}, nil
}

func (o history) Read() (map[string]struct{}, error) {
	historyMap := make(map[string]struct{})
	historyFile, err := o.getHistoryFile(o.before)
	// if err is of type EmptyHistoryError
	// then return the erorr and an empty historyMap
	if errors.Is(err, &EmptyHistoryError{}) {
		return historyMap, err
	} else if err != nil {
		return nil, err
	}

	file, err := os.Open(historyFile)
	if err != nil {
		return nil, fmt.Errorf("error opening a file %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		blob := scanner.Text()
		historyMap[blob] = struct{}{}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error non-EOF found %w", err)
	}

	return historyMap, nil
}

func (o history) getHistoryFile(before time.Time) (string, error) {
	var historyFilePath string
	historyFiles, err := os.ReadDir(o.historyDir)
	if err != nil {
		return "", fmt.Errorf("error reading a directory %w", err)
	}

	var latestFile fs.DirEntry
	var latestTime time.Time

	for _, historyFile := range historyFiles {
		if isHistoryFile(historyFile) {
			fileTime, err := getFileDate(historyFile)
			if err != nil {
				return "", err
			}

			if isLatestHistoryFile(fileTime, latestTime, before) {
				latestFile = historyFile
				latestTime = fileTime
			}
		}
	}
	if latestFile != nil {
		historyFilePath = filepath.Join(o.historyDir, latestFile.Name())
	} else {
		return "", EmptyHistoryErrorf("no history metadata found under %s", filepath.Dir(o.historyDir))
	}
	return historyFilePath, nil
}

func isHistoryFile(historyFile fs.DirEntry) bool {
	return !historyFile.IsDir() && strings.HasPrefix(historyFile.Name(), historyNamePrefix)
}

func isLatestHistoryFile(fileTime, latestTime, before time.Time) bool {
	if !before.IsZero() {
		if fileTime.After(latestTime) && fileTime.Before(before) {
			return true
		}
	} else if fileTime.After(latestTime) {
		return true
	}
	return false
}

func getFileDate(historyFile fs.DirEntry) (time.Time, error) {
	fileDate := strings.TrimPrefix(historyFile.Name(), historyNamePrefix)
	dateTime, err := time.Parse(time.RFC3339, fileDate)
	if err != nil {
		log.Error("unable to parse time from filename %s: %s", historyFile.Name(), err.Error())
		return time.Time{}, fmt.Errorf("error parsing time %w", err)
	}
	return dateTime, nil
}

func (o history) Append(blobsToAppend map[string]struct{}) (map[string]struct{}, error) {

	filename := o.newFileName()

	historyBlobs, err := o.Read()
	if err != nil && !errors.Is(err, &EmptyHistoryError{}) {
		return nil, err
	}

	for k := range blobsToAppend {
		historyBlobs[k] = struct{}{}
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
			return historyBlobs, fmt.Errorf("unable to write to history file: %w", err)
		}
	}

	err = writer.Flush()
	if err != nil {
		return historyBlobs, fmt.Errorf("unable to flush history file: %w", err)
	}

	return historyBlobs, nil

}

func (o history) newFileName() string {
	return filepath.Join(o.historyDir, historyNamePrefix+time.Now().UTC().Format(time.RFC3339))
}

func (OSFileCreator) Create(filename string) (io.WriteCloser, error) {
	file, err := os.Create(filename)
	if err != nil {
		return file, fmt.Errorf("error creating a file %w", err)
	}
	return file, nil
}
