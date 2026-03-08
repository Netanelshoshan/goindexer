package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultRootPath       = "~/.goindexer/index_root.txt"
	DefaultDBPath         = "~/.goindexer/codebase.db"
	DefaultManifestPath   = "~/.goindexer/manifest.json"
	DefaultOllamaURL      = "http://localhost:11434"
	DefaultEmbedModel     = "qwen3-embedding:4b"
	DefaultBatchSize      = 32
	DefaultMaxFileSize    = 1024 * 1024 // 1MB
	DefaultEmbedDim       = 2560        // qwen3-embedding:4b; nomic-embed-text uses 768
	DefaultMaxChunkTokens = 4096
	DefaultMinChunkTokens = 64
	ReadFileLimit         = 50 * 1024 // 50KB
	GrepMatchLimit        = 100
)

var DefaultExcludes = []string{
	".git", "__pycache__", "node_modules", ".venv", "venv",
	"*.pyc", "*.pyo", ".mypy_cache", ".ruff_cache",
	"dist", "build", "*.egg-info", ".eggs",
	"vendor", ".next", ".nuxt", "coverage",
}

var IndexExtensions = map[string]bool{
	".py": true, ".go": true, ".js": true, ".ts": true,
	".jsx": true, ".tsx": true, ".rs": true, ".java": true,
	".kt": true, ".c": true, ".cpp": true, ".h": true,
	".hpp": true, ".cs": true, ".rb": true, ".php": true,
	".swift": true, ".scala": true,
}

type Config struct {
	RootPath       string
	DBPath         string
	ManifestPath   string
	OllamaURL      string
	EmbedModel     string
	BatchSize      int
	MaxFileSize    int64
	MaxChunkTokens int
	MinChunkTokens int
	EmbedDim       int
	ExtraExcludes  []string
}

// DataDir returns the .goindexer directory for a workspace root.
func DataDir(rootPath string) string {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return filepath.Join(rootPath, ".goindexer")
	}
	return filepath.Join(abs, ".goindexer")
}

// ForWorkspace returns a config with DB, manifest, root path, and settings under the workspace.
// Use when --path is set so the extension can write (sandboxed envs often block ~/.goindexer).
func ForWorkspace(rootPath string) *Config {
	dataDir := DataDir(rootPath)
	cfg := &Config{
		RootPath:       filepath.Join(dataDir, "index_root.txt"),
		DBPath:         filepath.Join(dataDir, "codebase.db"),
		ManifestPath:   filepath.Join(dataDir, "manifest.json"),
		OllamaURL:      envOrDefault("SOURCE_INDEX_OLLAMA_URL", DefaultOllamaURL),
		EmbedModel:     envOrDefault("SOURCE_INDEX_EMBED_MODEL", DefaultEmbedModel),
		BatchSize:      DefaultBatchSize,
		MaxFileSize:    DefaultMaxFileSize,
		MaxChunkTokens: DefaultMaxChunkTokens,
		MinChunkTokens: DefaultMinChunkTokens,
		EmbedDim:       DefaultEmbedDim,
	}
	applyEnvOverrides(cfg)
	return cfg
}

// Load returns config using ~/.goindexer/ (global). Use when running without --path.
// For extensions and --path mode, use ForWorkspace so all data stays under the codebase.
func Load() *Config {
	cfg := &Config{
		RootPath:       expandPath(envOrDefault("SOURCE_INDEX_ROOT_PATH", DefaultRootPath)),
		DBPath:         expandPath(envOrDefault("SOURCE_INDEX_DB_PATH", DefaultDBPath)),
		ManifestPath:   expandPath(envOrDefault("SOURCE_INDEX_MANIFEST_PATH", DefaultManifestPath)),
		OllamaURL:      envOrDefault("SOURCE_INDEX_OLLAMA_URL", DefaultOllamaURL),
		EmbedModel:     envOrDefault("SOURCE_INDEX_EMBED_MODEL", DefaultEmbedModel),
		BatchSize:      DefaultBatchSize,
		MaxFileSize:    DefaultMaxFileSize,
		MaxChunkTokens: DefaultMaxChunkTokens,
		MinChunkTokens: DefaultMinChunkTokens,
		EmbedDim:       DefaultEmbedDim,
	}
	applyEnvOverrides(cfg)
	return cfg
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SOURCE_INDEX_BATCH_SIZE"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cfg.BatchSize = n
		}
	}
	if v := os.Getenv("SOURCE_INDEX_MAX_FILE_SIZE"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cfg.MaxFileSize = int64(n)
		}
	}
	if v := os.Getenv("SOURCE_INDEX_MAX_CHUNK_TOKENS"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cfg.MaxChunkTokens = n
		}
	}
	if v := os.Getenv("SOURCE_INDEX_MIN_CHUNK_TOKENS"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cfg.MinChunkTokens = n
		}
	}
	if v := os.Getenv("SOURCE_INDEX_EMBED_DIM"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cfg.EmbedDim = n
		}
	}
	if v := os.Getenv("SOURCE_INDEX_EXTRA_EXCLUDES"); v != "" {
		if v != "" {
			cfg.ExtraExcludes = strings.Split(v, ",")
			for i := range cfg.ExtraExcludes {
				cfg.ExtraExcludes[i] = strings.TrimSpace(cfg.ExtraExcludes[i])
			}
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// configFile is the JSON written to .goindexer/config.json for debugging and reproducibility.
type configFile struct {
	RootPath       string   `json:"root_path"`
	DBPath         string   `json:"db_path"`
	ManifestPath   string   `json:"manifest_path"`
	OllamaURL      string   `json:"ollama_url"`
	EmbedModel     string   `json:"embed_model"`
	BatchSize      int      `json:"batch_size"`
	MaxFileSize    int64    `json:"max_file_size"`
	MaxChunkTokens int      `json:"max_chunk_tokens"`
	MinChunkTokens int      `json:"min_chunk_tokens"`
	EmbedDim       int      `json:"embed_dim"`
	ExtraExcludes  []string `json:"extra_excludes,omitempty"`
}

// SaveConfig writes the effective config to .goindexer/config.json under the data dir.
func SaveConfig(cfg *Config) error {
	dataDir := filepath.Dir(cfg.DBPath)
	path := filepath.Join(dataDir, "config.json")
	out := configFile{
		RootPath:       cfg.RootPath,
		DBPath:         cfg.DBPath,
		ManifestPath:   cfg.ManifestPath,
		OllamaURL:      cfg.OllamaURL,
		EmbedModel:     cfg.EmbedModel,
		BatchSize:      cfg.BatchSize,
		MaxFileSize:    cfg.MaxFileSize,
		MaxChunkTokens: cfg.MaxChunkTokens,
		MinChunkTokens: cfg.MinChunkTokens,
		EmbedDim:       cfg.EmbedDim,
		ExtraExcludes:  cfg.ExtraExcludes,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
