package storage

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/goccy/go-json"
)

type Project struct {
	Name string `json:"name"`
	Info string `json:"info,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

type ImagesInfo struct {
	MaxNumber   int
	LatestImage string

	UpdateAt time.Time
}

func (p *Project) init() error {
	err := mkdirAll(
		p.getImageDirPath(),
		p.getVideoDirPath(),
	)
	if err != nil {
		return err
	}

	if err = p.dumpImageInfo(&ImagesInfo{}); err != nil {
		return err
	}
	// todo: dump video info

	return nil
}

func (p *Project) SaveImage(image []byte) error {
	info, err := p.loadImageInfo()
	if err != nil {
		return err
	}
	name := p.generateImageName(image, info.MaxNumber)
	if err = os.WriteFile(p.GetImagePath(name), image, 0660); err != nil {
		return err
	}

	info.MaxNumber++
	info.LatestImage = name
	if err = p.dumpImageInfo(info); err != nil {
		return err
	}

	return nil
}

func (p *Project) LatestImageName() (string, error) {
	info, err := p.loadImageInfo()
	if err != nil {
		return "", err
	}

	return info.LatestImage, nil
}

func (p *Project) ListImages() ([]string, error) {
	files, err := os.ReadDir(p.getImageDirPath())
	if err != nil {
		return nil, err
	}
	var res []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), DefaultImageExt) {
			continue
		}
		res = append(res, file.Name())
	}

	return res, nil
}

func (p *Project) generateImageName(image []byte, number int) string {
	// generate filenames using md5?
	return fmt.Sprintf("%s-%d%s", p.Name, number, DefaultImageExt)
}

func (p *Project) loadImageInfo() (*ImagesInfo, error) {
	data, err := os.ReadFile(p.getImageInfoPath())
	if err != nil {
		return nil, fmt.Errorf("read image info err: %w", err)
	}
	info := &ImagesInfo{}
	if err = json.Unmarshal(data, info); err != nil {
		return nil, fmt.Errorf("unmarshal image info err: %w", err)
	}

	return info, nil
}

func (p *Project) dumpImageInfo(info *ImagesInfo) error {
	info.UpdateAt = time.Now()
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return os.WriteFile(p.getImageInfoPath(), data, DefaultFilePerm)
}

func (p *Project) GetImagePath(name string) string {
	return path.Join(storagePath, p.Name, DefaultImagesDir, name)
}

func (p *Project) getImageInfoPath() string {
	return path.Join(storagePath, p.Name, DefaultImagesDir, DefaultInfoFile)
}

func (p *Project) getImageDirPath() string {
	return path.Join(storagePath, p.Name, DefaultImagesDir)
}

func (p *Project) getVideoDirPath() string {
	return path.Join(storagePath, p.Name, DefaultImagesDir)
}

func mkdirAll(dirs ...string) error {
	for _, d := range dirs {
		err := os.MkdirAll(d, DefaultDirPerm)
		if err != nil {
			return err
		}
	}
	return nil
}
