// Package storage provides file and database handling.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// FileInfo contains file metadata for synchronization.
type FileInfo struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// FileSystem provides file operations for a project.
type FileSystem struct {
	basePath string
	md       goldmark.Markdown
}

// NewFileSystem creates a new file system handler.
func NewFileSystem(basePath string) *FileSystem {
	return &FileSystem{
		basePath: basePath,
		md:       goldmark.New(),
	}
}

// ReadMarkdown reads and parses a markdown file.
func (fs *FileSystem) ReadMarkdown(relativePath string) (string, error) {
	fullPath := filepath.Join(fs.basePath, relativePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read markdown file: %w", err)
	}
	return string(data), nil
}

// WriteMarkdown writes content to a markdown file atomically.
func (fs *FileSystem) WriteMarkdown(relativePath, content string) error {
	fullPath := filepath.Join(fs.basePath, relativePath)
	return AtomicWriteFile(fullPath, []byte(content))
}

// ListMarkdownFiles lists all markdown files in a directory.
func (fs *FileSystem) ListMarkdownFiles(relativePath string) ([]FileInfo, error) {
	dirPath := filepath.Join(fs.basePath, relativePath)

	var files []FileInfo
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(strings.ToLower(path), ".md") {
			relPath, _ := filepath.Rel(fs.basePath, path)
			files = append(files, FileInfo{
				Path:    relPath,
				ModTime: info.ModTime(),
				Size:    info.Size(),
			})
		}

		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return []FileInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list markdown files: %w", err)
	}

	return files, nil
}

// GetFileInfo returns file metadata.
func (fs *FileSystem) GetFileInfo(relativePath string) (*FileInfo, error) {
	fullPath := filepath.Join(fs.basePath, relativePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	return &FileInfo{
		Path:    relativePath,
		ModTime: info.ModTime(),
		Size:    info.Size(),
	}, nil
}

// EnsureDir ensures a directory exists.
func (fs *FileSystem) EnsureDir(relativePath string) error {
	fullPath := filepath.Join(fs.basePath, relativePath)
	return os.MkdirAll(fullPath, 0755)
}

// Exists checks if a file or directory exists.
func (fs *FileSystem) Exists(relativePath string) bool {
	fullPath := filepath.Join(fs.basePath, relativePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// Delete removes a file.
func (fs *FileSystem) Delete(relativePath string) error {
	fullPath := filepath.Join(fs.basePath, relativePath)
	return os.Remove(fullPath)
}

// ParseMarkdownTitle extracts the first H1 title from markdown content.
func (fs *FileSystem) ParseMarkdownTitle(content string) string {
	reader := text.NewReader([]byte(content))
	doc := fs.md.Parser().Parse(reader)

	var title string
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if heading, ok := n.(*ast.Heading); ok && entering && heading.Level == 1 {
			title = string(heading.Text([]byte(content)))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})

	return title
}

// ParseMarkdownFrontmatter extracts YAML frontmatter from markdown.
func (fs *FileSystem) ParseMarkdownFrontmatter(content string) (string, string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", content
	}

	var frontmatterEnd int
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			frontmatterEnd = i
			break
		}
	}

	if frontmatterEnd == 0 {
		return "", content
	}

	frontmatter := strings.Join(lines[1:frontmatterEnd], "\n")
	body := strings.Join(lines[frontmatterEnd+1:], "\n")

	return frontmatter, strings.TrimSpace(body)
}

// BasePath returns the base path of the filesystem.
func (fs *FileSystem) BasePath() string {
	return fs.basePath
}
