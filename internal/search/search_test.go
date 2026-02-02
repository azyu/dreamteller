package search

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/azyu/dreamteller/internal/storage"
)

// testDB creates a temporary SQLite database with FTS5 tables for testing.
// It returns the database, a cleanup function, and any error.
func testDB(t *testing.T) (*storage.SQLiteDB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dreamteller-test-*")
	require.NoError(t, err)

	// Create .dreamteller directory for the database
	dtDir := filepath.Join(tmpDir, ".dreamteller")
	err = os.MkdirAll(dtDir, 0755)
	require.NoError(t, err)

	db, err := storage.NewSQLiteDB(tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// mockTokenCounter implements TokenCounter for testing.
type mockTokenCounter struct {
	countFunc func(text string) int
	splitFunc func(text string, chunkSize int, overlap float64) []string
}

func (m *mockTokenCounter) Count(text string) int {
	if m.countFunc != nil {
		return m.countFunc(text)
	}
	// Default: roughly 1 token per 4 characters
	return len(text) / 4
}

func (m *mockTokenCounter) Split(text string, chunkSize int, overlap float64) []string {
	if m.splitFunc != nil {
		return m.splitFunc(text, chunkSize, overlap)
	}
	// Default: split into chunks of chunkSize characters
	if text == "" {
		return nil
	}
	charPerChunk := chunkSize * 4 // assuming 4 chars per token
	if charPerChunk <= 0 {
		charPerChunk = 100
	}
	var chunks []string
	for i := 0; i < len(text); i += charPerChunk {
		end := i + charPerChunk
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[i:end])
	}
	return chunks
}

// ============================================================================
// TestFTSEngine
// ============================================================================

func TestFTSEngine_Index_and_Search(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index some documents
	now := time.Now()
	err := engine.Index("The quick brown fox jumps over the lazy dog", SourceTypeChapter, "/chapters/ch1.md", 10, now, "{}")
	require.NoError(t, err)

	err = engine.Index("A lazy cat sleeps on the couch", SourceTypeChapter, "/chapters/ch2.md", 8, now, "{}")
	require.NoError(t, err)

	err = engine.Index("Dragons breathe fire and fly in the sky", SourceTypeCharacter, "/characters/dragon.md", 9, now, "{}")
	require.NoError(t, err)

	// Search for "lazy"
	results, err := engine.Search("lazy", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify results contain the expected content
	var foundFox, foundCat bool
	for _, r := range results {
		if r.SourcePath == "/chapters/ch1.md" {
			foundFox = true
			assert.Contains(t, r.Content, "lazy dog")
		}
		if r.SourcePath == "/chapters/ch2.md" {
			foundCat = true
			assert.Contains(t, r.Content, "lazy cat")
		}
	}
	assert.True(t, foundFox, "should find the fox document")
	assert.True(t, foundCat, "should find the cat document")

	// Search for "dragon" should only return one result
	results, err = engine.Search("dragon", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "/characters/dragon.md", results[0].SourcePath)
}

func TestFTSEngine_Search_EmptyQuery(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Empty query should return nil
	results, err := engine.Search("", 10)
	require.NoError(t, err)
	assert.Nil(t, results)

	// Whitespace-only query should return nil
	results, err = engine.Search("   ", 10)
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestFTSEngine_SearchWithFilter(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index documents with different source types
	now := time.Now()
	err := engine.Index("Hero is brave and strong", SourceTypeCharacter, "/characters/hero.md", 5, now, "{}")
	require.NoError(t, err)

	err = engine.Index("Villain is strong but evil", SourceTypeCharacter, "/characters/villain.md", 5, now, "{}")
	require.NoError(t, err)

	err = engine.Index("The castle is strong and ancient", SourceTypeSetting, "/world/castle.md", 6, now, "{}")
	require.NoError(t, err)

	err = engine.Index("The plot reveals strong conflict", SourceTypePlot, "/plots/main.md", 5, now, "{}")
	require.NoError(t, err)

	// Search for "strong" without filter should return all
	results, err := engine.Search("strong", 10)
	require.NoError(t, err)
	assert.Len(t, results, 4)

	// Search with filter for character type
	results, err = engine.SearchWithFilter("strong", SourceTypeCharacter, 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, SourceTypeCharacter, r.SourceType)
	}

	// Search with filter for setting type
	results, err = engine.SearchWithFilter("strong", SourceTypeSetting, 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, SourceTypeSetting, results[0].SourceType)
	assert.Equal(t, "/world/castle.md", results[0].SourcePath)

	// Search with filter for plot type
	results, err = engine.SearchWithFilter("strong", SourceTypePlot, 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, SourceTypePlot, results[0].SourceType)
}

func TestFTSEngine_DeleteBySource(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index documents
	now := time.Now()
	err := engine.Index("Chapter one content", SourceTypeChapter, "/chapters/ch1.md", 3, now, "{}")
	require.NoError(t, err)

	err = engine.Index("Chapter two content", SourceTypeChapter, "/chapters/ch2.md", 3, now, "{}")
	require.NoError(t, err)

	err = engine.Index("Chapter three content", SourceTypeChapter, "/chapters/ch3.md", 3, now, "{}")
	require.NoError(t, err)

	// Verify all are indexed
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Delete chapter 2
	err = engine.DeleteBySource("/chapters/ch2.md")
	require.NoError(t, err)

	// Verify count decreased
	count, err = engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Search should not find chapter two
	results, err := engine.Search("chapter", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	for _, r := range results {
		assert.NotEqual(t, "/chapters/ch2.md", r.SourcePath)
	}
}

func TestFTSEngine_DeleteBySource_NonExistent(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Delete non-existent source should not error
	err := engine.DeleteBySource("/nonexistent/file.md")
	require.NoError(t, err)
}

func TestFTSEngine_Reindex(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index some initial documents
	now := time.Now()
	err := engine.Index("Old content one", SourceTypeChapter, "/old/ch1.md", 3, now, "{}")
	require.NoError(t, err)

	err = engine.Index("Old content two", SourceTypeChapter, "/old/ch2.md", 3, now, "{}")
	require.NoError(t, err)

	// Verify initial state
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Reindex with new documents
	newChunks := []IndexableChunk{
		{
			Content:    "New content alpha",
			SourceType: SourceTypeCharacter,
			SourcePath: "/new/alpha.md",
			TokenCount: 3,
			MTime:      now,
			Metadata:   "{}",
		},
		{
			Content:    "New content beta",
			SourceType: SourceTypeSetting,
			SourcePath: "/new/beta.md",
			TokenCount: 3,
			MTime:      now,
			Metadata:   "{}",
		},
		{
			Content:    "New content gamma",
			SourceType: SourceTypePlot,
			SourcePath: "/new/gamma.md",
			TokenCount: 3,
			MTime:      now,
			Metadata:   "{}",
		},
	}

	err = engine.Reindex(newChunks)
	require.NoError(t, err)

	// Verify new state
	count, err = engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Old content should not be searchable
	results, err := engine.Search("old", 10)
	require.NoError(t, err)
	assert.Len(t, results, 0)

	// New content should be searchable
	results, err = engine.Search("new", 10)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify source types
	countByType, err := engine.GetChunkCountByType(SourceTypeCharacter)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countByType)

	countByType, err = engine.GetChunkCountByType(SourceTypeSetting)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countByType)

	countByType, err = engine.GetChunkCountByType(SourceTypePlot)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countByType)
}

