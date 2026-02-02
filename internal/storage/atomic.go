// Package storage provides file and database handling.
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AtomicWriter provides crash-safe file writing using temp file + rename.
type AtomicWriter struct {
	targetPath string
	tempFile   *os.File
}

// NewAtomicWriter creates a new atomic writer for the target path.
func NewAtomicWriter(targetPath string) (*AtomicWriter, error) {
	dir := filepath.Dir(targetPath)

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	return &AtomicWriter{
		targetPath: targetPath,
		tempFile:   tempFile,
	}, nil
}

// Write implements io.Writer.
func (w *AtomicWriter) Write(p []byte) (n int, err error) {
	return w.tempFile.Write(p)
}

// Commit syncs and renames the temp file to the target path.
func (w *AtomicWriter) Commit() error {
	tempPath := w.tempFile.Name()

	// Sync to ensure data is on disk
	if err := w.tempFile.Sync(); err != nil {
		w.tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := w.tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, w.targetPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Abort cancels the write and cleans up the temp file.
func (w *AtomicWriter) Abort() error {
	tempPath := w.tempFile.Name()
	w.tempFile.Close()
	return os.Remove(tempPath)
}

// AtomicWriteFile writes data to a file atomically.
func AtomicWriteFile(path string, data []byte) error {
	writer, err := NewAtomicWriter(path)
	if err != nil {
		return err
	}

	if _, err := writer.Write(data); err != nil {
		writer.Abort()
		return err
	}

	return writer.Commit()
}

// AtomicCopyFile copies a file atomically.
func AtomicCopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	writer, err := NewAtomicWriter(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(writer, srcFile); err != nil {
		writer.Abort()
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return writer.Commit()
}
