package archive

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/mholt/archiver/v3"
	"github.com/sirupsen/logrus"
)

const delimeter = "\t"

// Define error for archive validation
type ValidationError struct {
	Path string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("calculating checksum for %v: did not match provided known checksum", e.Path)
}

// AppendChecksum will conca the checksum of the
//archive to the archive
func AppendChecksum(hashFile *os.File, archivePath string) error {

	// Check for the provided file
	file, err := os.Stat(archivePath)

	if err != nil {
		return err
	}

	// Calculate checksum for provided file
	sum, err := generateCheckSum(archivePath)
	if err != nil {
		return fmt.Errorf("error generating checksum for file %s: %v", hashFile.Name(), err)
	}

	// append checksum to file
	if _, err = hashFile.Write([]byte(sum + delimeter + file.Name() + "\n")); err != nil {
		return err
	}

	return nil
}

// VerifyArchive will verify the contents of the archive against the provided metadata file
func VerifyArchive(a Archiver, archivePath, hashPath string) error {

	checksums, err := MapChecksum(hashPath)

	for file, knownsum := range checksums {
		if strings.Contains(archivePath, file) {
			acutualsum, err := generateCheckSum(archivePath)

			if err != nil {
				return fmt.Errorf("error calculating hash value for provided archive %s: %v", archivePath, err)
			}
			if knownsum == acutualsum {
				logrus.Infof("Checksum validated for file %s", archivePath)
			} else {
				return &ValidationError{archivePath}
			}

		}
	}

	if err != nil {
		return fmt.Errorf("error create checksum map: %v", err)
	}

	logrus.Infof("Walking through provided archive %s", archivePath)

	a.Walk(archivePath, func(f archiver.File) error {
		fmt.Println("Filename:", f.Name())
		return nil
	})

	return nil
}

// MapChecksum will return a map with a filename and associated checksum value
func MapChecksum(src string) (map[string]string, error) {

	// Make checksum map with filename as the key and checksum as the value
	checksums := make(map[string]string)

	file, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("error opening file %s: %v", src, err)
	}

	defer file.Close()

	logrus.Infof("Scanning %s for hash values", src)

	// Create file scanner
	scanner := bufio.NewScanner(file)

	// Scan each line to create map
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), delimeter)
		checksums[line[1]] = line[0]

	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error %v", err)
	}

	return checksums, nil
}

// getHash is a helper function to get the checksum of a bundle
func generateCheckSum(fpath string) (string, error) {

	data, err := ioutil.ReadFile(fpath)

	return fmt.Sprintf("%x", sha256.Sum256(data)), err
}
