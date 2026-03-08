package indexer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/netanelshoshan/goindexer/internal/config"
	"github.com/netanelshoshan/goindexer/internal/storage"
)

const numWorkers = 8

// ProgressFunc is called during indexing. Optional.
// indexed, total = files in index / total files in codebase.
// indexedThisRun, toIndexThisRun = files indexed this run / files to index this run (0 when not applicable).
type ProgressFunc func(indexed, total, indexedThisRun, toIndexThisRun int)

// RunIndex returns (indexed this run, total files in index, error).
func RunIndex(root string, force bool, cfg *config.Config, onProgress ProgressFunc) (indexedCount, totalInIndex int, err error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return 0, 0, err
	}
	st, err := storage.New(cfg)
	if err != nil {
		return 0, 0, err
	}
	defer st.Close()

	scanned, err := ScanFiles(rootAbs, cfg)
	if err != nil {
		return 0, 0, err
	}
	manifest, err := LoadManifest(cfg.ManifestPath)
	if err != nil {
		return 0, 0, err
	}
	delta := ComputeDelta(scanned, manifest, force)
	log.Printf("scan: %d files, to index: %d, to remove: %d", len(scanned), len(delta.ToIndex), len(delta.ToRemove))
	if len(delta.ToIndex) > 0 {
		log.Printf("Indexing... grep_search and read_file_context work now; search_codebase improves as indexing completes.")
	}

	for _, path := range delta.ToRemove {
		if err := st.DeleteByFilePath(path); err != nil {
			return 0, 0, fmt.Errorf("delete %s: %w", path, err)
		}
	}

	emb := NewEmbedder(cfg)
	indexedCount = 0
	processedCount := 0
	toIndexCount := len(delta.ToIndex)
	totalScanned := len(scanned)
	alreadyIndexed := len(manifest) - len(delta.ToRemove)
	var mu sync.Mutex
	const progressInterval = 10

	if onProgress != nil && totalScanned > 0 {
		onProgress(alreadyIndexed, totalScanned, 0, toIndexCount)
	}

	type work struct {
		path string
		hash string
	}
	workCh := make(chan work, len(delta.ToIndex))
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				fullPath := filepath.Join(rootAbs, w.path)
				content, err := os.ReadFile(fullPath)
				if err != nil {
					log.Printf("index skip %s: read: %v", w.path, err)
					mu.Lock()
					processedCount++
					mu.Unlock()
					continue
				}
				chunks, symbols, err := ChunkFile(w.path, content, cfg)
				if err != nil || len(chunks) == 0 {
					if err != nil {
						log.Printf("index skip %s: chunk: %v", w.path, err)
					}
					mu.Lock()
					processedCount++
					mu.Unlock()
					continue
				}
				texts := make([]string, len(chunks))
				for i, c := range chunks {
					texts[i] = c.Text
				}
				embeddings, err := emb.EmbedBatched(texts, cfg.BatchSize)
				if err != nil {
					log.Printf("index skip %s: embed: %v (is Ollama running with qwen3-embedding/other embedding model?)", w.path, err)
					mu.Lock()
					processedCount++
					mu.Unlock()
					continue
				}
				if err := st.AddChunks(chunks, embeddings); err != nil {
					log.Printf("index skip %s: storage: %v", w.path, err)
					mu.Lock()
					processedCount++
					mu.Unlock()
					continue
				}
				if len(symbols) > 0 {
					if err := st.AddSymbols(symbols); err != nil {
						log.Printf("index skip %s: symbols: %v", w.path, err)
					}
				}
				mu.Lock()
				indexedCount++
				processedCount++
				manifest[w.path] = w.hash
				n := indexedCount
				p := processedCount
				// Periodic save manifest every 10 files to provide checkpointing
				if indexedCount%10 == 0 {
					SaveManifest(cfg.ManifestPath, manifest)
				}
				mu.Unlock()

				indexedTotal := alreadyIndexed + p
				pct := 0
				if totalScanned > 0 {
					pct = indexedTotal * 100 / totalScanned
				}
				if p%progressInterval == 0 || p == toIndexCount {
					log.Printf("indexed %d/%d (processed %d/%d) this run, %d/%d total (%d%%)", n, toIndexCount, p, toIndexCount, indexedTotal, totalScanned, pct)
				}
				if onProgress != nil && totalScanned > 0 {
					onProgress(indexedTotal, totalScanned, p, toIndexCount)
				}
			}
		}()
	}
	for _, s := range delta.ToIndex {
		workCh <- work{path: s.Path, hash: s.Hash}
	}
	close(workCh)
	wg.Wait()

	// Report final count: total files in index = len(scanned)
	totalInIndex = len(scanned)
	if onProgress != nil && totalInIndex > 0 {
		onProgress(totalInIndex, totalInIndex, processedCount, toIndexCount)
	}

	if err := SaveManifest(cfg.ManifestPath, manifest); err != nil {
		return indexedCount, 0, err
	}
	rootDir := filepath.Dir(cfg.RootPath)
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return indexedCount, 0, err
	}
	if err := os.WriteFile(cfg.RootPath, []byte(rootAbs), 0644); err != nil {
		return indexedCount, 0, err
	}
	if err := config.SaveConfig(cfg); err != nil {
		log.Printf("save config: %v", err)
	}
	return indexedCount, totalInIndex, nil
}
