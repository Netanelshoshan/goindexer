package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/netanelshoshan/goindexer/internal/config"
	"github.com/netanelshoshan/goindexer/internal/filematch"
	"github.com/netanelshoshan/goindexer/internal/indexer"
	"github.com/netanelshoshan/goindexer/internal/storage"
)

type indexStatus string

const (
	indexStatusIdle      indexStatus = "idle"
	indexStatusRunning   indexStatus = "running"
	indexStatusCompleted indexStatus = "completed"
	indexStatusFailed    indexStatus = "failed"
)

type IndexState struct {
	mu             sync.RWMutex
	Status         indexStatus
	Indexed        int
	Total          int
	IndexedThisRun int
	ToIndexThisRun int
	RootPath       string
	ErrMsg         string
}

func (s *IndexState) setRunning(root string, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = indexStatusRunning
	s.Indexed = 0
	s.Total = total
	s.IndexedThisRun = 0
	s.ToIndexThisRun = 0
	s.RootPath = root
	s.ErrMsg = ""
}

func (s *IndexState) setProgress(indexed, total, indexedThisRun, toIndexThisRun int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Indexed = indexed
	s.Total = total
	s.IndexedThisRun = indexedThisRun
	s.ToIndexThisRun = toIndexThisRun
}

func (s *IndexState) setCompleted(indexed, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = indexStatusCompleted
	s.Indexed = indexed
	s.Total = total
	s.IndexedThisRun = 0
	s.ToIndexThisRun = 0
}

func (s *IndexState) setFailed(errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = indexStatusFailed
	s.ErrMsg = errMsg
}

func (s *IndexState) setIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = indexStatusIdle
	s.Indexed = 0
	s.Total = 0
	s.IndexedThisRun = 0
	s.ToIndexThisRun = 0
	s.RootPath = ""
	s.ErrMsg = ""
}

func (s *IndexState) get() (status indexStatus, indexed, total, indexedThisRun, toIndexThisRun int, rootPath, errMsg string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status, s.Indexed, s.Total, s.IndexedThisRun, s.ToIndexThisRun, s.RootPath, s.ErrMsg
}

func (s *IndexState) isRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status == indexStatusRunning
}

// ShouldReindexOnChange returns true when indexing has been requested (index_codebase or --index).
// Used by the file watcher to only auto re-index after the user has requested indexing.
func (h *ToolHandlers) ShouldReindexOnChange() bool {
	h.indexState.mu.RLock()
	status := h.indexState.Status
	h.indexState.mu.RUnlock()
	if status == indexStatusCompleted || status == indexStatusFailed {
		return true
	}
	h.cfgMu.RLock()
	rootPath := h.cfg.RootPath
	h.cfgMu.RUnlock()
	_, err := os.Stat(rootPath)
	return err == nil
}

type ToolHandlers struct {
	cfgMu       sync.RWMutex
	cfg         *config.Config
	st          *storage.Storage
	emb         *indexer.Embedder
	indexState  *IndexState
	startupPath string
}

func NewToolHandlers(cfg *config.Config, startupPath string) (*ToolHandlers, error) {
	st, err := storage.New(cfg)
	if err != nil {
		return nil, err
	}
	return &ToolHandlers{
		cfg:         cfg,
		st:          st,
		emb:         indexer.NewEmbedder(cfg),
		indexState:  &IndexState{Status: indexStatusIdle},
		startupPath: startupPath,
	}, nil
}

// switchToWorkspace closes the current storage and switches to the config for root.
// Call after indexing a path so search/read use the correct index.
func (h *ToolHandlers) switchToWorkspace(root string) error {
	newCfg := config.ForWorkspace(root)
	newSt, err := storage.New(newCfg)
	if err != nil {
		return err
	}
	h.cfgMu.Lock()
	oldSt := h.st
	h.cfg = newCfg
	h.st = newSt
	h.cfgMu.Unlock()
	return oldSt.Close()
}

func (h *ToolHandlers) Close() error {
	h.cfgMu.RLock()
	st := h.st
	h.cfgMu.RUnlock()
	return st.Close()
}

