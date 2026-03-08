package storage

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"

	"github.com/netanelshoshan/goindexer/internal/chunk"
	"github.com/netanelshoshan/goindexer/internal/config"
	"github.com/netanelshoshan/goindexer/internal/filematch"
)

type Storage struct {
	db  *sql.DB
	cfg *config.Config
	dim int
}

type SearchResult struct {
	FilePath   string
	SymbolName string
	SymbolType string
	StartLine  int
	EndLine    int
	Language   string
	Text       string
	Distance   float64
}

// SymbolEntry represents a definition or reference for the symbol table.
type SymbolEntry struct {
	SymbolName  string
	FilePath    string
	LineNumber  int
	Type        string // "def" or "ref"
	ParentScope string
	ChunkID     string
}

func New(cfg *config.Config) (*Storage, error) {
	sqlite_vec.Auto()
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	s := &Storage{db: db, cfg: cfg, dim: cfg.EmbedDim}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func chunkIDToInt(id string) int64 {
	h := fnv.New64a()
	h.Write([]byte(id))
	return int64(h.Sum64())
}

func (s *Storage) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			file_path TEXT,
			symbol_name TEXT,
			symbol_type TEXT,
			start_line INT,
			end_line INT,
			language TEXT,
			text TEXT
		)
	`)
	if err != nil {
		return err
	}
	dim := s.dim
	if dim <= 0 {
		dim = config.DefaultEmbedDim
	}
	_, err = s.db.Exec(fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
			vec_id integer primary key,
			embedding float[%d] distance_metric=cosine,
			chunk_id text
		)
	`, dim))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS symbols (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol_name TEXT NOT NULL,
			file_path TEXT NOT NULL,
			line_number INT NOT NULL,
			type TEXT NOT NULL,
			parent_scope TEXT,
			chunk_id TEXT,
			UNIQUE(file_path, line_number, type, symbol_name)
		)
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(symbol_name)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_symbols_def ON symbols(symbol_name, type) WHERE type='def'`)
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) AddChunks(chunks []*chunk.CodeChunk, embeddings [][]float32) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunks and embeddings length mismatch")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, ch := range chunks {
		id := ch.ID()
		_, err := tx.Exec(`
			INSERT OR REPLACE INTO chunks (id, file_path, symbol_name, symbol_type, start_line, end_line, language, text)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, id, ch.FilePath, ch.SymbolName, ch.SymbolType, ch.StartLine, ch.EndLine, ch.Language, ch.Text)
		if err != nil {
			return err
		}
		blob, err := sqlite_vec.SerializeFloat32(embeddings[i])
		if err != nil {
			return err
		}
		vecID := chunkIDToInt(id)
		_, err = tx.Exec(`
			INSERT OR REPLACE INTO vec_chunks (vec_id, embedding, chunk_id)
			VALUES (?, ?, ?)
		`, vecID, blob, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Storage) AddSymbols(entries []SymbolEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO symbols (symbol_name, file_path, line_number, type, parent_scope, chunk_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		_, err = stmt.Exec(e.SymbolName, e.FilePath, e.LineNumber, e.Type, e.ParentScope, e.ChunkID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReferenceResult holds a reference to a symbol (call site).
type ReferenceResult struct {
	SymbolName  string
	FilePath    string
	LineNumber  int
	ParentScope string
	ChunkID     string
	Text        string
}

func (s *Storage) FindReferences(symbolName string, defFilePath string, defLine int) ([]ReferenceResult, error) {
	rows, err := s.db.Query(`
		SELECT s.symbol_name, s.file_path, s.line_number, s.parent_scope, s.chunk_id, c.text
		FROM symbols s
		LEFT JOIN chunks c ON c.id = s.chunk_id
		WHERE s.symbol_name = ? AND s.type = 'ref'
		ORDER BY s.file_path, s.line_number
	`, symbolName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []ReferenceResult
	for rows.Next() {
		var r ReferenceResult
		var chunkID, text sql.NullString
		var parentScope sql.NullString
		if err := rows.Scan(&r.SymbolName, &r.FilePath, &r.LineNumber, &parentScope, &chunkID, &text); err != nil {
			return nil, err
		}
		if parentScope.Valid {
			r.ParentScope = parentScope.String
		}
		if chunkID.Valid {
			r.ChunkID = chunkID.String
		}
		if text.Valid {
			r.Text = text.String
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Storage) DeleteSymbolsByFilePath(path string) error {
	_, err := s.db.Exec(`DELETE FROM symbols WHERE file_path = ?`, path)
	return err
}

func (s *Storage) Search(embedding []float32, topK int, fileFilter string) ([]SearchResult, error) {
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, err
	}
	k := topK
	if fileFilter != "" {
		k = topK * 5
	}
	rows, err := s.db.Query(`
		WITH knn_matches AS (
			SELECT chunk_id, distance FROM vec_chunks
			WHERE embedding MATCH ? AND k = ?
		)
		SELECT k.chunk_id, k.distance, c.file_path, c.symbol_name, c.symbol_type, c.start_line, c.end_line, c.language, c.text
		FROM knn_matches k
		JOIN chunks c ON c.id = k.chunk_id
	`, blob, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var chunkID string
		if err := rows.Scan(&chunkID, &r.Distance, &r.FilePath, &r.SymbolName, &r.SymbolType, &r.StartLine, &r.EndLine, &r.Language, &r.Text); err != nil {
			return nil, err
		}
		if fileFilter != "" && !filematch.MatchFileFilter(fileFilter, r.FilePath) {
			continue
		}
		results = append(results, r)
		if len(results) >= topK {
			break
		}
	}
	return results, rows.Err()
}

func (s *Storage) ListByPathPrefix(prefix string) ([]SearchResult, error) {
	pattern := prefix + "%"
	rows, err := s.db.Query(`
		SELECT id, file_path, symbol_name, symbol_type, start_line, end_line, language, text
		FROM chunks
		WHERE file_path LIKE ?
		ORDER BY file_path, start_line
	`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var id string
		if err := rows.Scan(&id, &r.FilePath, &r.SymbolName, &r.SymbolType, &r.StartLine, &r.EndLine, &r.Language, &r.Text); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CountIndexedFiles returns the number of distinct files in the index.
// Used as fallback when manifest.json is missing.
func (s *Storage) CountIndexedFiles() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(DISTINCT file_path) FROM chunks`).Scan(&n)
	return n, err
}

func (s *Storage) ClearAll() error {
	if _, err := s.db.Exec(`DELETE FROM vec_chunks`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM chunks`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM symbols`); err != nil {
		return err
	}
	return nil
}

func (s *Storage) DeleteByFilePath(path string) error {
	rows, err := s.db.Query(`SELECT id FROM chunks WHERE file_path = ?`, path)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		_, err = s.db.Exec(`DELETE FROM vec_chunks WHERE vec_id = ?`, chunkIDToInt(id))
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`DELETE FROM chunks WHERE file_path = ?`, path)
	if err != nil {
		return err
	}
	return s.DeleteSymbolsByFilePath(path)
}
