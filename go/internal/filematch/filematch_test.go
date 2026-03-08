package filematch

import (
	"testing"
)

func TestMatchFileFilter(t *testing.T) {
	tests := []struct {
		pattern  string
		filePath string
		want     bool
	}{
		// Glob patterns
		{"*.swift", "AppDelegate.swift", true},
		{"*.swift", "pkg/ui/Foo.swift", true},
		{"*.swift", "Foo.swift", true},
		{"*.swift", "bar.go", false},
		{"*.go", "main.go", true},
		{"*.py", "script.py", true},
		// **/*.ext (common from LLMs)
		{"**/*.swift", "AppDelegate.swift", true},
		{"**/*.swift", "deep/path/to/File.swift", true},
		{"**/*.swift", "bar.go", false},
		{"**/*.go", "cmd/main.go", true},
		// Dir/**/*.ext normalizes to *.ext (matches any path)
		{"src/**/*.ts", "src/utils/helper.ts", true},
		{"app/**/*.swift", "other/View.swift", true},
		// Regex-style from LLMs
		{".*", "anything.txt", true},
		{".", "x", true},
		{".*\\.swift", "AppDelegate.swift", true},
		{".*\\.swift", "Auth.swift", true},
		{".*\\.go", "main.go", true},
		// No match
		{"*.swift", "readme.md", false},
		{"*.py", "data.json", false},
	}
	for _, tt := range tests {
		got := MatchFileFilter(tt.pattern, tt.filePath)
		if got != tt.want {
			t.Errorf("MatchFileFilter(%q, %q) = %v, want %v", tt.pattern, tt.filePath, got, tt.want)
		}
	}
}
