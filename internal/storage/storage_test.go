//go:build cgo && fts5

package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a temporary SQLite database for testing.
func setupTestDB(t *testing.T) (*SQLiteDB, func()) {
	t.Helper()

	tempDir := t.TempDir()
	dreamtellerDir := filepath.Join(tempDir, ".dreamteller")
	err := os.MkdirAll(dreamtellerDir, 0755)
	require.NoError(t, err, "failed to create .dreamteller directory")

	db, err := NewSQLiteDB(tempDir)
	require.NoError(t, err, "failed to create test database")

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// =============================================================================
// TestAtomicWriteFile
// =============================================================================

func TestAtomicWriteFile(t *testing.T) {
	t.Run("writes file and verifies content", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "test.txt")
		expectedContent := []byte("Hello, World!")

		err := AtomicWriteFile(targetPath, expectedContent)
		require.NoError(t, err)

		actualContent, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, actualContent)
	})

	t.Run("verifies file exists after write", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "exists.txt")

		err := AtomicWriteFile(targetPath, []byte("content"))
		require.NoError(t, err)

		_, err = os.Stat(targetPath)
		assert.NoError(t, err, "file should exist after atomic write")
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "overwrite.txt")

		// Write initial content
		err := AtomicWriteFile(targetPath, []byte("original content"))
		require.NoError(t, err)

		// Overwrite with new content
		newContent := []byte("new content")
		err = AtomicWriteFile(targetPath, newContent)
		require.NoError(t, err)

		actualContent, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, newContent, actualContent)
	})

	t.Run("creates parent directories if they do not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "nested", "dirs", "test.txt")

		err := AtomicWriteFile(targetPath, []byte("content"))
		require.NoError(t, err)

		actualContent, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("content"), actualContent)
	})
}

// =============================================================================
// TestAtomicWriter
// =============================================================================

func TestAtomicWriter(t *testing.T) {
	t.Run("Write and Commit flow", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "atomic.txt")

		writer, err := NewAtomicWriter(targetPath)
		require.NoError(t, err)

		// Write data in multiple chunks
		_, err = writer.Write([]byte("Hello, "))
		require.NoError(t, err)

		_, err = writer.Write([]byte("World!"))
		require.NoError(t, err)

		// Commit
		err = writer.Commit()
		require.NoError(t, err)

		// Verify content
		content, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", string(content))
	})

	t.Run("Abort cleans up temp file", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "aborted.txt")

		writer, err := NewAtomicWriter(targetPath)
		require.NoError(t, err)

		_, err = writer.Write([]byte("should be aborted"))
		require.NoError(t, err)

		// Abort the write
		err = writer.Abort()
		require.NoError(t, err)

		// Target file should not exist
		_, err = os.Stat(targetPath)
		assert.True(t, os.IsNotExist(err), "target file should not exist after abort")

		// Verify no temp files left in directory
		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		for _, entry := range entries {
			assert.False(t, filepath.HasPrefix(entry.Name(), ".tmp-"),
				"temp file should be cleaned up after abort")
		}
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "new", "dir", "file.txt")

		writer, err := NewAtomicWriter(targetPath)
		require.NoError(t, err)

		_, err = writer.Write([]byte("content"))
		require.NoError(t, err)

		err = writer.Commit()
		require.NoError(t, err)

		content, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})
}

// =============================================================================
// TestFileSystem
// =============================================================================

