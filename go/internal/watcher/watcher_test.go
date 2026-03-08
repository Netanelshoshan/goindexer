package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netanelshoshan/goindexer/internal/config"
)

func TestWatcher_FileWrite_TriggersReindex(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ForWorkspace(dir)

	// Create initial file so watcher has something to watch
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	var reindexCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Run(ctx, dir, cfg, Options{
		ShouldReindex: func() bool { return true },
		OnReindex: func() (int, int, error) {
			reindexCount.Add(1)
			return 0, 0, nil
		},
		Debounce: 50 * time.Millisecond,
	})

	// Give watcher time to start and add dirs
	time.Sleep(100 * time.Millisecond)

	// Modify file - should trigger reindex after debounce
	if err := os.WriteFile(filePath, []byte("package main\n\nfunc main() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + buffer
	time.Sleep(200 * time.Millisecond)

	if n := reindexCount.Load(); n < 1 {
		t.Errorf("expected at least 1 reindex after file write, got %d", n)
	}
}

func TestWatcher_FileCreate_TriggersReindex(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ForWorkspace(dir)

	var reindexCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Run(ctx, dir, cfg, Options{
		ShouldReindex: func() bool { return true },
		OnReindex: func() (int, int, error) {
			reindexCount.Add(1)
			return 0, 0, nil
		},
		Debounce: 50 * time.Millisecond,
	})

	time.Sleep(100 * time.Millisecond)

	// Create new file
	filePath := filepath.Join(dir, "new.go")
	if err := os.WriteFile(filePath, []byte("package p"), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	if n := reindexCount.Load(); n < 1 {
		t.Errorf("expected at least 1 reindex after file create, got %d", n)
	}
}

func TestWatcher_FileRemove_TriggersReindex(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ForWorkspace(dir)

	filePath := filepath.Join(dir, "to_remove.go")
	if err := os.WriteFile(filePath, []byte("package p"), 0644); err != nil {
		t.Fatal(err)
	}

	var reindexCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Run(ctx, dir, cfg, Options{
		ShouldReindex: func() bool { return true },
		OnReindex: func() (int, int, error) {
			reindexCount.Add(1)
			return 0, 0, nil
		},
		Debounce: 50 * time.Millisecond,
	})

	time.Sleep(100 * time.Millisecond)

	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	if n := reindexCount.Load(); n < 1 {
		t.Errorf("expected at least 1 reindex after file remove, got %d", n)
	}
}

func TestWatcher_ShouldReindexFalse_NoReindex(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ForWorkspace(dir)

	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	var reindexCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Run(ctx, dir, cfg, Options{
		ShouldReindex: func() bool { return false },
		OnReindex: func() (int, int, error) {
			reindexCount.Add(1)
			return 0, 0, nil
		},
		Debounce: 50 * time.Millisecond,
	})

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filePath, []byte("package main\n\nfunc foo() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	if n := reindexCount.Load(); n != 0 {
		t.Errorf("expected 0 reindex when ShouldReindex returns false, got %d", n)
	}
}

func TestWatcher_Debounce_MultipleRapidChanges_SingleReindex(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ForWorkspace(dir)

	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	var reindexCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	debounce := 100 * time.Millisecond
	go Run(ctx, dir, cfg, Options{
		ShouldReindex: func() bool { return true },
		OnReindex: func() (int, int, error) {
			reindexCount.Add(1)
			return 0, 0, nil
		},
		Debounce: debounce,
	})

	time.Sleep(100 * time.Millisecond)

	// Rapid successive writes - should debounce to single reindex
	for i := 0; i < 5; i++ {
		content := []byte("package main\n\n// edit " + string(rune('0'+i)))
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to fire
	time.Sleep(debounce + 50*time.Millisecond)

	if n := reindexCount.Load(); n != 1 {
		t.Errorf("expected 1 reindex (debounced), got %d", n)
	}
}

func TestWatcher_ContextCancel_StopsWatcher(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ForWorkspace(dir)

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		Run(ctx, dir, cfg, Options{
			ShouldReindex: func() bool { return true },
			OnReindex:     func() (int, int, error) { return 0, 0, nil },
			Debounce:      time.Second,
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
	// If we get here without hanging, the watcher stopped on context cancel
}
