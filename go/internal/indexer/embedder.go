package indexer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/netanelshoshan/goindexer/internal/config"
)

const (
	queryPrefix = "Find the most relevant code snippet given the following query:\n"
)

type Embedder struct {
	cfg    *config.Config
	client *http.Client
}

func NewEmbedder(cfg *config.Config) *Embedder {
	return &Embedder{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type embedRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (e *Embedder) Embed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	input := make([]string, len(texts))
	for i, t := range texts {
		input[i] = "Candidate code snippet:\n" + t
	}
	body, err := json.Marshal(embedRequest{
		Model: e.cfg.EmbedModel,
		Input: input,
	})
	if err != nil {
		return nil, err
	}
	url := e.cfg.OllamaURL + "/api/embed"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("ollama server error: %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed failed %d: %s", resp.StatusCode, string(b))
	}
	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(out.Embeddings))
	}
	return out.Embeddings, nil
}

func (e *Embedder) EmbedQuery(query string) ([]float32, error) {
	text := queryPrefix + query
	embeddings, err := e.Embed([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

func (e *Embedder) EmbedBatched(texts []string, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = e.cfg.BatchSize
	}
	var all [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		emb, err := e.Embed(batch)
		if err != nil {
			return nil, err
		}
		all = append(all, emb...)
	}
	return all, nil
}
