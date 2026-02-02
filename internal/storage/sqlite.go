// Package storage provides file and database handling.
package storage

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteDB manages the SQLite database for a project.
type SQLiteDB struct {
	db   *sql.DB
	path string
}

// NewSQLiteDB opens or creates a SQLite database.
func NewSQLiteDB(projectPath string) (*SQLiteDB, error) {
	dbPath := filepath.Join(projectPath, ".dreamteller", "store.db")

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sqliteDB := &SQLiteDB{
		db:   db,
		path: dbPath,
	}

	if err := sqliteDB.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return sqliteDB, nil
}

// initialize creates the required tables if they don't exist.
func (s *SQLiteDB) initialize() error {
	schema := `
	-- FTS5 virtual table for full-text search
	CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
		content,
		source_type,
		source_path,
		tokenize='porter unicode61'
	);

	-- Metadata table for chunks
	CREATE TABLE IF NOT EXISTS chunks_meta (
		rowid INTEGER PRIMARY KEY,
		source_type TEXT NOT NULL,
		source_path TEXT NOT NULL,
		token_count INTEGER NOT NULL,
		mtime INTEGER NOT NULL,
		metadata TEXT,
		created_at INTEGER NOT NULL
	);

	-- Index for source path lookups
	CREATE INDEX IF NOT EXISTS idx_chunks_meta_source
	ON chunks_meta(source_path);

	-- File tracking for incremental sync
	CREATE TABLE IF NOT EXISTS file_tracking (
		path TEXT PRIMARY KEY,
		mtime INTEGER NOT NULL,
		indexed_at INTEGER NOT NULL
	);

	-- Conversation history
	CREATE TABLE IF NOT EXISTS conversation (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp INTEGER NOT NULL
	);

	-- Schema version for migrations
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	);

	INSERT OR IGNORE INTO schema_version (version) VALUES (1);
	`

	_, err := s.db.Exec(schema)
	return err
}

// InsertChunk inserts a chunk into both FTS and metadata tables.
func (s *SQLiteDB) InsertChunk(content, sourceType, sourcePath string, tokenCount int, mtime time.Time, metadata string) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Insert into FTS table
	result, err := tx.Exec(
		"INSERT INTO chunks_fts (content, source_type, source_path) VALUES (?, ?, ?)",
		content, sourceType, sourcePath,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert into FTS: %w", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Insert metadata with same rowid
	_, err = tx.Exec(
		"INSERT INTO chunks_meta (rowid, source_type, source_path, token_count, mtime, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		rowID, sourceType, sourcePath, tokenCount, mtime.Unix(), metadata, time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert metadata: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return rowID, nil
}

// SearchChunks performs a full-text search and returns matching chunks.
func (s *SQLiteDB) SearchChunks(query string, limit int) ([]ChunkResult, error) {
	rows, err := s.db.Query(`
		SELECT
			chunks_fts.rowid,
			chunks_fts.content,
			chunks_fts.source_type,
			chunks_fts.source_path,
			chunks_meta.token_count,
			bm25(chunks_fts) as score
		FROM chunks_fts
		JOIN chunks_meta ON chunks_fts.rowid = chunks_meta.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY score
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []ChunkResult
	for rows.Next() {
		var r ChunkResult
		if err := rows.Scan(&r.ID, &r.Content, &r.SourceType, &r.SourcePath, &r.TokenCount, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// ChunkResult represents a search result.
type ChunkResult struct {
	ID         int64
	Content    string
	SourceType string
	SourcePath string
	TokenCount int
	Score      float64
}

// DeleteChunksBySource deletes all chunks for a given source path.
func (s *SQLiteDB) DeleteChunksBySource(sourcePath string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get rowids to delete
	rows, err := tx.Query("SELECT rowid FROM chunks_meta WHERE source_path = ?", sourcePath)
	if err != nil {
		return err
	}

	var rowIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		rowIDs = append(rowIDs, id)
	}
	rows.Close()

	for _, id := range rowIDs {
		if _, err := tx.Exec("DELETE FROM chunks_fts WHERE rowid = ?", id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM chunks_meta WHERE rowid = ?", id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpdateFileTracking updates the tracking information for a file.
func (s *SQLiteDB) UpdateFileTracking(path string, mtime time.Time) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO file_tracking (path, mtime, indexed_at)
		VALUES (?, ?, ?)
	`, path, mtime.Unix(), time.Now().Unix())
	return err
}

// GetFileTracking returns the tracking info for a file.
func (s *SQLiteDB) GetFileTracking(path string) (*FileTrackingInfo, error) {
	var info FileTrackingInfo
	var mtimeUnix, indexedAtUnix int64

	err := s.db.QueryRow(
		"SELECT path, mtime, indexed_at FROM file_tracking WHERE path = ?",
		path,
	).Scan(&info.Path, &mtimeUnix, &indexedAtUnix)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	info.MTime = time.Unix(mtimeUnix, 0)
	info.IndexedAt = time.Unix(indexedAtUnix, 0)
	return &info, nil
}

// FileTrackingInfo contains file tracking data.
type FileTrackingInfo struct {
	Path      string
	MTime     time.Time
	IndexedAt time.Time
}

// DeleteFileTracking removes tracking for a file.
func (s *SQLiteDB) DeleteFileTracking(path string) error {
	_, err := s.db.Exec("DELETE FROM file_tracking WHERE path = ?", path)
	return err
}

// GetAllTrackedFiles returns all tracked files.
func (s *SQLiteDB) GetAllTrackedFiles() ([]FileTrackingInfo, error) {
	rows, err := s.db.Query("SELECT path, mtime, indexed_at FROM file_tracking")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileTrackingInfo
	for rows.Next() {
		var info FileTrackingInfo
		var mtimeUnix, indexedAtUnix int64
		if err := rows.Scan(&info.Path, &mtimeUnix, &indexedAtUnix); err != nil {
			return nil, err
		}
		info.MTime = time.Unix(mtimeUnix, 0)
		info.IndexedAt = time.Unix(indexedAtUnix, 0)
		files = append(files, info)
	}

	return files, rows.Err()
}

// SaveConversationMessage saves a message to conversation history.
func (s *SQLiteDB) SaveConversationMessage(role, content string) error {
	_, err := s.db.Exec(
		"INSERT INTO conversation (role, content, timestamp) VALUES (?, ?, ?)",
		role, content, time.Now().Unix(),
	)
	return err
}

// GetConversationHistory returns the conversation history.
func (s *SQLiteDB) GetConversationHistory(limit int) ([]ConversationRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, role, content, timestamp
		FROM conversation
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ConversationRecord
	for rows.Next() {
		var msg ConversationRecord
		var timestampUnix int64
		if err := rows.Scan(&msg.ID, &msg.Role, &msg.Content, &timestampUnix); err != nil {
			return nil, err
		}
		msg.Timestamp = time.Unix(timestampUnix, 0)
		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}

// ConversationRecord represents a conversation message from the database.
type ConversationRecord struct {
	ID        int64
	Role      string
	Content   string
	Timestamp time.Time
}

// ClearConversation clears the conversation history.
func (s *SQLiteDB) ClearConversation() error {
	_, err := s.db.Exec("DELETE FROM conversation")
	return err
}

// Close closes the database connection.
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for advanced queries.
func (s *SQLiteDB) DB() *sql.DB {
	return s.db
}