func (h *ToolHandlers) getIndexRoot() (string, error) {
	h.cfgMu.RLock()
	rootPath := h.cfg.RootPath
	h.cfgMu.RUnlock()
	data, err := os.ReadFile(rootPath)
	if err != nil {
		return "", fmt.Errorf("no index root: call index_codebase first to index the codebase")
	}
	return strings.TrimSpace(string(data)), nil
}

func (h *ToolHandlers) resolvePath(rel string) (string, error) {
	root, err := h.getIndexRoot()
	if err != nil {
		return "", err
	}
	rootAbs, _ := filepath.Abs(root)
	rel = filepath.FromSlash(strings.TrimSpace(rel))
	var full string
	if filepath.IsAbs(rel) {
		full = filepath.Clean(rel)
	} else {
		full = filepath.Clean(filepath.Join(rootAbs, rel))
	}
	// Ensure path is under root (prevent /a/bc when root is /a/b)
	rootWithSep := rootAbs + string(filepath.Separator)
	if full != rootAbs && !strings.HasPrefix(full, rootWithSep) {
		return "", fmt.Errorf("path traversal not allowed")
	}
	return full, nil
}

type SearchCodebaseInput struct {
	Query      string `json:"query"`
	TopK       int    `json:"top_k"`
	FileFilter string `json:"file_filter"`
}

type TextOutput struct {
	Text string `json:"text"`
}

func (h *ToolHandlers) HandleSearchCodebase(ctx context.Context, req *mcp.CallToolRequest, input SearchCodebaseInput) (*mcp.CallToolResult, TextOutput, error) {
	if input.Query == "" {
		return nil, TextOutput{}, fmt.Errorf("query is required")
	}
	if input.TopK <= 0 {
		input.TopK = 10
	}
	embedding, err := h.emb.EmbedQuery(input.Query)
	if err != nil {
		return nil, TextOutput{}, err
	}
	h.cfgMu.RLock()
	results, err := h.st.Search(embedding, input.TopK, input.FileFilter)
	h.cfgMu.RUnlock()
	if err != nil {
		return nil, TextOutput{}, err
	}
	var b strings.Builder
	for _, r := range results {
		symbol := r.SymbolName
		if symbol == "" {
			symbol = "?"
		}
		fmt.Fprintf(&b, "--- %s:%d-%d (%s) ---\n%s\n\n", r.FilePath, r.StartLine, r.EndLine, symbol, r.Text)
	}
	return nil, TextOutput{Text: b.String()}, nil
}

type FindReferencesInput struct {
	SymbolName string `json:"symbol_name"`
	FilePath   string `json:"file_path"`
	LineNumber int    `json:"line_number"`
}

func (h *ToolHandlers) HandleFindReferences(ctx context.Context, req *mcp.CallToolRequest, input FindReferencesInput) (*mcp.CallToolResult, TextOutput, error) {
	if input.SymbolName == "" {
		return nil, TextOutput{}, fmt.Errorf("symbol_name is required")
	}
	h.cfgMu.RLock()
	refs, err := h.st.FindReferences(input.SymbolName, input.FilePath, input.LineNumber)
	h.cfgMu.RUnlock()
	if err != nil {
		return nil, TextOutput{}, err
	}
	var b strings.Builder
	for _, r := range refs {
		fmt.Fprintf(&b, "%s:%d", r.FilePath, r.LineNumber)
		if r.ParentScope != "" {
			fmt.Fprintf(&b, " (scope: %s)", r.ParentScope)
		}
		if r.Text != "" {
			lines := strings.Split(r.Text, "\n")
			preview := r.Text
			if len(lines) > 3 {
				preview = strings.Join(lines[:3], "\n") + "\n..."
			}
			fmt.Fprintf(&b, "\n%s\n", preview)
		}
		b.WriteString("\n")
	}
	return nil, TextOutput{Text: strings.TrimSpace(b.String())}, nil
}

