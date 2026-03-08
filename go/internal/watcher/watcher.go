package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/netanelshoshan/goindexer/internal/config"
	"github.com/netanelshoshan/goindexer/internal/indexer"
)

// Options configures the file watcher behavior.
type Options struct {
	// ShouldReindex is called before each reindex to decide if we should proceed.
	// When false, reindex is skipped.
	ShouldReindex func() bool
	// OnReindex is the callback invoked when file changes are detected (after debounce).
	// When nil, uses indexer.RunIndex with the given root and cfg.
	OnReindex func() (indexed, total int, err error)
	// Debounce is the delay before triggering reindex after a file change.
	// When zero, defaults to 2 seconds.
	Debounce time.Duration
}

const defaultDebounce = 2 * time.Second

// Run watches the root directory for file changes and triggers reindexing.
// It runs until ctx is cancelled.
func Run(ctx context.Context, root string, cfg *config.Config, opts Options) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watcher: %v", err)
		return
	}
	defer watcher.Close()

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		log.Printf("watcher: %v", err)
		return
	}
	if err := watcher.Add(rootAbs); err != nil {
		log.Printf("watcher add %s: %v", rootAbs, err)
		return
	}
	// Add subdirs (fsnotify does not watch recursively)
	filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == rootAbs {
			return nil
		}
		name := d.Name()
		if name == ".git" || name == "node_modules" || name == ".venv" || name == "venv" {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return nil
		}
		return nil
	})

	debounce := opts.Debounce
	if debounce == 0 {
		debounce = defaultDebounce
	}
	onReindex := opts.OnReindex
	if onReindex == nil {
		onReindex = func() (int, int, error) {
			return indexer.RunIndex(rootAbs, false, cfg, nil)
		}
	}
	shouldReindex := opts.ShouldReindex
	if shouldReindex == nil {
		shouldReindex = func() bool { return true }
	}

	var timer *time.Timer
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) == 0 {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, func() {
				if !shouldReindex() {
					return
				}
				log.Printf("re-indexing changed files after file change")
				if n, total, err := onReindex(); err != nil {
					log.Printf("re-index failed: %v", err)
				} else {
					log.Printf("re-indexed %d files (%d total)", n, total)
				}
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}
