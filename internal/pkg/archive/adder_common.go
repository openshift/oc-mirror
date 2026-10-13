package archive

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
)

func addFileToWriter(fi fs.FileInfo, pathToFile, pathInTar string, tarWriter *tar.Writer) error {
	header, err := tar.FileInfoHeader(fi, fi.Name())
	if err != nil {
		return err
	}
	header.Name = pathInTar

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	// Open the file for reading
	file, err := os.Open(pathToFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the file contents to the tar archive
	if _, err := io.Copy(tarWriter, file); err != nil {
		return err
	}
	return nil
}