type GrepSearchInput struct {
	Pattern     string `json:"pattern"`
	Path        string `json:"path"`
	FilePattern string `json:"file_pattern"`
}

func (h *ToolHandlers) HandleGrepSearch(ctx context.Context, req *mcp.CallToolRequest, input GrepSearchInput) (*mcp.CallToolResult, TextOutput, error) {
	if input.Pattern == "" {
		return nil, TextOutput{}, fmt.Errorf("pattern is required")
	}
	re, err := regexp.Compile(input.Pattern)
	if err != nil {
		return nil, TextOutput{}, fmt.Errorf("invalid regex: %w", err)
	}
	root, err := h.getIndexRoot()
	if err != nil {
		return nil, TextOutput{}, err
	}
	rootAbs, _ := filepath.Abs(root)
	searchRoot := rootAbs
	if input.Path != "" {
		resolved, err := h.resolvePath(input.Path)
		if err != nil {
			return nil, TextOutput{}, err
		}
		searchRoot = resolved
	}
	var matches []string
	count := 0
	err = filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if isExcludedDir(path, rootAbs) {
				return filepath.SkipDir
			}
			return nil
		}
		if input.FilePattern != "" {
			if !filematch.MatchFileFilter(input.FilePattern, path) {
				return nil
			}
		}
		if !config.IndexExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(content), "\n")
		rel, _ := filepath.Rel(rootAbs, path)
		rel = filepath.ToSlash(rel)
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, i+1, line))
				count++
				if count >= config.GrepMatchLimit {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return nil, TextOutput{}, err
	}
	return nil, TextOutput{Text: strings.Join(matches, "\n")}, nil
}

func isExcludedDir(path, root string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		for _, ex := range config.DefaultExcludes {
			if !strings.Contains(ex, "*") && p == ex {
				return true
			}
		}
	}
	return false
}

type ReadFileContextInput struct {
	Path      string      `json:"path"`
	StartLine json.Number `json:"start_line"`
	EndLine   json.Number `json:"end_line"`
}

func (h *ToolHandlers) HandleReadFileContext(ctx context.Context, req *mcp.CallToolRequest, input ReadFileContextInput) (*mcp.CallToolResult, TextOutput, error) {
	if input.Path == "" {
		return nil, TextOutput{}, fmt.Errorf("path is required")
	}
	fullPath, err := h.resolvePath(input.Path)
	if err != nil {
		return nil, TextOutput{}, err
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, TextOutput{}, err
	}
	if len(content) > config.ReadFileLimit {
		content = content[:config.ReadFileLimit]
	}
	lines := strings.Split(string(content), "\n")
	start, end := 1, len(lines)
	if input.StartLine != "" {
		n, err := input.StartLine.Int64()
		if err != nil {
			return nil, TextOutput{}, fmt.Errorf("invalid start_line: %w", err)
		}
		if n > 0 {
			start = int(n)
		}
	}
	if input.EndLine != "" {
		n, err := input.EndLine.Int64()
		if err != nil {
			return nil, TextOutput{}, fmt.Errorf("invalid end_line: %w", err)
		}
		if n > 0 {
			end = int(n)
		}
	}
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		start, end = end, start
	}
	selected := lines[start-1 : end]
	return nil, TextOutput{Text: strings.Join(selected, "\n")}, nil
}

type ListStructureInput struct {
	Path     string `json:"path"`
	Mode     string `json:"mode"`
	MaxDepth int    `json:"max_depth"`
}

