package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/netanelshoshan/goindexer/internal/config"
)

type ScannedFile struct {
	Path string
	Hash string
}

func ScanFiles(root string, cfg *config.Config) ([]ScannedFile, error) {
	var result []ScannedFile
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if isExcluded(path, rootAbs, cfg) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldIndex(path, cfg) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > cfg.MaxFileSize {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		h := sha256.Sum256(content)
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		result = append(result, ScannedFile{
			Path: rel,
			Hash: hex.EncodeToString(h[:]),
		})
		return nil
	})
	return result, err
}

func isExcluded(path, root string, cfg *config.Config) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		for _, ex := range config.DefaultExcludes {
			if matchExclude(p, ex) {
				return true
			}
		}
		for _, ex := range cfg.ExtraExcludes {
			if matchExclude(p, ex) {
				return true
			}
		}
	}
	return false
}

func matchExclude(name, pattern string) bool {
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "*") {
		return name == pattern
	}
	// Simple glob: *.pyc matches suffix
	if strings.HasPrefix(pattern, "*") && len(pattern) > 1 {
		return strings.HasSuffix(name, pattern[1:])
	}
	return false
}

func shouldIndex(path string, cfg *config.Config) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return config.IndexExtensions[ext]
}

type DeltaResult struct {
	ToIndex  []ScannedFile
	ToRemove []string
}

func ComputeDelta(scanned []ScannedFile, manifest map[string]string, force bool) DeltaResult {
	scannedMap := make(map[string]string)
	for _, s := range scanned {
		scannedMap[s.Path] = s.Hash
	}
	var toIndex []ScannedFile
	var toRemove []string
	if force {
		toIndex = scanned
		for path := range manifest {
			toRemove = append(toRemove, path)
		}
		return DeltaResult{ToIndex: toIndex, ToRemove: toRemove}
	}
	for _, s := range scanned {
		if old, ok := manifest[s.Path]; !ok || old != s.Hash {
			toIndex = append(toIndex, s)
		}
	}
	for path := range manifest {
		if _, ok := scannedMap[path]; !ok {
			toRemove = append(toRemove, path)
		}
	}
	return DeltaResult{ToIndex: toIndex, ToRemove: toRemove}
}

func LoadManifest(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = make(map[string]string)
	}
	return m, nil
}

func SaveManifest(path string, manifest map[string]string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
