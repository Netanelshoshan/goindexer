package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/netanelshoshan/goindexer/internal/config"
	"github.com/netanelshoshan/goindexer/internal/indexer"
	"github.com/netanelshoshan/goindexer/internal/server"
	"github.com/netanelshoshan/goindexer/internal/watcher"
)

// discoverWorkspace walks up from cwd looking for a directory containing .goindexer.
// Returns the workspace root path or empty string if not found.
func discoverWorkspace() string {
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

func main() {
	path := flag.String("path", "", "Index root path (required for indexing)")
	doIndex := flag.Bool("index", false, "Run index before starting server")
	watch := flag.Bool("watch", false, "Watch for file changes and re-index in background")
	flag.Parse()

	// When --path is empty, try SOURCE_INDEX_WORKSPACE or discover .goindexer from cwd
	// (Gemini/IDE may not substitute ${workspacePath} correctly)
	if *path == "" {
		if w := os.Getenv("SOURCE_INDEX_WORKSPACE"); w != "" {
			*path = w
		} else if discovered := discoverWorkspace(); discovered != "" {
			*path = discovered
		}
	}

	var cfg *config.Config
	if *path != "" {
		cfg = config.ForWorkspace(*path)
	} else {
		cfg = config.Load()
	}

	if *doIndex {
		if *path == "" {
			log.Fatal("--path is required for indexing (use --index)")
		}
		log.Printf("Indexing... searches will improve when complete. Progress:")
		n, total, err := indexer.RunIndex(*path, true, cfg, nil)
		if err != nil {
			log.Fatalf("index failed: %v", err)
		}
		log.Printf("indexed %d files (%d total in index)", n, total)
	}

	handlers, err := server.NewToolHandlers(cfg, *path)
	if err != nil {
		log.Fatalf("failed to create tool handlers: %v", err)
	}
	defer handlers.Close()

	s := mcp.NewServer(&mcp.Implementation{Name: "goindexer", Version: "1.0.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_codebase",
		Description: "Semantic search over indexed code. Returns relevant code chunks matching the query.",
	}, handlers.HandleSearchCodebase)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "grep_search",
		Description: "Regex search over files in the index. Supports path and file_pattern filters.",
	}, handlers.HandleGrepSearch)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "read_file_context",
		Description: "Read file contents with optional line range. Path is relative to index root.",
	}, handlers.HandleReadFileContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_structure",
		Description: "List directory tree or symbols. Mode: 'tree' or 'symbols'. Optional max_depth.",
	}, handlers.HandleListStructure)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "find_references",
		Description: "Find all references (call sites) to a symbol across the indexed codebase. Use symbol_name (required), optionally file_path and line_number to disambiguate.",
	}, handlers.HandleFindReferences)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "index_codebase",
		Description: "Start indexing the codebase. Optional path param; uses workspace path from config if omitted.",
	}, handlers.HandleIndexCodebase)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_index_status",
		Description: "Get current indexing status (idle, running, completed, failed). Optional path param: workspace root to load (e.g. current dir) when status shows 0 but .goindexer exists. Use when user asks if indexing is done.",
	}, handlers.HandleGetIndexStatus)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_index",
		Description: "Delete the codebase index. Clears all indexed data. User can re-index with index_codebase.",
	}, handlers.HandleDeleteIndex)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if *watch && *path != "" {
		go watcher.Run(ctx, *path, cfg, watcher.Options{
			ShouldReindex: handlers.ShouldReindexOnChange,
		})
	}

	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatalf("server error: %v", err)
	}
}