func (h *ToolHandlers) HandleListStructure(ctx context.Context, req *mcp.CallToolRequest, input ListStructureInput) (*mcp.CallToolResult, TextOutput, error) {
	if input.Mode == "" {
		input.Mode = "tree"
	}
	if input.MaxDepth <= 0 {
		input.MaxDepth = 4
	}
	root, err := h.getIndexRoot()
	if err != nil {
		return nil, TextOutput{}, err
	}
	rootAbs, _ := filepath.Abs(root)
	base := rootAbs
	if input.Path != "" {
		resolved, err := h.resolvePath(input.Path)
		if err != nil {
			return nil, TextOutput{}, err
		}
		base = resolved
	}
	if input.Mode == "symbols" {
		prefix := input.Path
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		h.cfgMu.RLock()
		results, err := h.st.ListByPathPrefix(prefix)
		h.cfgMu.RUnlock()
		if err != nil {
			return nil, TextOutput{}, err
		}
		var b strings.Builder
		for _, r := range results {
			fmt.Fprintf(&b, "%s:%d-%d %s %s\n", r.FilePath, r.StartLine, r.EndLine, r.SymbolType, r.SymbolName)
		}
		return nil, TextOutput{Text: b.String()}, nil
	}
	var b strings.Builder
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, path)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > input.MaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if isExcludedDir(path, rootAbs) && d.IsDir() {
			return filepath.SkipDir
		}
		name := filepath.Base(path)
		if d.IsDir() {
			name += "/"
		}
		indent := strings.Repeat("  ", depth-1)
		b.WriteString(indent + "├── " + name + "\n")
		return nil
	})
	if err != nil {
		return nil, TextOutput{}, fmt.Errorf("list_structure: %w", err)
	}
	return nil, TextOutput{Text: b.String()}, nil
}

type IndexCodebaseInput struct {
	Path string `json:"path"`
}

func (h *ToolHandlers) HandleIndexCodebase(ctx context.Context, req *mcp.CallToolRequest, input IndexCodebaseInput) (*mcp.CallToolResult, TextOutput, error) {
	var root string
	if input.Path != "" {
		abs, err := filepath.Abs(strings.TrimSpace(input.Path))
		if err != nil {
			return nil, TextOutput{}, fmt.Errorf("invalid path: %w", err)
		}
		root = abs
	} else if h.startupPath != "" {
		abs, err := filepath.Abs(h.startupPath)
		if err != nil {
			return nil, TextOutput{}, fmt.Errorf("invalid startup path: %w", err)
		}
		root = abs
	} else {
		h.cfgMu.RLock()
		rootPath := h.cfg.RootPath
		h.cfgMu.RUnlock()
		data, err := os.ReadFile(rootPath)
		if err != nil {
			return nil, TextOutput{}, fmt.Errorf("path is required for first index: provide path param or run server with --path")
		}
		root = strings.TrimSpace(string(data))
	}

	if h.indexState.isRunning() {
		return nil, TextOutput{Text: "indexing already in progress"}, nil
	}

	// Always use workspace-local config: DB, manifest, root path under root/.goindexer/
	workspaceCfg := config.ForWorkspace(root)
	rootDir := filepath.Dir(workspaceCfg.RootPath)
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, TextOutput{}, err
	}
	if err := os.WriteFile(workspaceCfg.RootPath, []byte(root), 0644); err != nil {
		return nil, TextOutput{}, err
	}

	h.indexState.setRunning(root, 0)
	go func() {
		onProgress := func(indexed, total, indexedThisRun, toIndexThisRun int) {
			h.indexState.setProgress(indexed, total, indexedThisRun, toIndexThisRun)
		}
		_, total, err := indexer.RunIndex(root, false, workspaceCfg, onProgress)
		if err != nil {
			h.indexState.setFailed(err.Error())
			return
		}
		if err := h.switchToWorkspace(root); err != nil {
			log.Printf("switch to workspace: %v", err)
		}
		h.indexState.setCompleted(total, total)
	}()

	return nil, TextOutput{Text: fmt.Sprintf("indexing started for %s", root)}, nil
}

// discoverWorkspaceFromCwd walks up from cwd looking for .goindexer. Used when server started with wrong workspace.
func discoverWorkspaceFromCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		marker := filepath.Join(dir, ".goindexer", "index_root.txt")
		if _, err := os.Stat(marker); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// tryLoadWorkspace attempts to load an existing index at root. If valid, switches to it. Returns (loaded, count).
