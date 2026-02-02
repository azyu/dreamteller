// Package search provides full-text search indexing and retrieval.
package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/azyu/dreamteller/internal/storage"
)

// TokenCounter provides token counting operations for chunking content.
type TokenCounter interface {
	Count(text string) int
	Split(text string, chunkSize int, overlap float64) []string
}

// Indexer manages the indexing pipeline for documents.
type Indexer struct {
	engine       *FTSEngine
	counter      TokenCounter
	chunkSize    int
	chunkOverlap float64
}

// DefaultChunkSize is the default number of tokens per chunk.
const DefaultChunkSize = 800

// DefaultChunkOverlap is the default overlap fraction between chunks.
const DefaultChunkOverlap = 0.15

// NewIndexer creates a new indexer with the specified configuration.
func NewIndexer(engine *FTSEngine, counter TokenCounter, chunkSize int, overlap float64) *Indexer {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if overlap < 0 || overlap >= 1 {
		overlap = DefaultChunkOverlap
	}

	return &Indexer{
		engine:       engine,
		counter:      counter,
		chunkSize:    chunkSize,
		chunkOverlap: overlap,
	}
}

// IndexFile indexes a single file by reading its content, splitting into chunks,
// and indexing each chunk with metadata.
func (idx *Indexer) IndexFile(path, sourceType string) error {
	return idx.indexFileWithFS(nil, path, sourceType)
}

// IndexFileWithFS indexes a single file using the provided filesystem.
func (idx *Indexer) IndexFileWithFS(fs *storage.FileSystem, path, sourceType string) error {
	return idx.indexFileWithFS(fs, path, sourceType)
}

// IndexFileWithContent indexes content directly without reading from disk.
// Useful for testing or when content is already available.
func (idx *Indexer) IndexFileWithContent(path, sourceType, content string, mtime time.Time) error {
	// Delete existing chunks for this file
	if err := idx.engine.DeleteBySource(path); err != nil {
		return fmt.Errorf("failed to delete existing chunks for %s: %w", path, err)
	}

	// Split content into chunks
	chunks := idx.chunkContent(content)
	if len(chunks) == 0 {
		return nil
	}

	// Index each chunk
	for i, chunk := range chunks {
		chunkID := generateChunkID(path, i)
		tokenCount := idx.counter.Count(chunk)

		metadata := map[string]interface{}{
			"chunk_index":  i,
			"total_chunks": len(chunks),
			"chunk_id":     chunkID,
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata for chunk %d: %w", i, err)
		}

		if err := idx.engine.Index(chunk, sourceType, path, tokenCount, mtime, string(metadataJSON)); err != nil {
			return fmt.Errorf("failed to index chunk %d of %s: %w", i, path, err)
		}
	}

	return nil
}

// IndexDirectory indexes all markdown files in a directory.
func (idx *Indexer) IndexDirectory(dir, sourceType string) error {
	return idx.indexDirectoryWithFS(nil, dir, sourceType)
}

// IndexDirectoryWithFS indexes all markdown files using a FileSystem instance.
func (idx *Indexer) IndexDirectoryWithFS(fs *storage.FileSystem, dir, sourceType string) error {
	return idx.indexDirectoryWithFS(fs, dir, sourceType)
}

func (idx *Indexer) indexDirectoryWithFS(fs *storage.FileSystem, dir, sourceType string) error {
	if fs == nil {
		return fmt.Errorf("filesystem is required for directory indexing")
	}

	files, err := fs.ListMarkdownFiles(dir)
	if err != nil {
		return fmt.Errorf("failed to list markdown files in %s: %w", dir, err)
	}

	var indexErrors []error
	for _, file := range files {
		if err := idx.indexFileWithFS(fs, file.Path, sourceType); err != nil {
			indexErrors = append(indexErrors, fmt.Errorf("failed to index %s: %w", file.Path, err))
		}
	}

	if len(indexErrors) > 0 {
		return fmt.Errorf("encountered %d errors during directory indexing: %v", len(indexErrors), indexErrors[0])
	}

	return nil
}

func (idx *Indexer) indexFileWithFS(fs *storage.FileSystem, path, sourceType string) error {
	if fs == nil {
		return fmt.Errorf("filesystem is required for file indexing")
	}

	content, err := fs.ReadMarkdown(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	fileInfo, err := fs.GetFileInfo(path)
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", path, err)
	}

	return idx.IndexFileWithContent(path, sourceType, content, fileInfo.ModTime)
}

