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

## MCP Client Configuration

Add goindexer as an MCP server in your preferred client. Replace `/path/to/goindexer/bin/goindexer` with the path to your built binary (e.g. `$HOME/dev/goindexer/go/bin/goindexer`), or add `go/bin` to your PATH.

### Codex

Config lives in `~/.codex/config.toml` (or project-scoped `.codex/config.toml` for trusted projects). Use the CLI or edit the file directly.

**CLI:**

```bash
codex mcp add goindexer --env SOURCE_INDEX_WORKSPACE=/path/to/your/project -- /path/to/goindexer/bin/goindexer --path /path/to/your/project --watch
```

**config.toml:**

```toml
[mcp_servers.goindexer]
command = "/path/to/goindexer/bin/goindexer"
args = ["--path", "/path/to/your/project", "--watch"]
cwd = "/path/to/your/project"

[mcp_servers.goindexer.env]
SOURCE_INDEX_OLLAMA_URL = "http://localhost:11434"
SOURCE_INDEX_EMBED_MODEL = "qwen3-embedding:4b"
```

For project-specific config, create `.codex/config.toml` in your project root and use the project path. In the Codex IDE, use MCP settings → Open config.toml.

### Claude Code (CLI)

Claude Code uses the `claude mcp add` CLI. Options must come before the server name; `--` separates the name from the command.

**Project scope** (creates `.mcp.json` in project root, shared with team):

```bash
claude mcp add --transport stdio --scope project goindexer -- /path/to/goindexer/bin/goindexer --path /path/to/your/project --watch
```

**User scope** (stored in `~/.claude.json`, available across projects):

```bash
claude mcp add --transport stdio --scope user goindexer -- /path/to/goindexer/bin/goindexer --path /path/to/your/project --watch
```

**With env vars:**

```bash
claude mcp add --transport stdio --scope project --env SOURCE_INDEX_EMBED_MODEL=qwen3-embedding:4b goindexer -- /path/to/goindexer/bin/goindexer --path /path/to/your/project --watch
```

Verify with `claude mcp list` or `/mcp` inside Claude Code.

### Claude Desktop

Config file location:

- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux:** `~/.config/Claude/claude_desktop_config.json`

Or: Settings → Developer → Edit Config.

```json
{
  "mcpServers": {
    "goindexer": {
      "command": "/path/to/goindexer/bin/goindexer",
      "args": ["--path", "/path/to/your/project", "--watch"],
      "env": {
        "SOURCE_INDEX_OLLAMA_URL": "http://localhost:11434",
        "SOURCE_INDEX_EMBED_MODEL": "qwen3-embedding:4b"
      }
    }
  }
}
```

Restart Claude Desktop after editing. Look for the hammer icon (🔨) to confirm tools are loaded.

### VS Code

Config in `.vscode/mcp.json` (workspace) or user profile. Command Palette: **MCP: Open User Configuration** or **MCP: Open Workspace Folder MCP Configuration**.

```json
{
  "servers": {
    "goindexer": {
      "type": "stdio",
      "command": "/path/to/goindexer/bin/goindexer",
      "args": ["--path", "${workspaceFolder}", "--watch"],
      "env": {
        "SOURCE_INDEX_OLLAMA_URL": "http://localhost:11434",
        "SOURCE_INDEX_EMBED_MODEL": "qwen3-embedding:4b"
      }
    }
  }
}
```

Use `${workspaceFolder}` so each workspace uses its own path. Ensure the goindexer binary is on your PATH or use an absolute path.

---

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

