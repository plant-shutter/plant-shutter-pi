package project

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"plant-shutter-pi/pkg/camera"
	"plant-shutter-pi/pkg/storage/consts"
	"plant-shutter-pi/pkg/storage/util"
)

type Project struct {
	Name string `json:"name"`
	Info string `json:"info"`
	// ms
	Interval int `json:"interval"`

	Settings camera.Settings `json:"config"`

	CreatedAt time.Time `json:"createdAt"`

	rootDir string
}

type ImagesInfo struct {
	MaxNumber   int    `json:"maxNumber"`
	LatestImage string `json:"latestImage"`

	UpdateAt time.Time `json:"updateAt"`
}

func (p *Project) SetRootDir(dir string) {
	p.rootDir = path.Join(dir, p.Name)
}

func New(name, info string, interval int, rootDir string, settings camera.Settings) (*Project, error) {
	p := &Project{
		Name:      name,
		Info:      info,
		Interval:  interval,
		Settings:  settings,
		CreatedAt: time.Now(),
	}
	p.SetRootDir(rootDir)
	err := util.MkdirAll(
		p.getImageDirPath(),
		p.getVideoDirPath(),
	)
	if err != nil {
		return p, err
	}

	if err = p.dumpImageInfo(&ImagesInfo{}); err != nil {
		return p, err
	}
	// todo: dump video info

	return p, nil
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

func (p *Project) LatestImage() ([]byte, error) {
	info, err := p.loadImageInfo()
	if err != nil {
		return nil, err
	}

	return p.GetImage(info.LatestImage)
}

func (p *Project) GetImage(name string) ([]byte, error) {
	// todo 路径检查
	file, err := os.ReadFile(path.Join(p.getImageDirPath(), name))
	if err != nil {
		return nil, fmt.Errorf("picture not found, %w", err)
	}

	return file, nil
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
		if !strings.HasSuffix(file.Name(), consts.DefaultImageExt) {
			continue
		}
		res = append(res, file.Name())
	}

	return res, nil
}

func (p *Project) Clear() error {
	return os.RemoveAll(p.rootDir)
}

func (p *Project) generateImageName(image []byte, number int) string {
	// generate filenames using md5?
	// fmt.Sprintf("%x", md5.Sum(data))
	return fmt.Sprintf("%s-%d%s", p.Name, number, consts.DefaultImageExt)
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

	return os.WriteFile(p.getImageInfoPath(), data, consts.DefaultFilePerm)
}

func (p *Project) GetImagePath(name string) string {
	return path.Join(p.rootDir, consts.DefaultImagesDir, name)
}

func (p *Project) getImageInfoPath() string {
	return path.Join(p.rootDir, consts.DefaultImagesDir, consts.DefaultInfoFile)
}

func (p *Project) getImageDirPath() string {
	return path.Join(p.rootDir, consts.DefaultImagesDir)
}

func (p *Project) getVideoDirPath() string {
	return path.Join(p.rootDir, consts.DefaultVideosDir)
}
