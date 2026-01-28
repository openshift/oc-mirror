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

	tarutils "github.com/openshift/oc-mirror/v2/internal/pkg/archive/utils"
)

type MirrorUnArchiver struct {
	UnArchiver
	workingDir   string
	cacheDir     string
	archiveFiles []string
}

const mirrorTarRegex = archiveFilePrefix + "_[0-9]{6}\\.tar"

func NewArchiveExtractor(archivePath, workingDir, cacheDir string) (MirrorUnArchiver, error) {
	ae := MirrorUnArchiver{
		workingDir: workingDir,
		cacheDir:   cacheDir,
	}
	files, err := os.ReadDir(archivePath)
	if err != nil {
		return MirrorUnArchiver{}, err
	}

	rxp := regexp.MustCompile(mirrorTarRegex)
	for _, chunk := range files {
		if rxp.MatchString(chunk.Name()) {
			ae.archiveFiles = append(ae.archiveFiles, filepath.Join(archivePath, chunk.Name()))
		}
	}
	if len(ae.archiveFiles) == 0 {
		return MirrorUnArchiver{}, fmt.Errorf("no tar archives matching %q found in %q", mirrorTarRegex, archivePath)
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
		stat, err := os.Stat(chunkPath)
		if err != nil {
			return fmt.Errorf("failed to access %q: %w", chunkPath, err)
		}
		if stat.Size() == 0 {
			return fmt.Errorf("empty archive file %q", chunkPath)
		}
		if !isTerminal {
			// FIXME: replace this by a proper log call
			fmt.Printf("Extracting chunk file (%d / %d): %s\n", i+1, len(o.archiveFiles), chunkPath)
		}
		bar := p.AddBar(stat.Size(),
			mpb.PrependDecorators(
				decor.Name(chunkPath+" "),
				decor.Counters(decor.SizeB1024(0), "(% .1f / % .1f)"),
			),
			mpb.AppendDecorators(decor.Elapsed(decor.ET_STYLE_GO)),
		)
		bar.EnableTriggerComplete()

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

func createFileWithProgress(parentDir string, header *tar.Header, reader *tar.Reader, bar *mpb.Bar) error {
	descriptor, err := tarutils.SanitizeArchivePath(parentDir, header.Name)
	if err != nil {
		return err
	}
	proxyReader := bar.ProxyReader(reader)
	defer proxyReader.Close()
	return tarutils.WriteFile(descriptor, proxyReader, header.FileInfo().Mode(), header.Size)
}
