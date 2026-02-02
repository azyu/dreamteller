// Package search provides full-text search capabilities for dreamteller.
package search

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/azyu/dreamteller/internal/storage"
)

// FTSSearchResult represents a search result from the FTS5 engine.
type FTSSearchResult struct {
	ID         int64
	Content    string
	SourceType string
	SourcePath string
	TokenCount int
	Score      float64
}

// FTSEngine implements a search engine using SQLite FTS5.
type FTSEngine struct {
	db *storage.SQLiteDB
}

// NewFTSEngine creates a new FTS5-backed search engine.
func NewFTSEngine(db *storage.SQLiteDB) *FTSEngine {
	return &FTSEngine{
		db: db,
	}
}

// Search performs a full-text search using FTS5 with BM25 scoring.
// The query is sanitized to prevent FTS5 syntax errors.
// Results are returned ordered by relevance score (best matches first).
func (e *FTSEngine) Search(query string, limit int) ([]FTSSearchResult, error) {
	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}

	// Sanitize the query for FTS5
	sanitizedQuery := sanitizeFTS5Query(query)
	if sanitizedQuery == "" {
		return nil, nil
	}

	rows, err := e.db.DB().Query(`
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
		LIMIT ?`,
		sanitizedQuery,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []FTSSearchResult
	for rows.Next() {
		var r FTSSearchResult
		if err := rows.Scan(
			&r.ID,
			&r.Content,
			&r.SourceType,
			&r.SourcePath,
			&r.TokenCount,
			&r.Score,
		); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// SearchWithFilter performs a full-text search filtered by source type.
// This is useful for narrowing results to specific content categories.
func (e *FTSEngine) SearchWithFilter(query string, sourceType string, limit int) ([]FTSSearchResult, error) {
	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}

	// Sanitize the query for FTS5
	sanitizedQuery := sanitizeFTS5Query(query)
	if sanitizedQuery == "" {
		return nil, nil
	}

	rows, err := e.db.DB().Query(`
		SELECT
			chunks_fts.rowid,
			chunks_fts.content,
			chunks_fts.source_type,
			chunks_fts.source_path,
			chunks_meta.token_count,
			bm25(chunks_fts) as score
		FROM chunks_fts
		JOIN chunks_meta ON chunks_fts.rowid = chunks_meta.rowid
		WHERE chunks_fts MATCH ? AND chunks_fts.source_type = ?
		ORDER BY score
		LIMIT ?`,
		sanitizedQuery,
		sourceType,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []FTSSearchResult
	for rows.Next() {
		var r FTSSearchResult
		if err := rows.Scan(
			&r.ID,
			&r.Content,
			&r.SourceType,
			&r.SourcePath,
			&r.TokenCount,
			&r.Score,
		); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// SearchWithHighlight performs a search and returns results with highlighted snippets.
// The highlightStart and highlightEnd strings wrap matched terms in the snippet.
func (e *FTSEngine) SearchWithHighlight(query string, limit int, highlightStart, highlightEnd string) ([]HighlightedResult, error) {
	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}
	if highlightStart == "" {
		highlightStart = "**"
	}
	if highlightEnd == "" {
		highlightEnd = "**"
	}

	// Sanitize the query for FTS5
	sanitizedQuery := sanitizeFTS5Query(query)
	if sanitizedQuery == "" {
		return nil, nil
	}

	rows, err := e.db.DB().Query(`
		SELECT
			chunks_fts.rowid,
			chunks_fts.content,
			snippet(chunks_fts, 0, ?, ?, '...', 32) as snippet,
			chunks_fts.source_type,
			chunks_fts.source_path,
			chunks_meta.token_count,
			bm25(chunks_fts) as score
		FROM chunks_fts
		JOIN chunks_meta ON chunks_fts.rowid = chunks_meta.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY score
		LIMIT ?`,
		highlightStart,
		highlightEnd,
		sanitizedQuery,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []HighlightedResult
	for rows.Next() {
		var r HighlightedResult
		if err := rows.Scan(
			&r.ID,
			&r.Content,
			&r.Snippet,
			&r.SourceType,
			&r.SourcePath,
			&r.TokenCount,
			&r.Score,
		); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// HighlightedResult extends FTSSearchResult with a highlighted snippet.
type HighlightedResult struct {
	FTSSearchResult
	Snippet string
}

// Index adds a chunk to the search index.
// The metadata string should be valid JSON or empty.
func (e *FTSEngine) Index(content, sourceType, sourcePath string, tokenCount int, mtime time.Time, metadata string) error {
	tx, err := e.db.DB().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into FTS table
	result, err := tx.Exec(
		"INSERT INTO chunks_fts (content, source_type, source_path) VALUES (?, ?, ?)",
		content, sourceType, sourcePath,
	)
	if err != nil {
		return fmt.Errorf("failed to insert into FTS index: %w", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get inserted row ID: %w", err)
	}

	// Insert metadata with same rowid
	_, err = tx.Exec(
		`INSERT INTO chunks_meta
			(rowid, source_type, source_path, token_count, mtime, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rowID,
		sourceType,
		sourcePath,
		tokenCount,
		mtime.Unix(),
		metadata,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert metadata: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteBySource removes all chunks for a given source path from the index.
func (e *FTSEngine) DeleteBySource(sourcePath string) error {
	tx, err := e.db.DB().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get rowids to delete
	rows, err := tx.Query("SELECT rowid FROM chunks_meta WHERE source_path = ?", sourcePath)
	if err != nil {
		return fmt.Errorf("failed to query chunks for deletion: %w", err)
	}

	var rowIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan row ID: %w", err)
		}
		rowIDs = append(rowIDs, id)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating row IDs: %w", err)
	}

	// Delete from both tables
	for _, id := range rowIDs {
		if _, err := tx.Exec("DELETE FROM chunks_fts WHERE rowid = ?", id); err != nil {
			return fmt.Errorf("failed to delete from FTS index: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM chunks_meta WHERE rowid = ?", id); err != nil {
			return fmt.Errorf("failed to delete from metadata table: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit deletion: %w", err)
	}

	return nil
}

// Clear removes all entries from the search index.
// This is typically used before a full reindex operation.
func (e *FTSEngine) Clear() error {
	tx, err := e.db.DB().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear both tables
	if _, err := tx.Exec("DELETE FROM chunks_fts"); err != nil {
		return fmt.Errorf("failed to clear FTS index: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM chunks_meta"); err != nil {
		return fmt.Errorf("failed to clear metadata table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit clear operation: %w", err)
	}

	return nil
}

// GetChunkCount returns the total number of indexed chunks.
func (e *FTSEngine) GetChunkCount() (int64, error) {
	var count int64
	err := e.db.DB().QueryRow("SELECT COUNT(*) FROM chunks_meta").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count chunks: %w", err)
	}
	return count, nil
}

// GetChunkCountByType returns the number of indexed chunks for a specific source type.
func (e *FTSEngine) GetChunkCountByType(sourceType string) (int64, error) {
	var count int64
	err := e.db.DB().QueryRow(
		"SELECT COUNT(*) FROM chunks_meta WHERE source_type = ?",
		sourceType,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count chunks by type: %w", err)
	}
	return count, nil
}

// Reindex clears the entire index and rebuilds it from the provided chunks.
// This is an atomic operation.
func (e *FTSEngine) Reindex(chunks []IndexableChunk) error {
	tx, err := e.db.DB().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing data
	if _, err := tx.Exec("DELETE FROM chunks_fts"); err != nil {
		return fmt.Errorf("failed to clear FTS index: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM chunks_meta"); err != nil {
		return fmt.Errorf("failed to clear metadata table: %w", err)
	}

	// Reinsert all chunks
	now := time.Now().Unix()
	for _, chunk := range chunks {
		// Insert into FTS
		result, err := tx.Exec(
			"INSERT INTO chunks_fts (content, source_type, source_path) VALUES (?, ?, ?)",
			chunk.Content, chunk.SourceType, chunk.SourcePath,
		)
		if err != nil {
			return fmt.Errorf("failed to insert %s into FTS index: %w", chunk.SourcePath, err)
		}

		rowID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get row ID for %s: %w", chunk.SourcePath, err)
		}

		// Insert metadata
		_, err = tx.Exec(
			`INSERT INTO chunks_meta
				(rowid, source_type, source_path, token_count, mtime, metadata, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			rowID,
			chunk.SourceType,
			chunk.SourcePath,
			chunk.TokenCount,
			chunk.MTime.Unix(),
			chunk.Metadata,
			now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert metadata for %s: %w", chunk.SourcePath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit reindex: %w", err)
	}

	return nil
}

// IndexableChunk represents a chunk to be indexed via Reindex.
type IndexableChunk struct {
	Content    string
	SourceType string
	SourcePath string
	TokenCount int
	MTime      time.Time
	Metadata   string
}

// GetChunkByID retrieves a chunk by its rowid.
func (e *FTSEngine) GetChunkByID(id int64) (*FTSSearchResult, error) {
	var r FTSSearchResult
	err := e.db.DB().QueryRow(`
		SELECT
			chunks_fts.rowid,
			chunks_fts.content,
			chunks_fts.source_type,
			chunks_fts.source_path,
			chunks_meta.token_count
		FROM chunks_fts
		JOIN chunks_meta ON chunks_fts.rowid = chunks_meta.rowid
		WHERE chunks_fts.rowid = ?`,
		id,
	).Scan(
		&r.ID,
		&r.Content,
		&r.SourceType,
		&r.SourcePath,
		&r.TokenCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk by ID: %w", err)
	}
	return &r, nil
}

// sanitizeFTS5Query prepares a query string for FTS5 MATCH.
// It escapes special characters and handles common query patterns.
func sanitizeFTS5Query(query string) string {
	// Trim whitespace
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	// Split into words and filter empty ones
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}

	// Build a safe query with proper escaping
	var sanitized []string
	for _, word := range words {
		// Remove FTS5 operators from individual words
		cleaned := cleanFTS5Word(word)
		if cleaned != "" {
			sanitized = append(sanitized, cleaned)
		}
	}

	if len(sanitized) == 0 {
		return ""
	}

	// Join with spaces (implicit AND in FTS5)
	return strings.Join(sanitized, " ")
}

// cleanFTS5Word removes or escapes FTS5 special characters from a word.
func cleanFTS5Word(word string) string {
	// Characters that have special meaning in FTS5
	specialChars := `"*^:()-`

	var result strings.Builder
	for _, ch := range word {
		if !strings.ContainsRune(specialChars, ch) {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

