package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestStrictAdder_NextChunk(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	ma, err := newStrictAdder(defaultSegSize*segMultiplier, testFolder, clog.New("trace"))
	if err != nil {
		t.Fatal(err)
	}
	defer ma.close()
	defer os.RemoveAll(testFolder)

	err = ma.nextChunk()
	if err != nil {
		t.Fatalf("should not fail: %v", err)
	}
	assert.Equal(t, 2, ma.currentChunkId)
	assert.Equal(t, int64(0), ma.sizeOfCurrentChunk)
	assert.Equal(t, filepath.Join(testFolder, fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 2)), ma.archiveFile.Name())
}

func TestStrictAdder_AddFile_BiggerThanMax(t *testing.T) {
	t.Run("adding file exceeding maxSize: should fail", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		// use a maxArchiveSize of 10K
		ma, err := newStrictAdder(int64(10*1024), testFolder, clog.New("trace"))
		if err != nil {
			t.Fatal(err)
		}
		defer ma.close()
		defer os.RemoveAll(testFolder)

		//adding a file of 119K
		err = ma.addFile(consts.TestFolder+"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests/image-references", "file1")
		if err == nil {
			t.Fatal("should fail but passed instead")
		}
	})
	t.Run("adding files: should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		// use a maxArchiveSize of 10K
		ma, err := newStrictAdder(int64(10*1024), testFolder, clog.New("trace"))
		if err != nil {
			t.Fatal(err)
		}
		defer ma.close()
		defer os.RemoveAll(testFolder)

		// first archive
		firstArchive := ma.archiveFile.Name()
		//adding a first file of size 5KB
		err = ma.addFile(consts.TestFolder+"archive-test-data/0000_03_config-operator_01_proxy.crd.yaml", "file1")
		if err != nil {
			t.Fatalf("should not fail : %v", err)
		}
		// assert this is still in first chunk
		assert.Equal(t, 1, ma.currentChunkId)
		//adding a second file of size 2.3KB
		err = ma.addFile(consts.TestFolder+"archive-test-data/0000_03_securityinternal-openshift_02_rangeallocation.crd.yaml", "file2")
		if err != nil {
			t.Fatalf("should not fail : %v", err)
		}
		// assert this is still in first chunk
		assert.Equal(t, 1, ma.currentChunkId)

		//adding a third file 4.9KB
		err = ma.addFile(consts.TestFolder+"archive-test-data/0000_03_marketplace-operator_01_operatorhub.crd.yaml", "file3")
		if err != nil {
			t.Fatalf("should not fail : %v", err)
		}
		assert.Equal(t, 2, ma.currentChunkId)

		// assert that the first archive has been saved to disk
		assert.FileExists(t, firstArchive, "archive1 should exist")
		assertContents(t, firstArchive, []string{"file1", "file2"})
		// assert that the second archive is saved to disk
		assert.FileExists(t, ma.archiveFile.Name(), "archive2 should exist")
	})
}

func TestStrictAdder_AddFolder_BiggerThanMax(t *testing.T) {
	type testCase struct {
		caseName               string
		archiveSizeBytes       int64
		foldersToAdd           []string
		expectedNumberOfChunks int
		expectedError          string
	}

	testCases := []testCase{
		{
			caseName:               "File bigger than max: should fail",
			archiveSizeBytes:       int64(10 * 1024),
			foldersToAdd:           []string{consts.TestFolder + "working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests"},
			expectedNumberOfChunks: 1,
			expectedError:          "maxArchiveSize 0G is too small compared to sizes of files that need to be included in the archive.\nimage-references: 0G\n Aborting archive generation",
		},
		{
			caseName:               "nominal case: should pass",
			archiveSizeBytes:       int64(200 * 1024),
			foldersToAdd:           []string{consts.TestFolder + "working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests", consts.TestFolder + "working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests"},
			expectedNumberOfChunks: 2,
			expectedError:          "",
		},
	}
	for _, aTestCase := range testCases {
		t.Run(aTestCase.caseName, func(t *testing.T) {
			testFolder := t.TempDir()
			defer os.RemoveAll(testFolder)
			// use a maxArchiveSize of 10K
			ma, err := newStrictAdder(aTestCase.archiveSizeBytes, testFolder, clog.New("trace"))
			if err != nil {
				t.Fatal(err)
			}
			defer ma.close()
			defer os.RemoveAll(testFolder)

			errs := make([]error, len(aTestCase.foldersToAdd))
			for i, folder := range aTestCase.foldersToAdd {
				errs[i] = ma.addAllFolder(folder, filepath.Dir(folder))
			}
			if aTestCase.expectedError != "" {
				for _, err := range errs {
					if err != nil {
						assert.Equal(t, aTestCase.expectedError, err.Error())
					}
				}
			} else {
				for _, err := range errs {
					if err != nil {
						t.Fatalf("should not fail : %v", err)
					}
				}
			}
			assert.Equal(t, aTestCase.expectedNumberOfChunks, ma.currentChunkId)
			for i := 1; i <= aTestCase.expectedNumberOfChunks; i++ {
				assert.FileExists(t, filepath.Join(ma.destination, fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, i)))
			}
		})
	}
}
