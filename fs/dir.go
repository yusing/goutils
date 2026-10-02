package fs

import (
	"fmt"
	"os"
	"path"
)

// ListFiles recursively lists files up to maxDepth levels below dir.
// Results are paths joined to dir. If hideHidden is true, hidden entries are skipped at every level.
func ListFiles(dir string, maxDepth int, hideHidden ...bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error listing directory %s: %w", dir, err)
	}
	hideHiddenFiles := len(hideHidden) > 0 && hideHidden[0]
	files := make([]string, 0)
	for _, entry := range entries {
		if hideHiddenFiles && entry.Name()[0] == '.' {
			continue
		}
		if entry.IsDir() {
			if maxDepth <= 0 {
				continue
			}
			subEntries, err := ListFiles(path.Join(dir, entry.Name()), maxDepth-1, hideHiddenFiles)
			if err != nil {
				return nil, err
			}
			files = append(files, subEntries...)
		} else {
			files = append(files, path.Join(dir, entry.Name()))
		}
	}
	return files, nil
}
