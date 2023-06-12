package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/vincent-vinf/go-jsend"
	"go.uber.org/zap"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"plant-shutter-pi/pkg/types"

	"plant-shutter-pi/pkg/camera"
	"plant-shutter-pi/pkg/ov"
	"plant-shutter-pi/pkg/schedule"
	"plant-shutter-pi/pkg/storage"
	"plant-shutter-pi/pkg/storage/consts"
	"plant-shutter-pi/pkg/utils"
	"plant-shutter-pi/pkg/utils/ps"
	"plant-shutter-pi/pkg/webdav"
)

const (
	webDavStart    = "start"
	webDavShutdown = "shutdown"

	runningProjectRouterKey = "running"
)

var (
	webdavPort = flag.Int("webdav-port", 9998, "webdav port")
	port       = flag.Int("port", 9999, "ui port")
	storageDir = flag.String("dir", "./plant-project", "")
	staticsDir = flag.String("statics", "./statics", "")
	devName    = flag.String("dev", "/dev/video0", "")
	width      = flag.Int("width", 3280, "")
	height     = flag.Int("height", 2464, "")

	cancelWebdav context.CancelFunc
	cancelLock   sync.Mutex

	logger *zap.SugaredLogger

	stg    *storage.Storage
	frames <-chan []byte
	dev    *device.Device
	sch    *schedule.Scheduler
)

func init() {
	logger = utils.GetLogger()
	flag.Parse()
	consts.Width = *width
	consts.Height = *height
}

func main() {
	defer logger.Sync()
	defer func() {
		if cancelWebdav != nil {
			cancelWebdav()
		}
	}()
	var err error

	// init storage
	stg, err = storage.New(*storageDir)
	if err != nil {
		logger.Fatal(err)
	}
	defer stg.Close()

	// init gin
	r := gin.New()
	//gin.SetMode(gin.ReleaseMode)
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(utils.Cors())
	if err := registerStaticsDir(r, *staticsDir, "/"); err != nil {
		logger.Fatal(err)
	}
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("page not found"))
	})

	apiRouter := r.Group("/api")

	deviceRouter := apiRouter.Group("/device")
	deviceRouter.GET("/realtime/video", realtimeVideo)
	deviceRouter.PUT("/webdav", ctlWebdav)
	deviceRouter.GET("/config", listConfig)
	deviceRouter.PUT("/config", updateConfig)
	deviceRouter.PUT("/config/reset", resetConfig)
	deviceRouter.GET("/disk", getDiskUsage)

	projectRouter := apiRouter.Group("/project")
	projectRouter.GET("/:name", getProject)
	projectRouter.GET(fmt.Sprintf("/%s", runningProjectRouterKey), getRunningProject)
	projectRouter.GET("", listProject)
	projectRouter.POST("", createProject)
	projectRouter.PUT("", updateProject)
	projectRouter.PUT("/:name/reset", resetProject)
	projectRouter.DELETE("/:name", deleteProject)

	projectRouter.GET("/:name/image", listProjectImages)
	projectRouter.GET("/:name/image/latest", projectLatestImage)
	projectRouter.GET("/:name/image/:image", getProjectImage)

	projectRouter.GET("/:name/video", listProjectVideos)
	projectRouter.GET("/:name/video/:video", getProjectVideo)

	// init camera
	if err = startDevice(*width, *height); err != nil {
		logger.Error(err)
		return
	}
	defer dev.Close()

	// init schedule
	sch = schedule.New(frames)
	defer sch.Clear()

	utils.ListenAndServe(r, *port)
}

func startDevice(w, h int) error {
	var err error
	dev, err = device.Open(
		*devName,
		device.WithBufferSize(0),
	)
	camera.InitControls(dev)
	// todo: get max pixel size
	//if w <= 0 || h <= 0 {
	//	info, err := v4l2.GetAllFormatFrameSizes(dev.Fd())
	//	if err != nil {
	//		return err
	//	}
	//
	//	logger.Info(info)
	//}
	err = dev.SetPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: uint32(w), Height: uint32(h)})
	if err != nil {
		return err
	}
	logger.Infof("set pix format to %d*%d", w, h)
	if err = dev.Start(context.Background()); err != nil {
		return err
	}

	frames = dev.GetOutput()

	return nil
}

func listConfig(c *gin.Context) {
	configs, err := camera.GetKnownCtrlConfigs(dev)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, jsend.Success(configs))
}

