package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"github.com/beevik/ntp"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/vincent-vinf/go-jsend"
	"go.uber.org/zap"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"plant-shutter-pi/pkg/storage/project"
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

//go:embed statics.zip
var zipData []byte

var (
	webdavPort = flag.Int("webdav-port", 9998, "webdav port")
	port       = flag.Int("port", 9999, "ui port")
	storageDir = flag.String("dir", "./plant-project", "")
	staticsDir = flag.String("statics", "./statics", "")
	devName    = flag.String("dev", "/dev/video0", "")
	width      = flag.Int("width", 0, "")
	height     = flag.Int("height", 0, "")

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
	err := unzipStatics()
	if err != nil {
		logger.Fatal(err)
	}

	defer logger.Sync()
	defer func() {
		if cancelWebdav != nil {
			cancelWebdav()
		}
	}()

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
	deviceRouter.PUT("/date", updateDate)
	deviceRouter.GET("/disk", getDiskUsage)
	deviceRouter.GET("/memory", getMemUsage)
	deviceRouter.GET("/camera", getCameraStatus)

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
	projectRouter.DELETE("/:name/image/:image", deleteProjectImage)
	projectRouter.DELETE("/:name/image", deleteProjectImages)

	projectRouter.GET("/:name/video", listProjectVideos)
	projectRouter.GET("/:name/video/:video", getProjectVideo)
	projectRouter.DELETE("/:name/video/:video", deleteProjectVideo)
	projectRouter.DELETE("/:name/video", deleteProjectVideos)

	ips, err := getLocalIPsWithPort(*port)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Info("listen ", ips)
	// init camera
	if err = startDevice(*width, *height); err != nil {
		logger.Error(fmt.Sprintf("camera %s is not ready, related functions will not be available, err: %s", *devName, err))
	}
	defer func() {
		if dev != nil {
			dev.Close()
		}
	}()

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
	if err != nil {
		return err
	}
	camera.InitControls(dev)
	if w <= 0 || h <= 0 {
		w, h, err = getMaxSize()
		if err != nil {
			return err
		}
	}
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
	if err := checkDevice(); err != nil {
		internalErr(c, err)
		return
	}
	configs, err := camera.GetKnownCtrlConfigs(dev)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, jsend.Success(configs))
}

