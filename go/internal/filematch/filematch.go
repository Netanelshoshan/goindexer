package filematch

import (
	"path/filepath"
	"regexp"
	"strings"
)

// MatchFileFilter returns true if filePath matches the given pattern.
// Supports: *.swift, **/*.swift, .*\.swift. Matches against the base filename only.
// Used by search_codebase (file_filter) and grep_search (file_pattern).
func MatchFileFilter(pattern string, filePath string) bool {
	base := filepath.Base(filePath)
	// Normalize **/*.ext to *.ext (common from LLMs)
	if idx := strings.LastIndex(pattern, "**/"); idx >= 0 {
		pattern = pattern[idx+3:]
	}
	return matchPattern(pattern, base)
}

func matchPattern(pattern, name string) bool {
	// Try glob first (e.g. *.swift, *.go)
	if matched, _ := filepath.Match(pattern, name); matched {
		return true
	}
	// Common regex-style patterns from LLMs
	if pattern == ".*" || pattern == "." {
		return true
	}
	// .*\.ext or .*.ext -> match files ending with .ext
	if strings.HasPrefix(pattern, ".*") {
		ext := strings.TrimPrefix(pattern, ".*")
		ext = strings.TrimPrefix(ext, ".")
		ext = strings.TrimPrefix(ext, "\\")
		if ext != "" && strings.HasSuffix(name, "."+ext) {
			return true
		}
	}
	// Try as regex on the base name (e.g. .*\.swift)
	if re, err := regexp.Compile(pattern); err == nil && re.MatchString(name) {
		return true
	}
	return false
}
