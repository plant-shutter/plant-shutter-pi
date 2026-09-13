package model

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"plant-shutter-pi/pkg/storage/consts"
	"plant-shutter-pi/pkg/utils"
)

type Project struct {
	ProjectEntity

	rootDir string
}

func (p *Project) SetRootDir(dir string) {
	p.rootDir = path.Join(dir, p.Name)
}

func New(name, info string, interval int32, rootDir string, camera CameraSettings) (*Project, error) {
	p := &Project{
		ProjectEntity: ProjectEntity{
			Name:           name,
			Info:           info,
			Interval:       interval,
			CameraSettings: camera,
			CreatedAt:      time.Now(),
		},
	}
	p.SetRootDir(rootDir)
	err := p.initStorage()
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Project) initStorage() error {
	err := utils.MkdirAll(
		p.getImageDirPath(),
	)
	if err != nil {
		return fmt.Errorf("create image dir: %w", err)
	}

	return nil
}

func (p *Project) SaveImage(image []byte) error {
	name := p.generateImageName(image, p.ImageCount)
	if err := os.WriteFile(p.GetImagePath(name), image, consts.DefaultFilePerm); err != nil {
		return err
	}

	p.ImageCount++
	p.LatestImageName = name

	return nil
}

func (p *Project) GetLatestImageName() (string, error) {
	return p.LatestImageName, nil
}

func (p *Project) GetLatestImage() ([]byte, error) {
	return p.GetImage(p.LatestImageName)
}

func (p *Project) GetImage(name string) ([]byte, error) {
	file, err := os.ReadFile(path.Join(p.getImageDirPath(), name))
	if err != nil {
		return nil, fmt.Errorf("picture not found, %w", err)
	}

	return file, nil
}

func (p *Project) ListImages(fun func(info fs.FileInfo) error) error {
	return listFiles(p.getImageDirPath(), consts.DefaultImageExt, fun)
}

func (p *Project) Clear() error {
	return os.RemoveAll(p.rootDir)
}

func (p *Project) Cleaned() bool {
	return p.ImageCount == 0
}

func (p *Project) ClearImages() error {
	err := os.RemoveAll(p.getImageDirPath())
	if err != nil {
		return err
	}

	return p.initStorage()
}

func (p *Project) generateImageName(image []byte, number int) string {
	return fmt.Sprintf("%s-%07d%s", p.Name, number, consts.DefaultImageExt)
}

func (p *Project) GetRootPath() string {
	return p.rootDir
}

func (p *Project) GetImagePath(name string) string {
	return path.Join(p.rootDir, consts.DefaultImagesDir, name)
}

func (p *Project) getImageDirPath() string {
	return path.Join(p.rootDir, consts.DefaultImagesDir)
}

func listFiles(dir string, ext string, fun func(info fs.FileInfo) error) error {
	if fun == nil {
		return nil
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ext) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return err
		}
		if err := fun(info); err != nil {
			return err
		}
	}

	return nil
}
