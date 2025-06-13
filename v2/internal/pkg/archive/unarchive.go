package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/term"
)

type MirrorUnArchiver struct {
	UnArchiver
	workingDir   string
	cacheDir     string
	archiveFiles []string
}

func NewArchiveExtractor(archivePath, workingDir, cacheDir string) (MirrorUnArchiver, error) {
	ae := MirrorUnArchiver{
		workingDir: workingDir,
		cacheDir:   cacheDir,
	}
	files, err := os.ReadDir(archivePath)
	if err != nil {
		return MirrorUnArchiver{}, err
	}

	rxp, err := regexp.Compile(archiveFilePrefix + "_[0-9]{6}\\.tar")
	if err != nil {
		return MirrorUnArchiver{}, err
	}
	for _, chunk := range files {
		if rxp.MatchString(chunk.Name()) {
			ae.archiveFiles = append(ae.archiveFiles, filepath.Join(archivePath, chunk.Name()))
		}
	}
	return ae, nil
}

// Unarchive extracts:
// * docker/v2* to cacheDir
// * working-dir to workingDir
func (o MirrorUnArchiver) Unarchive() error {
	// make sure workingDir exists
	if err := os.MkdirAll(o.workingDir, 0755); err != nil {
		return fmt.Errorf("unable to create working dir %q: %w", o.workingDir, err)
	}
	// make sure cacheDir exists
	if err := os.MkdirAll(o.cacheDir, 0755); err != nil {
		return fmt.Errorf("unable to create cache dir %q: %w", o.cacheDir, err)
	}

	isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	p := mpb.New(mpb.PopCompletedMode())
	for i, chunkPath := range o.archiveFiles {
		if !isTerminal {
			// FIXME: replace this by a proper log call
			fmt.Printf("Extracting chunk file (%d / %d): %s\n", i+1, len(o.archiveFiles), chunkPath)
		}
		stat, _ := os.Stat(chunkPath)
		bar := p.AddBar(stat.Size(),
			mpb.PrependDecorators(
				decor.Name(chunkPath+" "),
				decor.Counters(decor.SizeB1024(0), "(% .1f / % .1f)"),
			),
			mpb.AppendDecorators(decor.Elapsed(decor.ET_STYLE_GO)),
		)

		if err := o.unarchiveChunkTarFile(chunkPath, bar); err != nil {
			bar.Abort(false)
			bar.Wait()
			return err
		}
		// Force completion since we can skip extracting some content from the archive
		bar.SetCurrent(stat.Size())
	}
	p.Wait()

	return nil
}

func (o MirrorUnArchiver) unarchiveChunkTarFile(chunkPath string, bar *mpb.Bar) error { //nolint:cyclop // cc of 11 is fine for this
	chunkFile, err := os.Open(chunkPath)
	if err != nil {
		return fmt.Errorf("unable to open chunk tar file: %w", err)
	}
	defer chunkFile.Close()
	workingDirParent := filepath.Dir(o.workingDir)
	reader := tar.NewReader(chunkFile)
	for {
		header, err := reader.Next()

		// break the infinite loop when EOF
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("error reading archive %s: %w", chunkFile.Name(), err)
		}

		if header == nil {
			continue
		}

		// taking only files into account because we are considering that all
		// parent folders will be created recursively, and that, to the best of
		// our knowledge the archive doesn't include any symbolic links
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// for the moment we ignore imageSetConfig that is included in the tar
		// as well as any other files that are not working-dir or cache

		parentDir := ""
		switch {
		// case file belongs to working-dir
		case strings.Contains(header.Name, workingDirectory):
			parentDir = workingDirParent
		// case file belongs to the cache
		case strings.Contains(header.Name, cacheFilePrefix):
			parentDir = o.cacheDir
		default:
			continue
		}

		// if it's a file create it
		// make sure it's at least writable and executable by the user
		// since with every UnArchive, we should be able to rewrite the file
		if err := createFileWithProgress(parentDir, header, reader, bar); err != nil {
			return err
		}
	}

	return nil
}

// see https://github.com/securego/gosec/issues/324#issuecomment-935927967
func sanitizeArchivePath(dir, filePath string) (string, error) {
	v := filepath.Join(dir, filePath)
	// OCPBUGS-57387: use absolute paths otherwise the `.` needs special
	// treatment because of the way Golang handles it after `Clean`
	absV, err := filepath.Abs(v)
	if err != nil {
		return "", fmt.Errorf("get absolute path for %q: %w", v, err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("get absolute path for %q: %w", dir, err)
	}
	if strings.HasPrefix(absV, absDir+string(os.PathSeparator)) {
		return v, nil
	}
	return "", fmt.Errorf("content filepath is tainted: %s", v)
}

func createFileWithProgress(parentDir string, header *tar.Header, reader *tar.Reader, bar *mpb.Bar) error {
	descriptor, err := sanitizeArchivePath(parentDir, header.Name)
	if err != nil {
		return err
	}
	proxyReader := bar.ProxyReader(reader)
	defer proxyReader.Close()
	return writeFile(descriptor, proxyReader, header.FileInfo().Mode()|0755)
}

func writeFile(filePath string, reader io.Reader, perm os.FileMode) error {
	// make sure all the parent directories exist
	descriptorParent := filepath.Dir(filePath)
	if err := os.MkdirAll(descriptorParent, 0755); err != nil {
		return fmt.Errorf("unable to create parent directory for %s: %w", filePath, err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, perm)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filePath, err)
	}
	defer f.Close()

	// copy  contents
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("error copying file %s: %w", filePath, err)
	}

	return nil
}
