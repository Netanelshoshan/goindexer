package indexer

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/netanelshoshan/goindexer/internal/config"
)

func TestEmbedder_Integration(t *testing.T) {
	cfg := config.Load()
	// Allow override for different models (e.g. qwen3-embedding:4b)
	if m := os.Getenv("SOURCE_INDEX_EMBED_MODEL"); m != "" {
		cfg.EmbedModel = m
	}

	// Quick check if Ollama is reachable
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(cfg.OllamaURL + "/api/tags")
	if err != nil {
		t.Skipf("Ollama not reachable at %s: %v (run 'ollama serve')", cfg.OllamaURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Ollama returned %d at %s", resp.StatusCode, cfg.OllamaURL)
	}

	emb := NewEmbedder(cfg)

	t.Run("EmbedQuery", func(t *testing.T) {
		vec, err := emb.EmbedQuery("test query")
		if err != nil {
			t.Fatalf("EmbedQuery: %v", err)
		}
		if len(vec) == 0 {
			t.Error("expected non-empty embedding")
		}
		if cfg.EmbedDim > 0 && len(vec) != cfg.EmbedDim {
			t.Logf("embedding dim %d (config.EmbedDim=%d, model may differ)", len(vec), cfg.EmbedDim)
		}
	})

	t.Run("Embed_single", func(t *testing.T) {
		vecs, err := emb.Embed([]string{"hello world"})
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if len(vecs) != 1 {
			t.Errorf("expected 1 embedding, got %d", len(vecs))
		}
		if len(vecs) > 0 && len(vecs[0]) == 0 {
			t.Error("expected non-empty embedding vector")
		}
	})

	t.Run("Embed_batch", func(t *testing.T) {
		texts := []string{"a", "b", "c"}
		vecs, err := emb.Embed(texts)
		if err != nil {
			t.Fatalf("Embed batch: %v", err)
		}
		if len(vecs) != 3 {
			t.Errorf("expected 3 embeddings, got %d", len(vecs))
		}
		if len(vecs) > 0 && len(vecs[0]) != len(vecs[1]) {
			t.Error("expected same dimension across batch")
		}
	})

	t.Run("EmbedBatched", func(t *testing.T) {
		texts := []string{"x", "y", "z", "w"}
		vecs, err := emb.EmbedBatched(texts, 2)
		if err != nil {
			t.Fatalf("EmbedBatched: %v", err)
		}
		if len(vecs) != 4 {
			t.Errorf("expected 4 embeddings, got %d", len(vecs))
		}
	})
}
