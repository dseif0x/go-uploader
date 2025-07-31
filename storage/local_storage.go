package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	BasePath string
}

func NewLocalStorage(path string) (*LocalStorage, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{BasePath: path}, nil
}

func (l *LocalStorage) SaveFile(name string, data io.Reader) error {
	err := os.MkdirAll(filepath.Dir(filepath.Join(l.BasePath, name)), 0755)
	if err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	fullPath := filepath.Join(l.BasePath, name)
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()
	_, err = io.Copy(f, data)
	return err
}
