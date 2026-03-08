# goindexer

<p align="center">
  <img src="goindexer.png" alt="goindexer" width="320"/>
</p>

Semantic code search and codebase understanding via MCP. Uses tree-sitter chunking, Ollama embeddings, and SQLite vector search.

## Prerequisites

- [Ollama](https://ollama.com/) running with an embedding model:

  ```bash
  ollama pull qwen3-embedding:4b
  ollama serve
  ```

## Build

```bash
cd go
go build -o bin/goindexer ./cmd/goindexer
```

## Usage

### CLI

```bash
# Index a codebase, then start MCP server
goindexer --path /path/to/project --index --watch

# Start server only (index via MCP tools)
goindexer --path /path/to/project --watch
```

| Flag | Description |
|------|-------------|
| `--path` | Index root path (required for indexing). Falls back to `SOURCE_INDEX_WORKSPACE` or discovers `.goindexer` from cwd. |
| `--index` | Run full index before starting server |
| `--watch` | Watch for file changes and re-index in background |

### MCP Server

The server exposes tools over stdio (MCP transport). Use with Cursor, Claude Desktop, or any MCP client.

## Configuration

All settings via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SOURCE_INDEX_OLLAMA_URL` | `http://localhost:11434` | Ollama API URL |
| `SOURCE_INDEX_EMBED_MODEL` | `qwen3-embedding:4b` | Embedding model name |
| `SOURCE_INDEX_EMBED_DIM` | `2560` | Embedding dimension *(must match model)* |
| `SOURCE_INDEX_BATCH_SIZE` | `32` | Batch size for embedding requests |
| `SOURCE_INDEX_MAX_FILE_SIZE` | `1048576` | Max file size in bytes (1MB) |
| `SOURCE_INDEX_MAX_CHUNK_TOKENS` | `4096` | Max tokens per chunk |
| `SOURCE_INDEX_MIN_CHUNK_TOKENS` | `64` | Min tokens per chunk |
| `SOURCE_INDEX_EXTRA_EXCLUDES` | — | Comma-separated globs to exclude |
| `SOURCE_INDEX_WORKSPACE` | — | Workspace path when `--path` not set |

### Embedding models

Different models require different config. Set `SOURCE_INDEX_EMBED_DIM` to match the model's output dimension:

| Model | Embed dim |
|-------|-----------|
| `qwen3-embedding:4b` | 2560 |
| `nomic-embed-text` | 768 |

Switching to a model with a different dimension requires deleting the index and re-indexing.

## MCP Tools

| Tool | Description |
|------|-------------|
| `index_codebase` | Index the codebase. Call first before search. |
| `get_index_status` | Check indexing progress (idle, running, completed, failed). |
| `search_codebase` | Semantic search over indexed code. |
| `grep_search` | Regex search over files. |
| `read_file_context` | Read file contents with optional line range. |
| `list_structure` | Directory tree or symbols. Mode: `tree` or `symbols`. |
| `find_references` | Find all references to a symbol. |
| `delete_index` | Delete the index and clear all data. |

## Gemini CLI Extension

### Install

```bash
gemini extensions install github.com/netanelshoshanshoshan/goindexer
```

Or install a specific version:

```bash
gemini extensions install github.com/netanelshoshanshoshan/goindexer --ref v1.0.0
```


## Supported languages

Python, Go, JavaScript, TypeScript, JSX, TSX, Rust, Java, Kotlin, C, C++, C#, Ruby, PHP, Swift, Scala.

## Data layout

With `--path` set, index data lives under the project:

```
project/
  .goindexer/
    codebase.db      # SQLite + vectors
    manifest.json
    index_root.txt
    config.json      # Effective config snapshot
```

Without `--path`, data uses `~/.goindexer/` (global).

