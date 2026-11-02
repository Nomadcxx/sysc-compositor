package config

import (
	"crypto/rand"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sblinch/kdl-go"
)

//go:embed default-config.kdl
var DefaultConfigKDL []byte

// FirstRun describes the files written or left in place by first-start setup.
type FirstRun struct {
	Imported, Wrote                string
	Copied, Kept, Missing, Outside []string
}

// PrepareUserConfig imports once or writes the shipped default. Named configs
// and every existing user config, including dangling symlinks, stay untouched.
func PrepareUserConfig(p Path, getenv func(string) string, now time.Time) (FirstRun, error) {
	var result FirstRun
	target := UserPath(getenv)
	if p.Named != "" || target == "" {
		return result, nil
	}
	if _, err := os.Lstat(target); err == nil {
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	source := NiriUserPath(getenv)
	data, err := readImportFile(source)
	importing := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if !importing {
		data = DefaultConfigKDL
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(filepath.Dir(target))
	if err != nil {
		return result, err
	}
	defer root.Close()
	if _, err := root.Lstat("config.kdl"); err == nil {
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if importing {
		visited := map[string]bool{source: true}
		outside := make(map[string]bool)
		var copyIncludes func(string, []byte, int) error
		copyIncludes = func(from string, body []byte, depth int) error {
			// Treat bytes as opaque: a syntax error belongs to config loading, not
			// migration. A malformed document has no safely discoverable children.
			doc, err := kdl.Parse(strings.NewReader(string(body)))
			if err != nil {
				return nil
			}
			for _, node := range doc.Nodes {
				if nodeName(node) != "include" || len(node.Arguments) != 1 {
					continue
				}
				name, ok := node.Arguments[0].Value.(string)
				if !ok {
					continue
				}
				fromPath, err := resolveIncludePath(filepath.Dir(from), name)
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(filepath.Dir(source), fromPath)
				if err != nil {
					return err
				}
				if filepath.IsAbs(name) || name == "~" || strings.HasPrefix(name, "~/") || !filepath.IsLocal(relative) {
					if !outside[fromPath] {
						result.Outside = append(result.Outside, fromPath)
						outside[fromPath] = true
					}
					continue
				}
				if visited[fromPath] {
					continue
				}
				visited[fromPath] = true
				if depth >= maxIncludeDepth {
					return fmt.Errorf("config import: include depth exceeds %d", maxIncludeDepth)
				}
				dest := filepath.Join(filepath.Dir(target), relative)
				if _, err := root.Lstat(relative); err == nil {
					result.Kept = append(result.Kept, dest)
					continue
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
				included, err := readImportFile(fromPath)
				if errors.Is(err, os.ErrNotExist) {
					result.Missing = append(result.Missing, fromPath)
					continue
				}
				if err != nil {
					return err
				}
				if err := copyIncludes(fromPath, included, depth+1); err != nil {
					return err
				}
				if err := publishImport(root, relative, included); errors.Is(err, os.ErrExist) {
					result.Kept = append(result.Kept, dest)
				} else if err != nil {
					return err
				} else {
					result.Copied = append(result.Copied, dest)
				}
			}
			return nil
		}
		if err := copyIncludes(source, data, 0); err != nil {
			return result, err
		}
		headerPath := strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(source)
		data = append([]byte(fmt.Sprintf("// Imported from %s by sysc-compositor on %s.\n", headerPath, now.Format("2006-01-02"))), data...)
	}
	if err := publishImport(root, "config.kdl", data); errors.Is(err, os.ErrExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	result.Wrote = target
	if importing {
		result.Imported = source
	}
	return result, nil
}

// readImportFile refuses FIFOs and devices without blocking startup on open.
func readImportFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("config import: %s is not a regular file", path)
	}
	return io.ReadAll(f)
}

// publishImport publishes complete bytes without replacing any existing path.
// Root confines all relative include writes, including symlinked directories.
func publishImport(root *os.Root, name string, data []byte) error {
	if err := root.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(name), ".sysc-import-"+rand.Text())
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Link is an atomic no-replace publication; Rename would overwrite a file
	// created by the user or another first start after the existence check.
	return root.Link(temp, name)
}
