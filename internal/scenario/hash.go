package scenario

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// HashFiles hashes the named files and every regular file under the named
// directories, relative to root, in path order: each path and its content
// are folded into one SHA-256.
func HashFiles(root string, paths ...string) (string, error) {
	var files []string
	for _, p := range paths {
		err := filepath.WalkDir(filepath.Join(root, p), func(abs string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.Type().IsRegular() {
				rel, err := filepath.Rel(root, abs)
				if err != nil {
					return err
				}
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(files)
	h := sha256.New()
	for i, f := range files {
		if i > 0 && files[i-1] == f {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(b)
		h.Write([]byte(f))
		h.Write([]byte{0})
		h.Write(sum[:])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Hash is the set's content hash: every file under its root.
func (set *Set) Hash() (string, error) { return HashFiles(set.Root, ".") }

// Hash is a scenario's content hash: its file, its gold file, and its seed.
func (s *Scenario) Hash(root string) (string, error) {
	return HashFiles(root, s.File, s.GoldRaw, s.Seed)
}
