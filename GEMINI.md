# goindexer: Codebase Indexer

This extension provides semantic code search and codebase understanding via MCP tools. It uses tree-sitter chunking, Ollama embeddings, and SQLite vector search.

## Prerequisites

- [Ollama](https://ollama.com/) running with an embedding model (default: `qwen3-embedding:4b`):
  ```bash
  ollama pull qwen3-embedding:4b
  ollama serve
  ```

## Tools

| Tool | When to use |
|------|-------------|
| **index_codebase** | When the user wants to index their codebase, search across it, or asks to "index" / "build the index". Start indexing first; other tools depend on it. |
| **get_index_status** | When the user asks "is indexing done?", "how much is indexed?", or wants progress. If status shows 0 but the project has `.goindexer/`, call with `path` set to the project root (e.g. current directory) to load the existing index. |
| **search_codebase** | Semantic search over indexed code. Use for "find where X is used", "show me code that does Y", or conceptual queries. |
| **grep_search** | Regex/text search over files. Use for exact string or pattern matches (e.g. function names, error messages). |
| **read_file_context** | Read file contents with optional line range. Use when you need to see specific file content. |
| **list_structure** | Directory tree or symbols. Use for "show project structure" or "list symbols in this file". Mode: "tree" or "symbols". |
| **find_references** | Find all references (call sites) to a symbol. Use symbol_name; optionally file_path and line_number to disambiguate. |
| **delete_index** | When the user wants to clear the index and start fresh. |

## Workflow

1. User asks to search or understand the codebase → call **index_codebase** first.
2. User asks "is it done?" → call **get_index_status**.
3. Once indexing is complete (or in progress), use **search_codebase**, **grep_search**, **read_file_context**, **list_structure**, or **find_references** as needed.

The extension runs with `--watch`, so the index auto-updates when files change after indexing has been requested.