func updateConfig(c *gin.Context) {
	if err := checkDevice(); err != nil {
		internalErr(c, err)
		return
	}
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
	if err := checkDevice(); err != nil {
		internalErr(c, err)
		return
	}
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

func updateDate(c *gin.Context) {
	//start := time.Now()
	t := ov.Time{}
	err := c.Bind(&t)
	if err != nil {
		return
	}
	if n := t.NewTime.Sub(time.Now()); n > -time.Second && n < time.Second {
		c.JSON(http.StatusOK, jsend.Success("if the time difference is less than one second, skip the time setting"))
		return
	}
	//newTime, err := getNTPTime()
	//if err != nil {
	//	logger.Warnf("get ntp time failed, err: %s", err)
	//	newTime = t.NewTime.Add(time.Now().Sub(start))
	//}
	err = setSystemTime(t.NewTime)
	if err != nil {
		internalErr(c, err)
		return
	}
	logger.Info("now: ", time.Now())
	c.JSON(http.StatusOK, jsend.Success(fmt.Sprintf("successfully set time to %s", t.NewTime)))
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

func getMemUsage(c *gin.Context) {
	used, free, total, usedPercent, err := ps.MemoryStatus()
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

func getCameraStatus(c *gin.Context) {
	available := false
	if dev != nil {
		available = true
	}
	c.JSON(http.StatusOK, jsend.Success(map[string]any{
		"available": available,
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

	runningP := sch.GetProject()
	ovProject, err := fillOvProject(p, runningP)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(ovProject))
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
		ovProject, err := fillOvProject(p, runningP)
		if err != nil {
			internalErr(c, err)
			return
		}
		res = append(res, *ovProject)
	}

	c.JSON(http.StatusOK, jsend.Success(res))
	return
}

func fillOvProject(p, runningP *project.Project) (*ov.Project, error) {
	usage, err := ps.DirDiskUsage(p.GetRootPath())
	if err != nil {
		return nil, err
	}
	info, err := p.LoadImageInfo()
	if err != nil {
		return nil, err
	}
	var o ov.Project
	o.Project = p
	o.DiskUsage = humanize.Bytes(uint64(usage))
	if runningP != nil && runningP.Name == p.Name {
		o.Running = true
	}
	o.StartedAt = info.StartedAt
	o.EndedAt = info.EndedAt
	o.ImageTotal = info.MaxNumber
	if o.StartedAt != nil && o.EndedAt != nil {
		duration := o.EndedAt.Sub(*o.StartedAt)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) - hours*60
		o.Time = fmt.Sprintf("%02d:%02d", hours, minutes)
	} else {
		o.Time = "00:00"
	}

	return &o, nil
}

func createProject(c *gin.Context) {
	if err := checkDevice(); err != nil {
		internalErr(c, err)
		return
	}
	var p ov.NewProject
	err := c.Bind(&p)
	if err != nil {
		return
	}
	if p.Interval == nil {
		i := 124800
		p.Interval = &i
	}
	if *p.Interval < consts.MinInterval {
		*p.Interval = consts.MinInterval
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
	if p.Video == nil {
		p.Video = &types.VideoSetting{
			Enable:             true,
			FPS:                30,
			MaxImage:           450,
			ShootingDays:       6.5,
			TotalVideoLength:   2.5,
			PreviewVideoLength: 15,
		}
	}
	pj, err = stg.NewProject(p.Name, p.Info, *p.Interval, s, *p.Video)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(pj))
	return
}

func updateProject(c *gin.Context) {
	if err := checkDevice(); err != nil {
		internalErr(c, err)
		return
	}
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
		runningP := sch.GetProject()
		if runningP != nil && runningP.Name != p.Name {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s is running, please stop first", runningP.Name)))
			return
		}
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

func deleteProjectImage(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}
	name := c.Param("image")
	imagePath := p.GetImagePath(name)
	if err = os.Remove(imagePath); err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(fmt.Sprintf("remove image %s success", name)))
}

func deleteProjectImages(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}

	if err = p.ClearImages(); err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success("remove images success"))
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

func deleteProjectVideo(c *gin.Context) {
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
	if err = os.Remove(videoPath); err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(fmt.Sprintf("remove video %s success", videoName)))
}

func deleteProjectVideos(c *gin.Context) {
	p, err := stg.GetProject(c.Param("name"))
	if err != nil {
		internalErr(c, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
		return
	}

	if err = p.ClearVideos(); err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success("remove videos success"))
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
	ips, err := getLocalIPsWithPort(*webdavPort)
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, jsend.Success(ips))
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

func unzipStatics() error {
	_, err := os.Stat("statics")
	if err == nil {
		logger.Info("statics exist")
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	logger.Info("unzip statics file")
	if err = utils.Unzip(zipData, "."); err != nil {
		return err
	}
	zipData = nil
	runtime.GC()

	return nil
}

func getLocalIPsWithPort(port int) ([]string, error) {
	var ips []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ips = append(ips, fmt.Sprintf("%s:%d", ipnet.IP.String(), port))
			}
		}
	}

	return ips, nil
}

func setSystemTime(newTime time.Time) error {
	cmd := exec.Command("date", "-s", newTime.Format("2006-01-02 15:04:05"))
	output, err := cmd.CombinedOutput()
	logger.Infof("set system time: %s", string(output))
	return err
}

func getNTPTime() (time.Time, error) {
	r, err := ntp.QueryWithOptions("pool.ntp.org", ntp.QueryOptions{})
	if err != nil {
		return time.Now(), err
	}

	err = r.Validate()
	if err != nil {
		return time.Now(), err
	}

	// Use the clock offset to calculate the time.
	return time.Now().Add(r.ClockOffset), nil
}

func checkDevice() error {
	if dev == nil {
		return fmt.Errorf("camera %s is not ready, related functions will not be available", *devName)
	}

	return nil
}

func getMaxSize() (width, height int, err error) {
	sizes, err := v4l2.GetAllFormatFrameSizes(dev.Fd())
	if err != nil {
		panic(err)
	}
	for _, size := range sizes {
		if size.PixelFormat == v4l2.PixelFmtJPEG {
			width = int(size.Size.MaxWidth)
			height = int(size.Size.MaxHeight)

			return
		}
	}
	err = fmt.Errorf("unable to determine the maximum pixels of the camera")

	return
}