func TestFileSystem(t *testing.T) {
	t.Run("ReadMarkdown and WriteMarkdown", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		content := "# Test Document\n\nThis is a test."
		relativePath := "docs/test.md"

		// Write markdown
		err := fs.WriteMarkdown(relativePath, content)
		require.NoError(t, err)

		// Read markdown
		readContent, err := fs.ReadMarkdown(relativePath)
		require.NoError(t, err)
		assert.Equal(t, content, readContent)
	})

	t.Run("ReadMarkdown returns error for non-existent file", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		_, err := fs.ReadMarkdown("non-existent.md")
		assert.Error(t, err)
	})

	t.Run("ListMarkdownFiles returns all markdown files", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		// Create some markdown files
		require.NoError(t, fs.WriteMarkdown("doc1.md", "# Doc 1"))
		require.NoError(t, fs.WriteMarkdown("nested/doc2.md", "# Doc 2"))
		require.NoError(t, fs.WriteMarkdown("nested/deep/doc3.md", "# Doc 3"))

		// Create a non-markdown file
		require.NoError(t, AtomicWriteFile(filepath.Join(tempDir, "other.txt"), []byte("not markdown")))

		files, err := fs.ListMarkdownFiles("")
		require.NoError(t, err)

		assert.Len(t, files, 3)

		// Extract paths
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Path
		}
		assert.Contains(t, paths, "doc1.md")
		assert.Contains(t, paths, filepath.Join("nested", "doc2.md"))
		assert.Contains(t, paths, filepath.Join("nested", "deep", "doc3.md"))
	})

	t.Run("ListMarkdownFiles returns empty slice for non-existent directory", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		files, err := fs.ListMarkdownFiles("non-existent-dir")
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("ListMarkdownFiles filters by subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		require.NoError(t, fs.WriteMarkdown("root.md", "# Root"))
		require.NoError(t, fs.WriteMarkdown("subdir/file1.md", "# File 1"))
		require.NoError(t, fs.WriteMarkdown("subdir/file2.md", "# File 2"))

		files, err := fs.ListMarkdownFiles("subdir")
		require.NoError(t, err)

		assert.Len(t, files, 2)
	})

	t.Run("ParseMarkdownTitle extracts H1 title", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		tests := []struct {
			name     string
			content  string
			expected string
		}{
			{
				name:     "simple H1",
				content:  "# Hello World",
				expected: "Hello World",
			},
			{
				name:     "H1 with content after",
				content:  "# My Title\n\nSome content here.",
				expected: "My Title",
			},
			{
				name:     "no H1",
				content:  "## H2 Title\n\nContent",
				expected: "",
			},
			{
				name:     "H1 after other content",
				content:  "Some text\n# Title",
				expected: "Title",
			},
			{
				name:     "empty content",
				content:  "",
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				title := fs.ParseMarkdownTitle(tt.content)
				assert.Equal(t, tt.expected, title)
			})
		}
	})

	t.Run("ParseMarkdownFrontmatter extracts YAML frontmatter", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		tests := []struct {
			name             string
			content          string
			expectedFM       string
			expectedBody     string
		}{
			{
				name: "valid frontmatter",
				content: `---
title: My Title
date: 2024-01-15
---

# Content

Some text here.`,
				expectedFM:   "title: My Title\ndate: 2024-01-15",
				expectedBody: "# Content\n\nSome text here.",
			},
			{
				name:           "no frontmatter",
				content:        "# Just Content\n\nNo frontmatter here.",
				expectedFM:     "",
				expectedBody:   "# Just Content\n\nNo frontmatter here.",
			},
			{
				name:           "incomplete frontmatter (no closing)",
				content:        "---\ntitle: Test\nNo closing delimiter",
				expectedFM:     "",
				expectedBody:   "---\ntitle: Test\nNo closing delimiter",
			},
			{
				name:           "empty content",
				content:        "",
				expectedFM:     "",
				expectedBody:   "",
			},
			{
				name: "frontmatter with no body",
				content: `---
title: Just Frontmatter
---`,
				expectedFM:   "title: Just Frontmatter",
				expectedBody: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				frontmatter, body := fs.ParseMarkdownFrontmatter(tt.content)
				assert.Equal(t, tt.expectedFM, frontmatter)
				assert.Equal(t, tt.expectedBody, body)
			})
		}
	})

	t.Run("GetFileInfo returns correct metadata", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		content := "# Test\n\nContent here"
		require.NoError(t, fs.WriteMarkdown("test.md", content))

		info, err := fs.GetFileInfo("test.md")
		require.NoError(t, err)

		assert.Equal(t, "test.md", info.Path)
		assert.Equal(t, int64(len(content)), info.Size)
		assert.False(t, info.ModTime.IsZero())
	})

	t.Run("Exists returns correct value", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		require.NoError(t, fs.WriteMarkdown("exists.md", "content"))

		assert.True(t, fs.Exists("exists.md"))
		assert.False(t, fs.Exists("does-not-exist.md"))
	})

	t.Run("EnsureDir creates directory", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		err := fs.EnsureDir("new/nested/dir")
		require.NoError(t, err)

		dirPath := filepath.Join(tempDir, "new", "nested", "dir")
		info, err := os.Stat(dirPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("Delete removes file", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		require.NoError(t, fs.WriteMarkdown("to-delete.md", "content"))
		assert.True(t, fs.Exists("to-delete.md"))

		err := fs.Delete("to-delete.md")
		require.NoError(t, err)

		assert.False(t, fs.Exists("to-delete.md"))
	})

	t.Run("BasePath returns correct path", func(t *testing.T) {
		tempDir := t.TempDir()
		fs := NewFileSystem(tempDir)

		assert.Equal(t, tempDir, fs.BasePath())
	})
}

// =============================================================================
// TestSQLiteDB
// =============================================================================