func updateConfig(c *gin.Context) {
	if p := sch.GetProject(); p != nil {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s is running", p.Name)))
	}
	configs := make([]ov.UpdateConfig, 0)
	err := c.Bind(&configs)
	if err != nil {
		return
	}
	for _, cfg := range configs {
		if err = dev.SetControlValue(cfg.ID, cfg.Value); err != nil {
			internalErr(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, jsend.Success("set ctrls config"))
}

func resetConfig(c *gin.Context) {
	configs, err := camera.GetKnownCtrlConfigs(dev)
	if err != nil {
		internalErr(c, err)
		return
	}
	for _, cfg := range configs {
		if err = dev.SetControlValue(cfg.ID, cfg.Default); err != nil {
			internalErr(c, err)
			return
		}
	}
	configs, err = camera.GetKnownCtrlConfigs(dev)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(configs))
}

func getDiskUsage(c *gin.Context) {
	used, free, total, usedPercent, err := ps.DiskUsage(*storageDir)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(map[string]any{
		"used":        humanize.Bytes(used),
		"free":        humanize.Bytes(free),
		"total":       humanize.Bytes(total),
		"usedPercent": usedPercent,
	}))
}

func getProject(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}

	c.JSON(http.StatusOK, jsend.Success(p))
	return
}

func getRunningProject(c *gin.Context) {
	p := sch.GetProject()

	c.JSON(http.StatusOK, jsend.Success(p))
	return
}

func listProject(c *gin.Context) {
	projects, err := stg.ListProjects()
	if err != nil {
		internalErr(c, err)
		return
	}
	res := make([]ov.Project, 0)
	runningP := sch.GetProject()
	for _, p := range projects {
		usage, err := ps.DirDiskUsage(p.GetRootPath())
		if err != nil {
			internalErr(c, err)
			return
		}

		var o ov.Project
		o.Project = p
		o.DiskUsage = humanize.Bytes(uint64(usage))
		if runningP != nil && runningP.Name == p.Name {
			o.Running = true
		}
		res = append(res, o)
	}

	c.JSON(http.StatusOK, jsend.Success(res))
	return
}

func createProject(c *gin.Context) {
	var p ov.NewProject
	err := c.Bind(&p)
	if err != nil {
		return
	}

	if p.Interval < consts.MinInterval {
		p.Interval = consts.MinInterval
	}
	if p.Name == runningProjectRouterKey {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project name cannot be %s", p.Name)))
		return
	}

	pj, err := stg.GetProject(p.Name)
	if err != nil {
		internalErr(c, err)
		return
	}
	if pj != nil {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr("project already exists"))
		return
	}

	s, err := camera.GetKnownCtrlSettings(dev)
	if err != nil {
		internalErr(c, err)
		return
	}
	pj, err = stg.NewProject(p.Name, p.Info, p.Interval, s, p.Video)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(pj))
	return
}

func updateProject(c *gin.Context) {
	var p ov.UpdateProject
	err := c.Bind(&p)
	if err != nil {
		return
	}

	pj, err := stg.GetProject(p.Name)
	if err != nil {
		internalErr(c, err)
		return
	}
	if pj == nil {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr("project does not exist"))
		return
	}

	if p.Interval != nil {
		if *p.Interval < consts.MinInterval {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("interval %dms less than %dms", *p.Interval, consts.MinInterval)))
			return
		}
		pj.Interval = *p.Interval
	}
	if p.Info != nil {
		pj.Info = *p.Info
	}

	if p.Camera != nil || p.Video != nil {
		runningP := sch.GetProject()
		cleaned, err := pj.Cleaned()
		if err != nil {
			internalErr(c, err)
			return
		}
		if (runningP != nil && runningP.Name == pj.Name) || !cleaned {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s has been run, please reset first", pj.Name)))
			return
		}
	}
	if p.Video != nil {
		pj.Video = *p.Video
	}
	if p.Camera != nil && *p.Camera {
		setting, err := camera.GetKnownCtrlSettings(dev)
		if err != nil {
			internalErr(c, err)
			return
		}
		pj.Camera = setting
	}

	err = stg.UpdateProject(pj)
	if err != nil {
		internalErr(c, err)
		return
	}
	if p.Running != nil {
		if *p.Running {
			logger.Info("restore camera settings")
			camera.ApplySettings(dev, pj.Camera)
			sch.Begin(pj)
		} else {
			sch.Stop()
		}
	}

	c.JSON(http.StatusOK, jsend.Success(pj))
}

func resetProject(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	if pj := sch.GetProject(); pj != nil && pj.Name == p.Name {
		sch.Stop()
	}

	if err = stg.DeleteProject(p.Name); err != nil {
		internalErr(c, err)
		return
	}

	p, err = stg.NewProject(p.Name, p.Info, p.Interval, p.Camera, p.Video)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(p))
}

func deleteProject(c *gin.Context) {
	name := c.Param("name")

	pj, err := stg.GetProject(name)
	if err != nil {
		internalErr(c, err)
		return
	}
	if pj == nil {
		c.JSON(http.StatusOK, jsend.SimpleErr("project does not exist"))
		return
	}
	if p := sch.GetProject(); p != nil && p.Name == pj.Name {
		sch.Stop()
	}
	if err = stg.DeleteProject(name); err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(fmt.Sprintf("delete project %s success", name)))
	return
}

