package history

import (
	"bytes"
	"io"
	"testing"
	"time"

	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/stretchr/testify/assert"
)

type MockFileCreator struct {
	Buffer *bytes.Buffer
}

type nopCloser struct {
	io.Writer
}

func (m MockFileCreator) Create(name string) (io.WriteCloser, error) {
	m.Buffer = new(bytes.Buffer)
	return nopCloser{m.Buffer}, nil
}

func (nopCloser) Close() error { return nil }

func TestNewHistory(t *testing.T) {
	history, err := NewHistory(historyFakePath, time.Time{}, clog.New("trace"), MockFileCreator{})
	assert.NoError(t, err)
	assert.NotNil(t, history)
}

func TestRead(t *testing.T) {

	type testCase struct {
		caseName      string
		workingDir    string
		before        time.Time
		expectedError string
		expectedHist  map[string]string
	}

	testCases := []testCase{
		{
			caseName:      "valid history file - without specified time",
			workingDir:    historyFakePath,
			before:        time.Time{},
			expectedError: "",
			expectedHist: map[string]string{
				"sha256:1dddb0988d16": "",
				"sha256:3658954f1990": "",
				"sha256:e3dad360d035": "",
				"sha256:422e4fbe1ed8": "",
			},
		},
		{
			caseName:      "valid history file - with specified time",
			workingDir:    historyFakePath,
			before:        time.Date(2023, 11, 22, 0, 0, 0, 0, time.UTC),
			expectedError: "",
			expectedHist: map[string]string{
				"sha256:1dddb0988d16": "",
			},
		},
		{
			caseName:      "invalid working dir",
			workingDir:    "./invalid-workindir",
			before:        time.Time{},
			expectedError: "open ./invalid-workindir: no such file or directory",
			expectedHist:  nil,
		},
	}

	for _, test := range testCases {
		t.Run(test.caseName, func(t *testing.T) {
			history, err := NewHistory(test.workingDir, test.before, clog.New("trace"), MockFileCreator{})
			assert.NoError(t, err)

			historyMap, err := history.Read()
			if test.expectedError != "" {
				assert.EqualError(t, err, test.expectedError)
			}
			assert.Equal(t, test.expectedHist, historyMap)
		})
	}
}

func TestAppend(t *testing.T) {

	type testCase struct {
		caseName      string
		workingDir    string
		before        time.Time
		blobsToAppend map[string]string
		expectedError string
		expectedHist  map[string]string
	}

	testCases := []testCase{
		{
			caseName:   "valid history file - without specified time",
			workingDir: historyFakePath,
			before:     time.Time{},
			blobsToAppend: map[string]string{
				"sha256:20f695d2a913": "",
			},
			expectedError: "",
			expectedHist: map[string]string{
				"sha256:422e4fbe1ed8": "",
				"sha256:1dddb0988d16": "",
				"sha256:3658954f1990": "",
				"sha256:e3dad360d035": "",
				"sha256:20f695d2a913": "",
			},
		},
		{
			caseName:   "valid history file - with specified time",
			workingDir: historyFakePath,
			before:     time.Date(2023, 11, 22, 0, 0, 0, 0, time.UTC),
			blobsToAppend: map[string]string{
				"sha256:20f695d2a913": "",
			},
			expectedError: "",
			expectedHist: map[string]string{
				"sha256:1dddb0988d16": "",
				"sha256:20f695d2a913": "",
			},
		},
		{
			caseName:      "invalid working dir",
			workingDir:    "./invalid-workindir",
			before:        time.Time{},
			expectedError: "open ./invalid-workindir: no such file or directory",
			expectedHist:  nil,
		},
	}

	for _, test := range testCases {
		t.Run(test.caseName, func(t *testing.T) {
			history, err := NewHistory(test.workingDir, test.before, clog.New("trace"), MockFileCreator{})
			assert.NoError(t, err)
			historyBlobs, err := history.Append(test.blobsToAppend)

			if test.expectedError != "" {
				assert.EqualError(t, err, test.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedHist, historyBlobs)
			}
		})
	}

}
