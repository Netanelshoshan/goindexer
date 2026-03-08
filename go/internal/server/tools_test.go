package server

import (
	"testing"
)

func TestIsExcludedDir(t *testing.T) {
	tests := []struct {
		path, root string
		want      bool
	}{
		{"/proj/node_modules", "/proj", true},
		{"/proj/node_modules/foo", "/proj", true},
		{"/proj/.git", "/proj", true},
		{"/proj/src", "/proj", false},
		{"/proj/venv/lib", "/proj", true},
	}
	for _, tt := range tests {
		got := isExcludedDir(tt.path, tt.root)
		if got != tt.want {
			t.Errorf("isExcludedDir(%q, %q) = %v, want %v", tt.path, tt.root, got, tt.want)
		}
	}
}
