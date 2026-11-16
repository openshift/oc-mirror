package origin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// placeholder matches the ${NAME} and ${NAME:ARGUMENT} placeholders replaced by RenderDir.
var placeholder = regexp.MustCompile(`\$\{([A-Z_]+)(?::([^}]+))?\}`)

// Resolver returns the value of the placeholder name, given its argument, which may be empty.
type Resolver func(name, argument string) (string, error)

// RenderDir copies srcDir to dstDir, replacing the ${NAME} and ${NAME:ARGUMENT} placeholders of
// every file with the value resolve returns for them. The test data uses them to refer to the
// origin registry and its images, whose address and digests change on every run. A placeholder
// that cannot be resolved is an error, so that a typo does not go unnoticed.
func RenderDir(srcDir, dstDir string, resolve Resolver) error {
	return filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var errs []error
		rendered := placeholder.ReplaceAllStringFunc(string(data), func(match string) string {
			groups := placeholder.FindStringSubmatch(match)
			value, err := resolve(groups[1], groups[2])
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", match, err))
				return match
			}
			return value
		})
		if len(errs) > 0 {
			return fmt.Errorf("failed to render %s: %w", path, errors.Join(errs...))
		}
		return os.WriteFile(target, []byte(rendered), info.Mode().Perm())
	})
}
