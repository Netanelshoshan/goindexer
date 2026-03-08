package indexer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/netanelshoshan/goindexer/internal/config"
)

func TestComputeDelta_EmptyManifest(t *testing.T) {
	scanned := []ScannedFile{
		{Path: "a.go", Hash: "h1"},
		{Path: "b.py", Hash: "h2"},
	}
	manifest := map[string]string{}
	delta := ComputeDelta(scanned, manifest, false)
	if len(delta.ToIndex) != 2 {
		t.Errorf("expected 2 to index, got %d", len(delta.ToIndex))
	}
	if len(delta.ToRemove) != 0 {
		t.Errorf("expected 0 to remove, got %d", len(delta.ToRemove))
	}
}

func TestComputeDelta_NoChanges(t *testing.T) {
	scanned := []ScannedFile{
		{Path: "a.go", Hash: "h1"},
	}
	manifest := map[string]string{"a.go": "h1"}
	delta := ComputeDelta(scanned, manifest, false)
	if len(delta.ToIndex) != 0 {
		t.Errorf("expected 0 to index, got %d", len(delta.ToIndex))
	}
	if len(delta.ToRemove) != 0 {
		t.Errorf("expected 0 to remove, got %d", len(delta.ToRemove))
	}
}

func TestComputeDelta_ModifiedAndRemoved(t *testing.T) {
	scanned := []ScannedFile{
		{Path: "a.go", Hash: "h1new"},
	}
	manifest := map[string]string{
		"a.go": "h1old",
		"b.py": "h2",
	}
	delta := ComputeDelta(scanned, manifest, false)
	if len(delta.ToIndex) != 1 || delta.ToIndex[0].Path != "a.go" {
		t.Errorf("expected 1 to index (a.go), got %v", delta.ToIndex)
	}
	if len(delta.ToRemove) != 1 || delta.ToRemove[0] != "b.py" {
		t.Errorf("expected 1 to remove (b.py), got %v", delta.ToRemove)
	}
}

func TestComputeDelta_Force(t *testing.T) {
	scanned := []ScannedFile{
		{Path: "a.go", Hash: "h1"},
	}
	manifest := map[string]string{"b.py": "h2"}
	delta := ComputeDelta(scanned, manifest, true)
	if len(delta.ToIndex) != 1 {
		t.Errorf("expected 1 to index, got %d", len(delta.ToIndex))
	}
	if len(delta.ToRemove) != 1 || delta.ToRemove[0] != "b.py" {
		t.Errorf("expected 1 to remove (b.py), got %v", delta.ToRemove)
	}
}

func TestMatchExclude(t *testing.T) {
	tests := []struct {
		name, pattern string
		want          bool
	}{
		{"node_modules", "node_modules", true},
		{"node_modules", ".git", false},
		{".git", ".git", true},
		{"foo.pyc", "*.pyc", true},
		{"foo.py", "*.pyc", false},
	}
	for _, tt := range tests {
		got := matchExclude(tt.name, tt.pattern)
		if got != tt.want {
			t.Errorf("matchExclude(%q, %q) = %v, want %v", tt.name, tt.pattern, got, tt.want)
		}
	}
}

func TestShouldIndex(t *testing.T) {
	cfg := config.Load()
	if !shouldIndex("a.go", cfg) {
		t.Error("expected .go to be indexed")
	}
	if !shouldIndex("b.swift", cfg) {
		t.Error("expected .swift to be indexed")
	}
	if shouldIndex("x.xyz", cfg) {
		t.Error("expected .xyz to not be indexed")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	manifest := map[string]string{"a.go": "h1", "b.py": "h2"}
	if err := SaveManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 entries, got %d", len(loaded))
	}
	if loaded["a.go"] != "h1" || loaded["b.py"] != "h2" {
		t.Errorf("loaded manifest mismatch: %v", loaded)
	}
}

func TestLoadManifest_NotExist(t *testing.T) {
	loaded, err := LoadManifest(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty manifest, got %d entries", len(loaded))
	}
}

func TestScanFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.py"), []byte("x=1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "x.js"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.MaxFileSize = 1024 * 1024
	scanned, err := ScanFiles(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// node_modules should be excluded
	if len(scanned) != 2 {
		t.Errorf("expected 2 files (a.go, b.py), got %d: %v", len(scanned), scanned)
	}
	for _, s := range scanned {
		if s.Hash == "" {
			t.Error("expected non-empty hash")
		}
	}
}

func TestManifestJSON(t *testing.T) {
	m := map[string]string{"a": "b"}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["a"] != "b" {
		t.Error("roundtrip failed")
	}
}
