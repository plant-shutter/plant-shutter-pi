package image

import (
	"crypto/md5"
	"fmt"
	"github.com/gabriel-vasile/mimetype"
	"io"
	"os"
	"path"
	"path/filepath"
)

const (
	DefaultImagesDir = "images"
	DefaultVideosDir = "images"
	DefaultInfoFile  = "info.json"
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

func (s *Storage) SaveImage(project string, fileName string, src io.Reader) error {
	dst := s.GetImagePath(project, fileName)

	return saveFile(dst, src)
}

func saveFile(dst string, src io.Reader) error {
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

func (s *Storage) GetImagePath(project string, fileName string) string {
	return path.Join(s.path, project, DefaultImagesDir, fileName)
}

func (s *Storage) GetVideoPath(project string, fileName string) string {
	return path.Join(s.path, project, DefaultVideosDir, fileName)
}

func generateName(suffix string, data []byte) string {
	return fmt.Sprintf("%x-%s%s", md5.Sum(data), suffix, mimetype.Detect(data).Extension())
}