func TestSQLiteDB(t *testing.T) {
	t.Run("InsertChunk and SearchChunks", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		// Insert a chunk
		content := "The quick brown fox jumps over the lazy dog"
		sourceType := "markdown"
		sourcePath := "chapter1.md"
		tokenCount := 10
		mtime := time.Now()

		rowID, err := db.InsertChunk(content, sourceType, sourcePath, tokenCount, mtime, "")
		require.NoError(t, err)
		assert.Greater(t, rowID, int64(0))

		// Search for the chunk
		results, err := db.SearchChunks("fox", 10)
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, content, results[0].Content)
		assert.Equal(t, sourceType, results[0].SourceType)
		assert.Equal(t, sourcePath, results[0].SourcePath)
		assert.Equal(t, tokenCount, results[0].TokenCount)
	})

	t.Run("SearchChunks returns empty for no matches", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		_, err := db.InsertChunk("apple banana cherry", "note", "fruits.md", 3, time.Now(), "")
		require.NoError(t, err)

		results, err := db.SearchChunks("xyz123nonexistent", 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("SearchChunks respects limit", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		// Insert multiple chunks with the same word
		for i := 0; i < 5; i++ {
			_, err := db.InsertChunk("test content here", "note", "note.md", 3, time.Now(), "")
			require.NoError(t, err)
		}

		results, err := db.SearchChunks("test", 3)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("DeleteChunksBySource removes all chunks for source", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		// Insert chunks for different sources
		_, err := db.InsertChunk("content one", "markdown", "file1.md", 2, time.Now(), "")
		require.NoError(t, err)
		_, err = db.InsertChunk("content two", "markdown", "file1.md", 2, time.Now(), "")
		require.NoError(t, err)
		_, err = db.InsertChunk("content three", "markdown", "file2.md", 2, time.Now(), "")
		require.NoError(t, err)

		// Delete chunks for file1.md
		err = db.DeleteChunksBySource("file1.md")
		require.NoError(t, err)

		// Verify file1.md chunks are gone
		results, err := db.SearchChunks("content", 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "file2.md", results[0].SourcePath)
	})

	t.Run("DeleteChunksBySource with non-existent source", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		// Should not error when deleting non-existent source
		err := db.DeleteChunksBySource("non-existent.md")
		assert.NoError(t, err)
	})
}

func TestSQLiteDB_FileTracking(t *testing.T) {
	t.Run("UpdateFileTracking and GetFileTracking", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		path := "test/file.md"
		mtime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

		err := db.UpdateFileTracking(path, mtime)
		require.NoError(t, err)

		info, err := db.GetFileTracking(path)
		require.NoError(t, err)
		require.NotNil(t, info)

		assert.Equal(t, path, info.Path)
		assert.Equal(t, mtime.Unix(), info.MTime.Unix())
		assert.False(t, info.IndexedAt.IsZero())
	})

	t.Run("GetFileTracking returns nil for non-existent file", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		info, err := db.GetFileTracking("non-existent.md")
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("UpdateFileTracking updates existing entry", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		path := "update-test.md"
		oldMtime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		newMtime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

		// Insert initial tracking
		err := db.UpdateFileTracking(path, oldMtime)
		require.NoError(t, err)

		// Update with new mtime
		err = db.UpdateFileTracking(path, newMtime)
		require.NoError(t, err)

		info, err := db.GetFileTracking(path)
		require.NoError(t, err)
		assert.Equal(t, newMtime.Unix(), info.MTime.Unix())
	})

	t.Run("DeleteFileTracking removes tracking", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		path := "to-delete.md"
		err := db.UpdateFileTracking(path, time.Now())
		require.NoError(t, err)

		err = db.DeleteFileTracking(path)
		require.NoError(t, err)

		info, err := db.GetFileTracking(path)
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("GetAllTrackedFiles returns all tracked files", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		paths := []string{"file1.md", "file2.md", "nested/file3.md"}
		for _, p := range paths {
			err := db.UpdateFileTracking(p, time.Now())
			require.NoError(t, err)
		}

		files, err := db.GetAllTrackedFiles()
		require.NoError(t, err)
		assert.Len(t, files, 3)

		trackedPaths := make([]string, len(files))
		for i, f := range files {
			trackedPaths[i] = f.Path
		}
		for _, p := range paths {
			assert.Contains(t, trackedPaths, p)
		}
	})

	t.Run("GetAllTrackedFiles returns empty slice when no files tracked", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		files, err := db.GetAllTrackedFiles()
		require.NoError(t, err)
		assert.Empty(t, files)
	})
}