func (h *ToolHandlers) tryLoadWorkspace(root string) (bool, int) {
	workspaceCfg := config.ForWorkspace(root)
	if _, err := os.Stat(workspaceCfg.RootPath); err != nil {
		return false, 0
	}
	data, err := os.ReadFile(workspaceCfg.RootPath)
	if err != nil {
		return false, 0
	}
	storedRoot := strings.TrimSpace(string(data))
	if storedRoot != "" {
		root = storedRoot
		workspaceCfg = config.ForWorkspace(root)
	}
	count := 0
	if manifest, err := indexer.LoadManifest(workspaceCfg.ManifestPath); err == nil && len(manifest) > 0 {
		count = len(manifest)
	} else if newSt, err := storage.New(workspaceCfg); err == nil {
		if c, err := newSt.CountIndexedFiles(); err == nil && c > 0 {
			count = c
		}
		newSt.Close()
	}
	if count == 0 {
		return false, 0
	}
	if err := h.switchToWorkspace(root); err != nil {
		log.Printf("switch to workspace %s: %v", root, err)
		return false, 0
	}
	return true, count
}

type GetIndexStatusInput struct {
	Path string `json:"path"` // Optional: workspace path to load (e.g. when server started with wrong config)
}

type DeleteIndexInput struct{}

func (h *ToolHandlers) HandleDeleteIndex(ctx context.Context, req *mcp.CallToolRequest, input DeleteIndexInput) (*mcp.CallToolResult, TextOutput, error) {
	if h.indexState.isRunning() {
		return nil, TextOutput{Text: "cannot delete index while indexing is in progress"}, nil
	}
	h.cfgMu.RLock()
	cfg := h.cfg
	st := h.st
	h.cfgMu.RUnlock()
	if err := st.ClearAll(); err != nil {
		return nil, TextOutput{}, fmt.Errorf("clear storage: %w", err)
	}
	if err := indexer.SaveManifest(cfg.ManifestPath, map[string]string{}); err != nil {
		return nil, TextOutput{}, fmt.Errorf("clear manifest: %w", err)
	}
	_ = os.Remove(cfg.RootPath)
	h.indexState.setIdle()
	return nil, TextOutput{Text: "index deleted"}, nil
}

func (h *ToolHandlers) HandleGetIndexStatus(ctx context.Context, req *mcp.CallToolRequest, input GetIndexStatusInput) (*mcp.CallToolResult, TextOutput, error) {
	status, indexed, total, indexedThisRun, toIndexThisRun, rootPath, errMsg := h.indexState.get()

	// When idle, try to restore status from persistent index (manifest or DB) so we show meaningful data after process restart
	if status == indexStatusIdle {
		tryRoot := input.Path
		if tryRoot == "" {
			h.cfgMu.RLock()
			rootPathFile := h.cfg.RootPath
			h.cfgMu.RUnlock()
			if data, err := os.ReadFile(rootPathFile); err == nil {
				tryRoot = strings.TrimSpace(string(data))
			}
		}
		if tryRoot == "" {
			// Current config has no index; discover from cwd (Gemini may not pass workspacePath)
			tryRoot = discoverWorkspaceFromCwd()
		}
		if tryRoot != "" {
			absRoot, _ := filepath.Abs(strings.TrimSpace(tryRoot))
			if absRoot != "" {
				tryRoot = absRoot
			}
			if loaded, n := h.tryLoadWorkspace(tryRoot); loaded {
				status = indexStatusCompleted
				indexed = n
				total = n
				rootPath = tryRoot
			}
		}
	}

	percent := 0
	if total > 0 {
		percent = indexed * 100 / total
	}
	out := map[string]interface{}{
		"status":         string(status),
		"indexed":        indexed,
		"total":          total,
		"total_in_index": indexed,
		"total_scanned":  total,
		"percent":        percent,
		"root_path":      rootPath,
	}
	if status == indexStatusRunning {
		out["indexed_this_run"] = indexedThisRun
		out["to_index_this_run"] = toIndexThisRun
	}
	if errMsg != "" {
		out["error"] = errMsg
	}
	b, _ := json.Marshal(out)
	return nil, TextOutput{Text: string(b)}, nil
}