// SyncWithFileSystem performs mtime-based incremental sync.
// It compares file mtimes with indexed mtimes, reindexes changed files,
// and deletes chunks for removed files.
func (idx *Indexer) SyncWithFileSystem(fs *storage.FileSystem, db *storage.SQLiteDB) error {
	if fs == nil {
		return fmt.Errorf("filesystem is required for sync")
	}
	if db == nil {
		return fmt.Errorf("database is required for sync")
	}

	// Get all currently tracked files from database
	trackedFiles, err := db.GetAllTrackedFiles()
	if err != nil {
		return fmt.Errorf("failed to get tracked files: %w", err)
	}

	// Build a map of tracked files for quick lookup
	trackedMap := make(map[string]storage.FileTrackingInfo)
	for _, tf := range trackedFiles {
		trackedMap[tf.Path] = tf
	}

	// Get all current markdown files from filesystem
	currentFiles, err := fs.ListMarkdownFiles(".")
	if err != nil {
		return fmt.Errorf("failed to list markdown files: %w", err)
	}

	// Build a set of current file paths
	currentPaths := make(map[string]struct{})
	for _, f := range currentFiles {
		currentPaths[f.Path] = struct{}{}
	}

	// Process current files
	for _, file := range currentFiles {
		tracked, exists := trackedMap[file.Path]

		needsReindex := !exists || file.ModTime.After(tracked.MTime)

		if needsReindex {
			sourceType := determineSourceType(file.Path)

			if err := idx.indexFileWithFS(fs, file.Path, sourceType); err != nil {
				return fmt.Errorf("failed to reindex %s: %w", file.Path, err)
			}

			if err := db.UpdateFileTracking(file.Path, file.ModTime); err != nil {
				return fmt.Errorf("failed to update tracking for %s: %w", file.Path, err)
			}
		}
	}

	// Delete chunks for removed files
	for path := range trackedMap {
		if _, exists := currentPaths[path]; !exists {
			if err := idx.engine.DeleteBySource(path); err != nil {
				return fmt.Errorf("failed to delete chunks for removed file %s: %w", path, err)
			}

			if err := db.DeleteFileTracking(path); err != nil {
				return fmt.Errorf("failed to delete tracking for %s: %w", path, err)
			}
		}
	}

	return nil
}

// FullReindex clears the entire index and rebuilds it from scratch.
func (idx *Indexer) FullReindex(fs *storage.FileSystem) error {
	if fs == nil {
		return fmt.Errorf("filesystem is required for full reindex")
	}

	// Clear the existing index
	if err := idx.engine.Clear(); err != nil {
		return fmt.Errorf("failed to clear index: %w", err)
	}

	// Index all markdown files from the project root
	return idx.indexDirectoryWithFS(fs, ".", "document")
}

// FullReindexWithDB clears the entire index and tracking, then rebuilds from scratch.
func (idx *Indexer) FullReindexWithDB(fs *storage.FileSystem, db *storage.SQLiteDB) error {
	if fs == nil {
		return fmt.Errorf("filesystem is required for full reindex")
	}
	if db == nil {
		return fmt.Errorf("database is required for full reindex")
	}

	// Clear the existing index
	if err := idx.engine.Clear(); err != nil {
		return fmt.Errorf("failed to clear index: %w", err)
	}

	// Clear all file tracking
	trackedFiles, err := db.GetAllTrackedFiles()
	if err != nil {
		return fmt.Errorf("failed to get tracked files: %w", err)
	}

	for _, tf := range trackedFiles {
		if err := db.DeleteFileTracking(tf.Path); err != nil {
			return fmt.Errorf("failed to delete tracking for %s: %w", tf.Path, err)
		}
	}

	// Get all markdown files
	files, err := fs.ListMarkdownFiles(".")
	if err != nil {
		return fmt.Errorf("failed to list markdown files: %w", err)
	}

	// Index each file
	for _, file := range files {
		sourceType := determineSourceType(file.Path)

		if err := idx.indexFileWithFS(fs, file.Path, sourceType); err != nil {
			return fmt.Errorf("failed to index %s: %w", file.Path, err)
		}

		if err := db.UpdateFileTracking(file.Path, file.ModTime); err != nil {
			return fmt.Errorf("failed to update tracking for %s: %w", file.Path, err)
		}
	}

	return nil
}

// chunkContent splits content into overlapping chunks using the token counter.
func (idx *Indexer) chunkContent(content string) []string {
	if content == "" {
		return nil
	}

	chunks := idx.counter.Split(content, idx.chunkSize, idx.chunkOverlap)
	if len(chunks) == 0 {
		return nil
	}

	return chunks
}

// generateChunkID creates a unique identifier for a chunk based on file path and index.
func generateChunkID(path string, index int) string {
	data := fmt.Sprintf("%s:%d", path, index)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// determineSourceType infers the source type from the file path.
func determineSourceType(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(dir)

	switch base {
	case "characters":
		return SourceTypeCharacter
	case "world", "settings":
		return SourceTypeSetting
	case "plots":
		return SourceTypePlot
	case "chapters":
		return SourceTypeChapter
	default:
		return "document"
	}
}

// ChunkSize returns the current chunk size setting.
func (idx *Indexer) ChunkSize() int {
	return idx.chunkSize
}

// ChunkOverlap returns the current chunk overlap setting.
func (idx *Indexer) ChunkOverlap() float64 {
	return idx.chunkOverlap
}
