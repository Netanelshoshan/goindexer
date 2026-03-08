package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("expandPath(~/foo/bar) = %q, want %q", got, want)
	}
	got = expandPath("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandPath(/absolute/path) = %q", got)
	}
}

func TestParseInt(t *testing.T) {
	n, err := parseInt("42")
	if err != nil || n != 42 {
		t.Errorf("parseInt(42) = %d, %v", n, err)
	}
	n, err = parseInt("  100  ")
	if err != nil || n != 100 {
		t.Errorf("parseInt('  100  ') = %d, %v", n, err)
	}
	_, err = parseInt("abc")
	if err == nil {
		t.Error("parseInt(abc) should error")
	}
}

func TestIndexExtensions(t *testing.T) {
	if !IndexExtensions[".go"] {
		t.Error(".go should be in IndexExtensions")
	}
	if !IndexExtensions[".swift"] {
		t.Error(".swift should be in IndexExtensions")
	}
	if IndexExtensions[".xyz"] {
		t.Error(".xyz should not be in IndexExtensions")
	}
}