func projectLatestImage(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	image, err := p.LatestImage()
	if err != nil {
		internalErr(c, err)
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.Writer.Write(image)
}

func getProjectImage(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	image, err := p.GetImage(c.Param("image"))
	if err != nil {
		internalErr(c, err)
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.Writer.Write(image)
}

func listProjectImages(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	list := make([]types.File, 0)
	var totalSize int64
	err = p.ListImages(func(info fs.FileInfo) error {
		list = append(list, infoToFile(info))
		totalSize += info.Size()

		return nil
	})
	if err != nil {
		internalErr(c, err)
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	subImages, prev, next := getPage(list, page, pageSize)
	c.JSON(http.StatusOK, jsend.Success(map[string]any{
		"page":      page,
		"pageSize":  pageSize,
		"prevPage":  prev,
		"nextPage":  next,
		"total":     len(list),
		"images":    subImages,
		"totalSize": humanize.Bytes(uint64(totalSize)),
	}))
}

func getProjectVideo(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	videoName := c.Param("video")
	videoPath := p.GetVideoPath(videoName)
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", videoName))
	c.Writer.Header().Set("Content-Type", "application/octet-stream")
	c.File(videoPath)
}

func listProjectVideos(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	list := make([]types.File, 0)
	var totalSize int64
	err = p.ListVideos(func(info fs.FileInfo) error {
		list = append(list, infoToFile(info))
		totalSize += info.Size()

		return nil
	})
	if err != nil {
		internalErr(c, err)
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	subVideos, prev, next := getPage(list, page, pageSize)
	c.JSON(http.StatusOK, jsend.Success(map[string]any{
		"page":      page,
		"pageSize":  pageSize,
		"prevPage":  prev,
		"nextPage":  next,
		"total":     len(list),
		"video":     subVideos,
		"totalSize": humanize.Bytes(uint64(totalSize)),
	}))
}

func realtimeVideo(c *gin.Context) {
	mimeWriter := multipart.NewWriter(c.Writer)
	c.Header("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", mimeWriter.Boundary()))
	partHeader := make(textproto.MIMEHeader)
	partHeader.Add("Content-Type", "image/jpeg")

	for frame := range frames {
		partWriter, err := mimeWriter.CreatePart(partHeader)
		if err != nil {
			logger.Errorf("failed to create multi-part writer: %s", err)
			return
		}

		if _, err := partWriter.Write(frame); err != nil {
			logger.Errorf("failed to write image: %s", err)
		}
	}
}

func ctlWebdav(c *gin.Context) {
	op := c.Query("op")
	switch op {
	case webDavStart:
		startWebdav(c)
	case webDavShutdown:
		shutdownWebdav(c)
	default:
		c.JSON(http.StatusBadRequest, jsend.SimpleErr("unknown operation"))
	}
}

func startWebdav(c *gin.Context) {
	cancelLock.Lock()
	defer cancelLock.Unlock()
	if cancelWebdav != nil {
		c.JSON(http.StatusOK, jsend.Success("the webdav service is already enabled"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	webdav.Serve(ctx, *webdavPort, *storageDir)
	cancelWebdav = cancel
	//url := location.Get(c)
	c.JSON(http.StatusOK, jsend.Success(c.Request.Host))
}

func shutdownWebdav(c *gin.Context) {
	cancelLock.Lock()
	defer cancelLock.Unlock()
	if cancelWebdav == nil {
		c.JSON(http.StatusOK, jsend.SimpleErr("the webdav service has been shut down"))
		return
	}
	cancelWebdav()
	cancelWebdav = nil

	c.JSON(http.StatusOK, jsend.Success(nil))
}

func registerStaticsDir(group gin.IRoutes, dir, relativeGroup string) error {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("the specified directory %s does not exist", dir)
	}
	dir = filepath.ToSlash(filepath.Clean(dir))
	group.StaticFile(relativeGroup, filepath.Join(dir, "index.html"))
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			relativePath := path.Join(relativeGroup, strings.Replace(filepath.ToSlash(p), dir, "", 1))
			group.StaticFile(relativePath, p)
		}
		return nil
	})
}

func internalErr(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, jsend.SimpleErr(err.Error()))
}

func getPage(strs []types.File, page, pageSize int) ([]types.File, int, int) {
	total := len(strs)
	if page < 1 {
		page = 1
	}
	startIndex := (page - 1) * pageSize
	if startIndex >= total {
		return nil, 0, 0
	}
	endIndex := startIndex + pageSize
	if endIndex > total {
		endIndex = total
	}

	prevPage := page - 1

	nextPage := page + 1
	if endIndex == total {
		nextPage = 0
	}

	return strs[startIndex:endIndex], prevPage, nextPage
}

func infoToFile(info fs.FileInfo) types.File {
	return types.File{
		Name:    info.Name(),
		Size:    humanize.Bytes(uint64(info.Size())),
		ModTime: info.ModTime(),
	}
}
