package project

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"go.uber.org/zap"

	"plant-shutter-pi/pkg/storage/consts"
	"plant-shutter-pi/pkg/types"
	"plant-shutter-pi/pkg/utils"
	"plant-shutter-pi/pkg/video"
)

var (
	logger *zap.SugaredLogger
)

func init() {
	logger = utils.GetLogger()
}

type Project struct {
	Name string `json:"name"`
	Info string `json:"info"`
	// ms
	Interval int                  `json:"interval"`
	Camera   types.CameraSettings `json:"camera"`
	Video    types.VideoSetting   `json:"video"`

	CreatedAt time.Time `json:"createdAt"`

	video   *video.Builder
	rootDir string
}

type ImagesInfo struct {
	MaxNumber   int    `json:"maxNumber"`
	LatestImage string `json:"latestImage"`

	StartedAt *time.Time `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`

	UpdateAt *time.Time `json:"updateAt"`
}

type VideoInfo struct {
	MaxNumber int `json:"maxNumber"`

	UpdateAt *time.Time `json:"updateAt"`
}

func (p *Project) SetRootDir(dir string) {
	p.rootDir = path.Join(dir, p.Name)
}

func New(name, info string, interval int, rootDir string, camera types.CameraSettings, video types.VideoSetting) (*Project, error) {
	p := &Project{
		Name:      name,
		Info:      info,
		Interval:  interval,
		Camera:    camera,
		Video:     video,
		CreatedAt: time.Now(),
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
		p.getVideoDirPath(),
	)
	if err != nil {
		return err
	}

	if _, err = p.LoadImageInfo(); err != nil {
		if err = p.dumpImageInfo(&ImagesInfo{}, false); err != nil {
			return err
		}
	}
	if _, err = p.loadVideoInfo(); err != nil {
		if err = p.dumpVideoInfo(&VideoInfo{}); err != nil {
			return err
		}
	}

	return nil
}

func (p *Project) SaveImage(image []byte) error {
	info, err := p.LoadImageInfo()
	if err != nil {
		return err
	}
	name := p.generateImageName(image, info.MaxNumber)
	if err = os.WriteFile(p.GetImagePath(name), image, consts.DefaultFilePerm); err != nil {
		return err
	}

	info.MaxNumber++
	info.LatestImage = name
	if err = p.dumpImageInfo(info, true); err != nil {
		return err
	}
	if p.Video.Enable {
		if p.video == nil {
			logger.Info("create video")
			if err = p.NewVideoBuilder(); err != nil {
				return err
			}
		} else if p.video.GetCnt() >= p.Video.MaxImage {
			logger.Info("save video")
			_ = p.video.Close()
			if err = p.NewVideoBuilder(); err != nil {
				return err
			}
		}

		logger.Info("add image to video")
		if err = p.video.Add(image); err != nil {
			return err
		}
	}

	return nil
}

func (p *Project) NewVideoBuilder() error {
	info, err := p.loadVideoInfo()
	if err != nil {
		return err
	}

	name := p.generateVideoName(info.MaxNumber)
	logger.Infof("new video builder %s", name)
	p.video, err = video.NewBuilder(path.Join(p.getVideoDirPath(), name), consts.Width, consts.Height, p.Video.FPS)
	if err != nil {
		return err
	}
	info.MaxNumber++
	if err = p.dumpVideoInfo(info); err != nil {
		return err
	}

	return nil
}

func (p *Project) LatestImageName() (string, error) {
	info, err := p.LoadImageInfo()
	if err != nil {
		return "", err
	}

	return info.LatestImage, nil
}

func (p *Project) LatestImage() ([]byte, error) {
	info, err := p.LoadImageInfo()
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

func (p *Project) ListImages(fun func(info fs.FileInfo) error) error {
	return listFiles(p.getImageDirPath(), consts.DefaultImageExt, fun)
}

func (p *Project) GetVideoPath(name string) string {
	return path.Join(p.getVideoDirPath(), name)
}

func (p *Project) ListVideos(fun func(info fs.FileInfo) error) error {
	return listFiles(p.getVideoDirPath(), consts.DefaultVideoExt, fun)
}

func (p *Project) Clear() error {
	_ = p.Close()
	return os.RemoveAll(p.rootDir)
}

func (p *Project) ClearImages() error {
	err := os.RemoveAll(p.getImageDirPath())
	if err != nil {
		return err
	}

	return p.initStorage()
}

func (p *Project) ClearVideos() error {
	err := os.RemoveAll(p.getVideoDirPath())
	if err != nil {
		return err
	}

	return p.initStorage()
}

func (p *Project) Close() error {
	if p.video != nil {
		return p.video.Close()
	}

	return nil
}

func (p *Project) Cleaned() (bool, error) {
	name, err := p.LatestImageName()
	if err != nil {
		return false, err
	}

	return name == "", nil
}

func (p *Project) generateImageName(image []byte, number int) string {
	// generate filenames using md5?
	// fmt.Sprintf("%x", md5.Sum(data))
	return fmt.Sprintf("%s-%07d%s", p.Name, number, consts.DefaultImageExt)
}

func (p *Project) generateVideoName(number int) string {
	return fmt.Sprintf("%s-%06d%s", p.Name, number, consts.DefaultVideoExt)
}

func (p *Project) LoadImageInfo() (*ImagesInfo, error) {
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

func (p *Project) dumpImageInfo(info *ImagesInfo, newImage bool) error {
	t := time.Now()
	info.UpdateAt = &t
	if newImage {
		if info.StartedAt == nil {
			info.StartedAt = &t
		}
		info.EndedAt = &t
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return os.WriteFile(p.getImageInfoPath(), data, consts.DefaultFilePerm)
}

func (p *Project) loadVideoInfo() (*VideoInfo, error) {
	data, err := os.ReadFile(p.getVideoInfoPath())
	if err != nil {
		return nil, fmt.Errorf("read video info err: %w", err)
	}
	info := &VideoInfo{}
	if err = json.Unmarshal(data, info); err != nil {
		return nil, fmt.Errorf("unmarshal video info err: %w", err)
	}

	return info, nil
}

func (p *Project) dumpVideoInfo(info *VideoInfo) error {
	t := time.Now()
	info.UpdateAt = &t
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return os.WriteFile(p.getVideoInfoPath(), data, consts.DefaultFilePerm)
}

func (p *Project) GetRootPath() string {
	return p.rootDir
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

func (p *Project) getVideoInfoPath() string {
	return path.Join(p.rootDir, consts.DefaultVideosDir, consts.DefaultInfoFile)
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
