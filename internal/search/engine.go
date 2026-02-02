// Package search provides search functionality for story content.
package search

import (
	"errors"
	"time"
)

// Common errors returned by search operations.
var (
	// ErrDocumentNotFound is returned when a document cannot be found by ID.
	ErrDocumentNotFound = errors.New("document not found")

	// ErrIndexCorrupted is returned when the search index is in an invalid state.
	ErrIndexCorrupted = errors.New("search index corrupted")
)

// SourceType constants for document categorization.
const (
	SourceTypeCharacter = "character"
	SourceTypeSetting   = "setting"
	SourceTypePlot      = "plot"
	SourceTypeChapter   = "chapter"
)

// SearchEngine defines the interface for search operations.
// Implementations should be safe for concurrent use.
type SearchEngine interface {
	// Search performs a search query and returns matching documents.
	// Returns an empty slice if no documents match.
	Search(query string, opts SearchOptions) ([]SearchResult, error)

	// Index adds or updates a document in the search index.
	// If a document with the same ID exists, it is replaced.
	Index(doc Document) error

	// Delete removes a document from the search index by ID.
	// Returns ErrDocumentNotFound if the document does not exist.
	Delete(docID string) error

	// Reindex replaces the entire index with the provided documents.
	// This is an atomic operation: either all documents are indexed or none.
	Reindex(docs []Document) error

	// Close releases any resources held by the search engine.
	Close() error
}

// SearchOptions configures search behavior.
type SearchOptions struct {
	// Limit is the maximum number of results to return.
	// If 0, a default limit is applied.
	Limit int

	// Offset is the number of results to skip for pagination.
	Offset int

	// FilterType restricts results to a specific source type.
	// Valid values: "character", "setting", "plot", "chapter".
	// Empty string matches all types.
	FilterType string

	// MinScore is the minimum relevance score for results (0.0-1.0).
	// Documents with scores below this threshold are excluded.
	MinScore float64
}

// SearchResult represents a single search result with relevance information.
type SearchResult struct {
	// Document is the matched document.
	Document Document

	// Score is the relevance score for this result (0.0-1.0).
	// Higher scores indicate better matches.
	Score float64

	// Highlights contains text snippets showing where matches occurred.
	// Each string is a fragment of the document content with matches marked.
	Highlights []string
}

// Document represents a searchable content unit.
type Document struct {
	// ID is a unique identifier for the document.
	ID string

	// Content is the searchable text content.
	Content string

	// SourceType indicates the content category.
	// Values: "character", "setting", "plot", "chapter".
	SourceType string

	// SourcePath is the file path or URI of the source content.
	SourcePath string

	// Metadata contains additional key-value pairs for filtering and display.
	Metadata map[string]string

	// TokenCount is the number of tokens in the content.
	// Used for context budget calculations.
	TokenCount int

	// ModTime is when the source content was last modified.
	ModTime time.Time
}

// DefaultSearchOptions returns SearchOptions with sensible defaults.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		Limit:    20,
		Offset:   0,
		MinScore: 0.0,
	}
}

// WithLimit returns a copy of the options with the specified limit.
func (o SearchOptions) WithLimit(limit int) SearchOptions {
	return SearchOptions{
		Limit:      limit,
		Offset:     o.Offset,
		FilterType: o.FilterType,
		MinScore:   o.MinScore,
	}
}

// WithOffset returns a copy of the options with the specified offset.
func (o SearchOptions) WithOffset(offset int) SearchOptions {
	return SearchOptions{
		Limit:      o.Limit,
		Offset:     offset,
		FilterType: o.FilterType,
		MinScore:   o.MinScore,
	}
}

// WithFilterType returns a copy of the options with the specified filter type.
func (o SearchOptions) WithFilterType(filterType string) SearchOptions {
	return SearchOptions{
		Limit:      o.Limit,
		Offset:     o.Offset,
		FilterType: filterType,
		MinScore:   o.MinScore,
	}
}

// WithMinScore returns a copy of the options with the specified minimum score.
func (o SearchOptions) WithMinScore(minScore float64) SearchOptions {
	return SearchOptions{
		Limit:      o.Limit,
		Offset:     o.Offset,
		FilterType: o.FilterType,
		MinScore:   minScore,
	}
}

// IsValidSourceType returns true if the given type is a valid source type.
func IsValidSourceType(sourceType string) bool {
	switch sourceType {
	case SourceTypeCharacter, SourceTypeSetting, SourceTypePlot, SourceTypeChapter, "":
		return true
	default:
		return false
	}
}

// NewDocument creates a new Document with the required fields.
func NewDocument(id, content, sourceType, sourcePath string) Document {
	return Document{
		ID:         id,
		Content:    content,
		SourceType: sourceType,
		SourcePath: sourcePath,
		Metadata:   make(map[string]string),
		ModTime:    time.Now(),
	}
}

// WithMetadata returns a copy of the document with the specified metadata.
func (d Document) WithMetadata(key, value string) Document {
	metadata := make(map[string]string, len(d.Metadata)+1)
	for k, v := range d.Metadata {
		metadata[k] = v
	}
	metadata[key] = value

	return Document{
		ID:         d.ID,
		Content:    d.Content,
		SourceType: d.SourceType,
		SourcePath: d.SourcePath,
		Metadata:   metadata,
		TokenCount: d.TokenCount,
		ModTime:    d.ModTime,
	}
}

// WithTokenCount returns a copy of the document with the specified token count.
func (d Document) WithTokenCount(count int) Document {
	return Document{
		ID:         d.ID,
		Content:    d.Content,
		SourceType: d.SourceType,
		SourcePath: d.SourcePath,
		Metadata:   d.Metadata,
		TokenCount: count,
		ModTime:    d.ModTime,
	}
}

// WithModTime returns a copy of the document with the specified modification time.
func (d Document) WithModTime(modTime time.Time) Document {
	return Document{
		ID:         d.ID,
		Content:    d.Content,
		SourceType: d.SourceType,
		SourcePath: d.SourcePath,
		Metadata:   d.Metadata,
		TokenCount: d.TokenCount,
		ModTime:    modTime,
	}
}
