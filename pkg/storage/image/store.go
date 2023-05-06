package image

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

type Storage struct {
	path string
}

func New(path string) (*Storage, error) {
	if path == "" {
		return nil, fmt.Errorf("path can not be empty")
	}
	return &Storage{path: path}, nil
}

func (s *Storage) Save(projectID string, fileName string, src io.Reader) error {
	dst := s.GetPath(projectID, fileName)
	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

func (s *Storage) GetPath(project string, fileName string) string {
	return path.Join(s.path, project, fileName)
}