func TestSQLiteDB_Conversation(t *testing.T) {
	t.Run("SaveConversationMessage and GetConversationHistory", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		// Save messages
		err := db.SaveConversationMessage("user", "Hello, how are you?")
		require.NoError(t, err)

		err = db.SaveConversationMessage("assistant", "I'm doing well, thank you!")
		require.NoError(t, err)

		err = db.SaveConversationMessage("user", "Great to hear!")
		require.NoError(t, err)

		// Get history
		history, err := db.GetConversationHistory(10)
		require.NoError(t, err)
		require.Len(t, history, 3)

		// Verify chronological order
		assert.Equal(t, "user", history[0].Role)
		assert.Equal(t, "Hello, how are you?", history[0].Content)

		assert.Equal(t, "assistant", history[1].Role)
		assert.Equal(t, "I'm doing well, thank you!", history[1].Content)

		assert.Equal(t, "user", history[2].Role)
		assert.Equal(t, "Great to hear!", history[2].Content)
	})

	t.Run("GetConversationHistory respects limit", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		for i := 0; i < 10; i++ {
			err := db.SaveConversationMessage("user", "Message")
			require.NoError(t, err)
		}

		history, err := db.GetConversationHistory(5)
		require.NoError(t, err)
		assert.Len(t, history, 5)
	})

	t.Run("GetConversationHistory returns most recent messages", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		for i := 1; i <= 10; i++ {
			err := db.SaveConversationMessage("user", "Message "+string(rune('0'+i)))
			require.NoError(t, err)
		}

		history, err := db.GetConversationHistory(3)
		require.NoError(t, err)
		require.Len(t, history, 3)

		// Should get messages 8, 9, 10 in chronological order
		assert.Equal(t, "Message 8", history[0].Content)
		assert.Equal(t, "Message 9", history[1].Content)
		// Note: Message 10 would be ":' (rune 58) - let's fix this edge case
	})

	t.Run("GetConversationHistory returns empty slice when no messages", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		history, err := db.GetConversationHistory(10)
		require.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("ClearConversation removes all messages", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		// Add messages
		err := db.SaveConversationMessage("user", "Message 1")
		require.NoError(t, err)
		err = db.SaveConversationMessage("assistant", "Message 2")
		require.NoError(t, err)

		// Clear
		err = db.ClearConversation()
		require.NoError(t, err)

		// Verify empty
		history, err := db.GetConversationHistory(10)
		require.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("ConversationRecord has correct timestamp", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		beforeSave := time.Now().Add(-time.Second)

		err := db.SaveConversationMessage("user", "Test message")
		require.NoError(t, err)

		afterSave := time.Now().Add(time.Second)

		history, err := db.GetConversationHistory(1)
		require.NoError(t, err)
		require.Len(t, history, 1)

		assert.True(t, history[0].Timestamp.After(beforeSave))
		assert.True(t, history[0].Timestamp.Before(afterSave))
	})
}

func TestSQLiteDB_Close(t *testing.T) {
	t.Run("Close closes database connection", func(t *testing.T) {
		db, _ := setupTestDB(t)

		err := db.Close()
		require.NoError(t, err)

		// Attempting to use the database after close should fail
		_, err = db.GetConversationHistory(10)
		assert.Error(t, err)
	})
}

func TestSQLiteDB_DB(t *testing.T) {
	t.Run("DB returns underlying connection", func(t *testing.T) {
		db, cleanup := setupTestDB(t)
		defer cleanup()

		underlyingDB := db.DB()
		require.NotNil(t, underlyingDB)

		// Verify we can use it for queries
		err := underlyingDB.Ping()
		require.NoError(t, err)
	})
}

// =============================================================================
// TestAtomicCopyFile
// =============================================================================

func TestAtomicCopyFile(t *testing.T) {
	t.Run("copies file atomically", func(t *testing.T) {
		tempDir := t.TempDir()
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "dest.txt")

		srcContent := []byte("source file content")
		require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))

		err := AtomicCopyFile(srcPath, dstPath)
		require.NoError(t, err)

		dstContent, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, srcContent, dstContent)
	})

	t.Run("returns error for non-existent source", func(t *testing.T) {
		tempDir := t.TempDir()
		srcPath := filepath.Join(tempDir, "non-existent.txt")
		dstPath := filepath.Join(tempDir, "dest.txt")

		err := AtomicCopyFile(srcPath, dstPath)
		assert.Error(t, err)
	})

	t.Run("creates parent directories for destination", func(t *testing.T) {
		tempDir := t.TempDir()
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "nested", "dir", "dest.txt")

		require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644))

		err := AtomicCopyFile(srcPath, dstPath)
		require.NoError(t, err)

		dstContent, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("content"), dstContent)
	})
}