func TestFTSEngine_Reindex_EmptyChunks(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index initial data
	now := time.Now()
	err := engine.Index("Some content", SourceTypeChapter, "/ch1.md", 2, now, "{}")
	require.NoError(t, err)

	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Reindex with empty slice should clear everything
	err = engine.Reindex([]IndexableChunk{})
	require.NoError(t, err)

	count, err = engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestFTSEngine_Clear(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index documents
	now := time.Now()
	err := engine.Index("Content one", SourceTypeChapter, "/ch1.md", 2, now, "{}")
	require.NoError(t, err)

	err = engine.Index("Content two", SourceTypeChapter, "/ch2.md", 2, now, "{}")
	require.NoError(t, err)

	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Clear the index
	err = engine.Clear()
	require.NoError(t, err)

	// Verify cleared
	count, err = engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// ============================================================================
// TestIndexer
// ============================================================================

func TestIndexer_chunkContent(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	counter := &mockTokenCounter{
		splitFunc: func(text string, chunkSize int, overlap float64) []string {
			// Split by double newlines for testing
			if text == "" {
				return nil
			}
			// Simple split: every 100 chars
			var chunks []string
			for i := 0; i < len(text); i += 100 {
				end := i + 100
				if end > len(text) {
					end = len(text)
				}
				chunks = append(chunks, text[i:end])
			}
			return chunks
		},
	}

	indexer := NewIndexer(engine, counter, 100, 0.1)

	tests := []struct {
		name        string
		content     string
		wantChunks  int
		description string
	}{
		{
			name:        "empty content returns nil",
			content:     "",
			wantChunks:  0,
			description: "empty string should return nil chunks",
		},
		{
			name:        "short content single chunk",
			content:     "Short text that fits in one chunk.",
			wantChunks:  1,
			description: "content shorter than chunk size should be single chunk",
		},
		{
			name:        "long content multiple chunks",
			content:     "This is a very long piece of content that should be split into multiple chunks. It contains many words and sentences that together form a larger document. The indexer should properly split this into several chunks based on the configured chunk size.",
			wantChunks:  3,
			description: "content longer than chunk size should be split",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := indexer.chunkContent(tt.content)
			if tt.wantChunks == 0 {
				assert.Nil(t, chunks)
			} else {
				assert.Len(t, chunks, tt.wantChunks, tt.description)
			}
		})
	}
}

func TestIndexer_chunkContent_WithCustomSplitter(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	customChunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
	counter := &mockTokenCounter{
		splitFunc: func(text string, chunkSize int, overlap float64) []string {
			if text == "" {
				return nil
			}
			return customChunks
		},
	}

	indexer := NewIndexer(engine, counter, 100, 0.15)

	chunks := indexer.chunkContent("any content")
	assert.Equal(t, customChunks, chunks)
}

func TestGenerateChunkID_Deterministic(t *testing.T) {
	// Same inputs should always produce same output
	path := "/path/to/file.md"
	index := 5

	id1 := generateChunkID(path, index)
	id2 := generateChunkID(path, index)
	id3 := generateChunkID(path, index)

	assert.Equal(t, id1, id2)
	assert.Equal(t, id2, id3)
	assert.Len(t, id1, 16) // 8 bytes = 16 hex chars
}

func TestGenerateChunkID_UniqueForDifferentInputs(t *testing.T) {
	// Different paths should produce different IDs
	id1 := generateChunkID("/path/one.md", 0)
	id2 := generateChunkID("/path/two.md", 0)
	assert.NotEqual(t, id1, id2)

	// Different indices should produce different IDs
	id3 := generateChunkID("/path/one.md", 0)
	id4 := generateChunkID("/path/one.md", 1)
	assert.NotEqual(t, id3, id4)
}

func TestDetermineSourceType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "characters directory",
			path:     "/project/characters/hero.md",
			expected: SourceTypeCharacter,
		},
		{
			name:     "world directory",
			path:     "/project/world/castle.md",
			expected: SourceTypeSetting,
		},
		{
			name:     "settings directory",
			path:     "/project/settings/town.md",
			expected: SourceTypeSetting,
		},
		{
			name:     "plots directory",
			path:     "/project/plots/main.md",
			expected: SourceTypePlot,
		},
		{
			name:     "chapters directory",
			path:     "/project/chapters/chapter1.md",
			expected: SourceTypeChapter,
		},
		{
			name:     "nested characters",
			path:     "/project/content/characters/villain.md",
			expected: SourceTypeCharacter,
		},
		{
			name:     "unknown directory",
			path:     "/project/notes/ideas.md",
			expected: "document",
		},
		{
			name:     "root level file",
			path:     "readme.md",
			expected: "document",
		},
		{
			name:     "deeply nested plots",
			path:     "/a/b/c/plots/story.md",
			expected: SourceTypePlot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineSourceType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIndexer_IndexFileWithContent(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	counter := &mockTokenCounter{
		countFunc: func(text string) int {
			return len(text) / 4
		},
		splitFunc: func(text string, chunkSize int, overlap float64) []string {
			if text == "" {
				return nil
			}
			// Return content as single chunk for simple testing
			return []string{text}
		},
	}

	indexer := NewIndexer(engine, counter, 800, 0.15)

	// Index file content
	mtime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	err := indexer.IndexFileWithContent(
		"/characters/hero.md",
		SourceTypeCharacter,
		"The hero is brave and strong.",
		mtime,
	)
	require.NoError(t, err)

	// Verify it was indexed
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Search should find it
	results, err := engine.Search("brave", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "/characters/hero.md", results[0].SourcePath)
	assert.Equal(t, SourceTypeCharacter, results[0].SourceType)
}

func TestIndexer_IndexFileWithContent_MultipleChunks(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	counter := &mockTokenCounter{
		countFunc: func(text string) int {
			return len(text) / 4
		},
		splitFunc: func(text string, chunkSize int, overlap float64) []string {
			if text == "" {
				return nil
			}
			// Split into 3 chunks for testing
			return []string{
				"First chunk content about the beginning.",
				"Second chunk content about the middle.",
				"Third chunk content about the end.",
			}
		},
	}

	indexer := NewIndexer(engine, counter, 800, 0.15)

	// Index file that will be split into multiple chunks
	mtime := time.Now()
	err := indexer.IndexFileWithContent(
		"/chapters/long.md",
		SourceTypeChapter,
		"Any content - the mock will split it into 3 chunks",
		mtime,
	)
	require.NoError(t, err)

	// Verify 3 chunks were indexed
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Each chunk should be searchable
	results, err := engine.Search("beginning", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	results, err = engine.Search("middle", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	results, err = engine.Search("end", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestIndexer_IndexFileWithContent_ReplacesExisting(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	counter := &mockTokenCounter{
		splitFunc: func(text string, chunkSize int, overlap float64) []string {
			if text == "" {
				return nil
			}
			return []string{text}
		},
	}

	indexer := NewIndexer(engine, counter, 800, 0.15)
	path := "/characters/hero.md"
	mtime := time.Now()

	// Index initial content
	err := indexer.IndexFileWithContent(path, SourceTypeCharacter, "Hero is brave", mtime)
	require.NoError(t, err)

	results, err := engine.Search("brave", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Index updated content (should replace)
	err = indexer.IndexFileWithContent(path, SourceTypeCharacter, "Hero is wise", mtime)
	require.NoError(t, err)

	// Old content should be gone
	results, err = engine.Search("brave", 10)
	require.NoError(t, err)
	assert.Len(t, results, 0)

	// New content should be searchable
	results, err = engine.Search("wise", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Still only one chunk
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestIndexer_IndexFileWithContent_EmptyContent(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	counter := &mockTokenCounter{
		splitFunc: func(text string, chunkSize int, overlap float64) []string {
			if text == "" {
				return nil
			}
			return []string{text}
		},
	}

	indexer := NewIndexer(engine, counter, 800, 0.15)

	// Index empty content should not error
	err := indexer.IndexFileWithContent("/empty.md", SourceTypeChapter, "", time.Now())
	require.NoError(t, err)

	// Nothing should be indexed
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestIndexer_DefaultValues(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	counter := &mockTokenCounter{}

	// Test with invalid values - should use defaults
	indexer := NewIndexer(engine, counter, -100, 2.0)
	assert.Equal(t, DefaultChunkSize, indexer.ChunkSize())
	assert.Equal(t, DefaultChunkOverlap, indexer.ChunkOverlap())

	// Test with zero chunk size
	indexer2 := NewIndexer(engine, counter, 0, -0.5)
	assert.Equal(t, DefaultChunkSize, indexer2.ChunkSize())
	assert.Equal(t, DefaultChunkOverlap, indexer2.ChunkOverlap())

	// Test with valid values
	indexer3 := NewIndexer(engine, counter, 500, 0.2)
	assert.Equal(t, 500, indexer3.ChunkSize())
	assert.Equal(t, 0.2, indexer3.ChunkOverlap())
}

// ============================================================================
// TestSearchOptions
// ============================================================================

func TestDefaultSearchOptions(t *testing.T) {
	opts := DefaultSearchOptions()

	assert.Equal(t, 20, opts.Limit)
	assert.Equal(t, 0, opts.Offset)
	assert.Equal(t, "", opts.FilterType)
	assert.Equal(t, 0.0, opts.MinScore)
}

func TestSearchOptions_WithLimit(t *testing.T) {
	opts := DefaultSearchOptions()

	// Apply WithLimit
	newOpts := opts.WithLimit(50)

	// Original unchanged
	assert.Equal(t, 20, opts.Limit)

	// New has updated value
	assert.Equal(t, 50, newOpts.Limit)

	// Other fields preserved
	assert.Equal(t, opts.Offset, newOpts.Offset)
	assert.Equal(t, opts.FilterType, newOpts.FilterType)
	assert.Equal(t, opts.MinScore, newOpts.MinScore)
}

func TestSearchOptions_WithOffset(t *testing.T) {
	opts := DefaultSearchOptions().WithLimit(100)

	newOpts := opts.WithOffset(25)

	// Original unchanged
	assert.Equal(t, 0, opts.Offset)

	// New has updated value
	assert.Equal(t, 25, newOpts.Offset)

	// Other fields preserved
	assert.Equal(t, 100, newOpts.Limit)
	assert.Equal(t, opts.FilterType, newOpts.FilterType)
	assert.Equal(t, opts.MinScore, newOpts.MinScore)
}

func TestSearchOptions_WithFilterType(t *testing.T) {
	opts := DefaultSearchOptions()

	newOpts := opts.WithFilterType(SourceTypeCharacter)

	// Original unchanged
	assert.Equal(t, "", opts.FilterType)

	// New has updated value
	assert.Equal(t, SourceTypeCharacter, newOpts.FilterType)

	// Other fields preserved
	assert.Equal(t, opts.Limit, newOpts.Limit)
	assert.Equal(t, opts.Offset, newOpts.Offset)
	assert.Equal(t, opts.MinScore, newOpts.MinScore)
}

func TestSearchOptions_WithMinScore(t *testing.T) {
	opts := DefaultSearchOptions()

	newOpts := opts.WithMinScore(0.5)

	// Original unchanged
	assert.Equal(t, 0.0, opts.MinScore)

	// New has updated value
	assert.Equal(t, 0.5, newOpts.MinScore)

	// Other fields preserved
	assert.Equal(t, opts.Limit, newOpts.Limit)
	assert.Equal(t, opts.Offset, newOpts.Offset)
	assert.Equal(t, opts.FilterType, newOpts.FilterType)
}

func TestSearchOptions_FluentChaining(t *testing.T) {
	opts := DefaultSearchOptions().
		WithLimit(50).
		WithOffset(10).
		WithFilterType(SourceTypeSetting).
		WithMinScore(0.75)

	assert.Equal(t, 50, opts.Limit)
	assert.Equal(t, 10, opts.Offset)
	assert.Equal(t, SourceTypeSetting, opts.FilterType)
	assert.Equal(t, 0.75, opts.MinScore)
}

func TestSearchOptions_Immutability(t *testing.T) {
	original := DefaultSearchOptions()

	// Apply multiple operations
	_ = original.WithLimit(100)
	_ = original.WithOffset(50)
	_ = original.WithFilterType(SourceTypeCharacter)
	_ = original.WithMinScore(0.9)

	// Original should be completely unchanged
	expected := DefaultSearchOptions()
	assert.Equal(t, expected.Limit, original.Limit)
	assert.Equal(t, expected.Offset, original.Offset)
	assert.Equal(t, expected.FilterType, original.FilterType)
	assert.Equal(t, expected.MinScore, original.MinScore)
}

// ============================================================================
// TestDocument
// ============================================================================

func TestNewDocument(t *testing.T) {
	doc := NewDocument(
		"doc-123",
		"Some content here",
		SourceTypeCharacter,
		"/characters/hero.md",
	)

	assert.Equal(t, "doc-123", doc.ID)
	assert.Equal(t, "Some content here", doc.Content)
	assert.Equal(t, SourceTypeCharacter, doc.SourceType)
	assert.Equal(t, "/characters/hero.md", doc.SourcePath)
	assert.NotNil(t, doc.Metadata)
	assert.Empty(t, doc.Metadata)
	assert.Equal(t, 0, doc.TokenCount)
	assert.False(t, doc.ModTime.IsZero())
}

func TestDocument_WithMetadata(t *testing.T) {
	doc := NewDocument("id", "content", SourceTypeChapter, "/path.md")

	// Add metadata
	newDoc := doc.WithMetadata("author", "John Doe")

	// Original unchanged
	assert.Empty(t, doc.Metadata)

	// New has metadata
	assert.Equal(t, "John Doe", newDoc.Metadata["author"])

	// Other fields preserved
	assert.Equal(t, doc.ID, newDoc.ID)
	assert.Equal(t, doc.Content, newDoc.Content)
	assert.Equal(t, doc.SourceType, newDoc.SourceType)
	assert.Equal(t, doc.SourcePath, newDoc.SourcePath)
}

func TestDocument_WithMetadata_MultipleKeys(t *testing.T) {
	doc := NewDocument("id", "content", SourceTypeChapter, "/path.md").
		WithMetadata("key1", "value1").
		WithMetadata("key2", "value2").
		WithMetadata("key3", "value3")

	assert.Equal(t, "value1", doc.Metadata["key1"])
	assert.Equal(t, "value2", doc.Metadata["key2"])
	assert.Equal(t, "value3", doc.Metadata["key3"])
}

func TestDocument_WithMetadata_OverwriteKey(t *testing.T) {
	doc := NewDocument("id", "content", SourceTypeChapter, "/path.md").
		WithMetadata("key", "original")

	newDoc := doc.WithMetadata("key", "updated")

	assert.Equal(t, "original", doc.Metadata["key"])
	assert.Equal(t, "updated", newDoc.Metadata["key"])
}

func TestDocument_WithTokenCount(t *testing.T) {
	doc := NewDocument("id", "content", SourceTypeChapter, "/path.md")
	assert.Equal(t, 0, doc.TokenCount)

	newDoc := doc.WithTokenCount(150)

	// Original unchanged
	assert.Equal(t, 0, doc.TokenCount)

	// New has token count
	assert.Equal(t, 150, newDoc.TokenCount)

	// Other fields preserved
	assert.Equal(t, doc.ID, newDoc.ID)
	assert.Equal(t, doc.Content, newDoc.Content)
}

func TestDocument_WithModTime(t *testing.T) {
	doc := NewDocument("id", "content", SourceTypeChapter, "/path.md")
	originalTime := doc.ModTime

	customTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	newDoc := doc.WithModTime(customTime)

	// Original unchanged
	assert.Equal(t, originalTime, doc.ModTime)

	// New has custom time
	assert.Equal(t, customTime, newDoc.ModTime)

	// Other fields preserved
	assert.Equal(t, doc.ID, newDoc.ID)
	assert.Equal(t, doc.Content, newDoc.Content)
}

func TestDocument_FluentChaining(t *testing.T) {
	customTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	doc := NewDocument("doc-1", "My story content", SourceTypeChapter, "/chapters/ch1.md").
		WithMetadata("title", "Chapter One").
		WithMetadata("wordCount", "5000").
		WithTokenCount(1250).
		WithModTime(customTime)

	assert.Equal(t, "doc-1", doc.ID)
	assert.Equal(t, "My story content", doc.Content)
	assert.Equal(t, SourceTypeChapter, doc.SourceType)
	assert.Equal(t, "/chapters/ch1.md", doc.SourcePath)
	assert.Equal(t, "Chapter One", doc.Metadata["title"])
	assert.Equal(t, "5000", doc.Metadata["wordCount"])
	assert.Equal(t, 1250, doc.TokenCount)
	assert.Equal(t, customTime, doc.ModTime)
}

func TestDocument_Immutability(t *testing.T) {
	original := NewDocument("id", "content", SourceTypeChapter, "/path.md")
	originalTime := original.ModTime

	// Apply multiple operations
	_ = original.WithMetadata("key", "value")
	_ = original.WithTokenCount(100)
	_ = original.WithModTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	// Original should be completely unchanged
	assert.Equal(t, "id", original.ID)
	assert.Equal(t, "content", original.Content)
	assert.Empty(t, original.Metadata)
	assert.Equal(t, 0, original.TokenCount)
	assert.Equal(t, originalTime, original.ModTime)
}

// ============================================================================
// TestIsValidSourceType
// ============================================================================

func TestIsValidSourceType(t *testing.T) {
	tests := []struct {
		sourceType string
		valid      bool
	}{
		{SourceTypeCharacter, true},
		{SourceTypeSetting, true},
		{SourceTypePlot, true},
		{SourceTypeChapter, true},
		{"", true}, // empty is valid (matches all)
		{"unknown", false},
		{"CHAPTER", false}, // case sensitive
		{"Character", false},
		{"document", false},
	}

	for _, tt := range tests {
		t.Run(tt.sourceType, func(t *testing.T) {
			result := IsValidSourceType(tt.sourceType)
			assert.Equal(t, tt.valid, result)
		})
	}
}

// ============================================================================
// TestSanitizeFTS5Query
// ============================================================================

func TestSanitizeFTS5Query(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple word",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "multiple words",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "removes special characters",
			input:    "hello* world^",
			expected: "hello world",
		},
		{
			name:     "removes quotes",
			input:    `"exact phrase"`,
			expected: "exact phrase",
		},
		{
			name:     "trims whitespace",
			input:    "   spaced   words   ",
			expected: "spaced words",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "removes parentheses",
			input:    "(grouped) words",
			expected: "grouped words",
		},
		{
			name:     "removes colons",
			input:    "field:value",
			expected: "fieldvalue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFTS5Query(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// TestFTSEngine_GetChunkByID
// ============================================================================

func TestFTSEngine_GetChunkByID(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index a document
	now := time.Now()
	err := engine.Index("Test content for retrieval", SourceTypeChapter, "/test.md", 5, now, "{}")
	require.NoError(t, err)

	// Get the chunk count to find the ID
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Search to get the ID
	results, err := engine.Search("retrieval", 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Get chunk by ID
	chunk, err := engine.GetChunkByID(results[0].ID)
	require.NoError(t, err)
	require.NotNil(t, chunk)

	assert.Equal(t, results[0].ID, chunk.ID)
	assert.Equal(t, "Test content for retrieval", chunk.Content)
	assert.Equal(t, SourceTypeChapter, chunk.SourceType)
	assert.Equal(t, "/test.md", chunk.SourcePath)
	assert.Equal(t, 5, chunk.TokenCount)
}

func TestFTSEngine_GetChunkByID_NotFound(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Try to get non-existent chunk
	chunk, err := engine.GetChunkByID(99999)
	require.NoError(t, err)
	assert.Nil(t, chunk)
}

// ============================================================================
// TestFTSEngine_SearchWithHighlight
// ============================================================================

func TestFTSEngine_SearchWithHighlight(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index document
	now := time.Now()
	err := engine.Index(
		"The brave knight fought the dragon in an epic battle at the castle gates.",
		SourceTypeChapter,
		"/chapter.md",
		15,
		now,
		"{}",
	)
	require.NoError(t, err)

	// Search with highlights
	results, err := engine.SearchWithHighlight("dragon", 10, "<<", ">>")
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Snippet should contain highlighted term
	assert.Contains(t, results[0].Snippet, "<<dragon>>")
}

func TestFTSEngine_SearchWithHighlight_DefaultMarkers(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)

	// Index document
	now := time.Now()
	err := engine.Index("The wizard cast a spell", SourceTypeChapter, "/ch.md", 5, now, "{}")
	require.NoError(t, err)

	// Search with empty markers (should use defaults)
	results, err := engine.SearchWithHighlight("wizard", 10, "", "")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Snippet, "**wizard**")
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestFTSEngine_ConcurrentOperations(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	engine := NewFTSEngine(db)
	now := time.Now()

	// Index some base content
	for i := 0; i < 10; i++ {
		err := engine.Index(
			"Base content for concurrent testing",
			SourceTypeChapter,
			"/base.md",
			5,
			now,
			"{}",
		)
		require.NoError(t, err)
	}

	// Verify all indexed
	count, err := engine.GetChunkCount()
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)
}

// testDBRaw creates a raw SQL database for lower-level testing
func testDBRaw(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	tmpFile.Close()

	db, err := sql.Open("sqlite3", tmpFile.Name()+"?_journal_mode=WAL")
	require.NoError(t, err)

	// Create FTS5 tables
	schema := `
		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			content,
			source_type,
			source_path,
			tokenize='porter unicode61'
		);

		CREATE TABLE IF NOT EXISTS chunks_meta (
			rowid INTEGER PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_path TEXT NOT NULL,
			token_count INTEGER NOT NULL,
			mtime INTEGER NOT NULL,
			metadata TEXT,
			created_at INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_chunks_meta_source
		ON chunks_meta(source_path);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	return db, cleanup
}
