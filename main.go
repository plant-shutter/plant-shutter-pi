package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/beevik/ntp"
	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/vincent-vinf/go-jsend"
	"go.uber.org/zap"
	"plant-shutter-pi/pkg/plugin"
	"plant-shutter-pi/pkg/plugin/gpio"

	"plant-shutter-pi/pkg/storage/model"
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
	webdavPort = flag.Int("webdav-port", 8080, "webdav port")
	port       = flag.Int("port", 80, "ui port")
	storageDir = flag.String("dir", "./plant-project", "")
	staticsDir = flag.String("statics", "./statics", "")
	devName    = flag.String("dev", "/dev/video0", "")
	width      = flag.Int("width", 1920, "")
	height     = flag.Int("height", 1080, "")

	flashPin           = flag.String("flash-pin", "", "// \"11\": gpio number\n// \"GPIO11\": gpio name as defined per the bcm238x CPU driver\n// \"P1_23\": board header P1 position 23 name as defined by the rpi board driver")
	flashTriggerOnHigh = flag.Bool("flash-trigger-on-high", true, "")

	logger       *zap.SugaredLogger
	webdavServer *webdav.Webdav

	stg    *storage.Storage
	dev    *camera.Camera
	sch    *schedule.Scheduler
	frames <-chan []byte
)

func init() {
	logger = utils.GetLogger()
	flag.Parse()
}

func main() {
	fmt.Println("┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓")
	fmt.Println("┃              🌿  plant-shutter v0.1.0                        ┃")
	fmt.Println("┃            Raspberry Pi Camera Automation                    ┃")
	fmt.Println("┃  repo: https://github.com/plant-shutter/plant-shutter-pi     ┃")
	fmt.Println("┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛")

	defer logger.Sync()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	err := unzipStatics()
	if err != nil {
		logger.Fatal(err)
	}

	webdavServer = webdav.New(ctx, *webdavPort, *storageDir)

	// init storage
	stg, err = storage.New(*storageDir)
	if err != nil {
		logger.Fatal(err)
	}

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

	//projectRouter.GET("/:name/video", listProjectVideos)
	//projectRouter.GET("/:name/video/:video", getProjectVideo)
	//projectRouter.DELETE("/:name/video/:video", deleteProjectVideo)
	//projectRouter.DELETE("/:name/video", deleteProjectVideos)

	// init camera
	if err = initDevice(ctx, *devName, *width, *height, *flashPin, *flashTriggerOnHigh); err != nil {
		logger.Error(fmt.Sprintf("camera %s is not ready, related functions will not be available, err: %s", *devName, err))
	}
	defer dev.Stop()

	ips, err := getLocalIPsWithPort(*port)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Info("listen ", ips)

	utils.ListenAndServe(ctx, r, *port)
}

func initDevice(ctx context.Context, devName string, w, h int, flashPin string, flashLevel bool) error {
	dev = camera.New(ctx, devName)
	var err error
	if w <= 0 || h <= 0 {
		w, h, err = dev.GetMaxSize()
		if err != nil {
			return err
		}
	}
	consts.Width = w
	consts.Height = h

	frames, err = dev.Start(consts.Width, consts.Height)
	if err != nil {
		return err
	}
	logger.Info("start device ", devName)
	dev.ResetSettings()

	var plugins []plugin.Plugin
	if flashPin != "" {
		flash, err := gpio.NewFlashPin(flashPin, flashLevel)
		if err != nil {
			return fmt.Errorf("failed to init flash pin %s, err: %s", flashPin, err)
		}
		plugins = append(plugins, flash)
	}
	// init schedule
	// todo plugin
	logger.Info("start schedule")
	sch = schedule.New(ctx, stg, frames, plugins)

	return nil
}

func listConfig(c *gin.Context) {
	configs, err := dev.GetKnownCtrlConfigs()
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
	configs, err := dev.GetKnownCtrlConfigs()
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
	configs, err = dev.GetKnownCtrlConfigs()
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

func fillOvProject(p, runningP *model.Project) (*ov.Project, error) {
	usage, err := ps.DirDiskUsage(p.GetRootPath())
	if err != nil {
		return nil, err
	}
	var o ov.Project
	o.Project = p
	o.Video = ov.GetVideoSettingFromProject(p)
	o.DiskUsage = humanize.Bytes(uint64(usage))
	if runningP != nil && runningP.Name == p.Name {
		o.Running = true
	}
	if !p.StartedAt.IsZero() {
		o.StartedAt = &p.StartedAt
	}
	if !p.EndedAt.IsZero() {
		o.EndedAt = &p.EndedAt
	}
	o.ImageTotal = p.ImageCount
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
	var p ov.NewProject
	err := c.Bind(&p)
	if err != nil {
		return
	}
	if p.Interval == nil {
		var i int32 = 124800
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

	if p.Video == nil {
		p.Video = &model.VideoSetting{
			Enable:             true,
			FPS:                30,
			MaxImage:           450,
			ShootingDays:       6.5,
			TotalVideoLength:   2.5,
			PreviewVideoLength: 15,
		}
	}
	pj, err = stg.NewProject(p.Name, p.Info, *p.Interval, make(model.CameraSettings), *p.Video)
	if err != nil {
		internalErr(c, err)
		return
	}
	dev.UpdateSettings(pj.CameraSettings)

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
		pj.Interval = int32(*p.Interval)
	}
	if p.Info != nil {
		pj.Info = *p.Info
	}

	if p.Camera != nil || p.Video != nil {
		runningP := sch.GetProject()
		if (runningP != nil && runningP.Name == pj.Name) || !pj.Cleaned() {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s has been run, please reset first", pj.Name)))
			return
		}
	}
	if p.Video != nil {
		pj.VideoFPS = p.Video.FPS
		pj.VideoMaxImage = p.Video.MaxImage
		pj.ShootingDays = p.Video.ShootingDays
		pj.TotalVideoLength = p.Video.TotalVideoLength
		pj.PreviewVideoLength = p.Video.PreviewVideoLength
	}
	if p.Camera != nil && *p.Camera {
		setting, err := dev.GetKnownCtrlSettings()
		if err != nil {
			internalErr(c, err)
			return
		}
		pj.CameraSettings = setting
	}

	if p.Running != nil {
		runningP := sch.GetProject()
		if runningP != nil && runningP.Name != p.Name {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s is running, please stop first", runningP.Name)))
			return
		}
		if *p.Running {
			logger.Info("restore camera settings")
			dev.UpdateSettings(pj.CameraSettings)
			sch.Begin(pj)
			err = stg.SetLastRunningProject(pj.Name)
			if err != nil {
				internalErr(c, err)
				return
			}
		} else {
			sch.Stop()
			err = stg.ClearLastRunningProject()
			if err != nil {
				internalErr(c, err)
				return
			}
		}
	}

	err = stg.UpdateProject(pj)
	if err != nil {
		internalErr(c, err)
		return
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

	p, err = stg.NewProject(p.Name, p.Info, p.Interval, p.CameraSettings, model.VideoSetting{
		Enable:             p.Enable,
		FPS:                p.VideoFPS,
		MaxImage:           p.VideoMaxImage,
		ShootingDays:       p.ShootingDays,
		TotalVideoLength:   p.TotalVideoLength,
		PreviewVideoLength: p.PreviewVideoLength,
	})
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
	image, err := p.GetLatestImage()
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

//
//func getProjectVideo(c *gin.Context) {
//	p, err := stg.GetProject(c.Param("name"))
//	if err != nil {
//		internalErr(c, err)
//		return
//	}
//	if p == nil {
//		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
//		return
//	}
//	videoName := c.Param("video")
//	videoPath := p.GetVideoPath(videoName)
//	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", videoName))
//	c.Writer.Header().Set("Content-Type", "application/octet-stream")
//	c.File(videoPath)
//}
//
//func deleteProjectVideo(c *gin.Context) {
//	p, err := stg.GetProject(c.Param("name"))
//	if err != nil {
//		internalErr(c, err)
//		return
//	}
//	if p == nil {
//		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
//		return
//	}
//	videoName := c.Param("video")
//	videoPath := p.GetVideoPath(videoName)
//	if err = os.Remove(videoPath); err != nil {
//		internalErr(c, err)
//		return
//	}
//
//	c.JSON(http.StatusOK, jsend.Success(fmt.Sprintf("remove video %s success", videoName)))
//}
//
//func deleteProjectVideos(c *gin.Context) {
//	p, err := stg.GetProject(c.Param("name"))
//	if err != nil {
//		internalErr(c, err)
//		return
//	}
//	if p == nil {
//		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
//		return
//	}
//
//	if err = p.ClearVideos(); err != nil {
//		internalErr(c, err)
//		return
//	}
//
//	c.JSON(http.StatusOK, jsend.Success("remove videos success"))
//}
//
//func listProjectVideos(c *gin.Context) {
//	p, err := stg.GetProject(c.Param("name"))
//	if err != nil {
//		internalErr(c, err)
//		return
//	}
//	if p == nil {
//		c.JSON(http.StatusNotFound, jsend.SimpleErr("project not found"))
//		return
//	}
//	list := make([]types.File, 0)
//	var totalSize int64
//	err = p.ListVideos(func(info fs.FileInfo) error {
//		list = append(list, infoToFile(info))
//		totalSize += info.Size()
//
//		return nil
//	})
//	if err != nil {
//		internalErr(c, err)
//		return
//	}
//	page, _ := strconv.Atoi(c.Query("page"))
//	pageSize, _ := strconv.Atoi(c.Query("page_size"))
//	subVideos, prev, next := getPage(list, page, pageSize)
//	c.JSON(http.StatusOK, jsend.Success(map[string]any{
//		"page":      page,
//		"pageSize":  pageSize,
//		"prevPage":  prev,
//		"nextPage":  next,
//		"total":     len(list),
//		"video":     subVideos,
//		"totalSize": humanize.Bytes(uint64(totalSize)),
//	}))
//}

func realtimeVideo(c *gin.Context) {
	mimeWriter := multipart.NewWriter(c.Writer)
	c.Header("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", mimeWriter.Boundary()))
	partHeader := make(textproto.MIMEHeader)
	partHeader.Add("Content-Type", "image/jpeg")

	for {
		select {
		case frame := <-frames:
			frame, ok := camera.DrainLatest(c, frame, frames)
			if !ok {
				logger.Warn("realtime video frames close")
				return
			}
			if len(frame) == 0 {
				logger.Error("empty frame received")
				continue
			}
			err := writeMimePart(c, mimeWriter, partHeader, frame)
			if err != nil {
				logger.Warnf("failed to write image: %s", err)
				return
			}
		case <-time.After(5 * time.Second):
			logger.Errorf("timeout reading frame")
			data, err := os.ReadFile("camera-disconnect.png")
			if err != nil {
				logger.Warnf("failed to read camera disconnect.png: %s", err)
				continue
			}
			err = writeMimePart(c, mimeWriter, partHeader, data)
			if err != nil {
				logger.Warnf("failed to write image: %s", err)
				return
			}
		case <-c.Done():
			logger.Warn("realtime video context done in for")
		}
	}
}

func writeMimePart(c *gin.Context, mimeWriter *multipart.Writer, partHeader textproto.MIMEHeader, frame []byte) error {
	partWriter, err := mimeWriter.CreatePart(partHeader)
	if err != nil {
		return fmt.Errorf("create part failed: %s", err)
	}

	if _, err = partWriter.Write(frame); err != nil {
		return fmt.Errorf("write part failed: %s", err)
	}
	return http.NewResponseController(c.Writer).Flush()
}

func ctlWebdav(c *gin.Context) {
	op := c.Query("op")
	switch op {
	case webDavStart:
		webdavServer.Start()
		ips, err := getLocalIPsWithPort(*webdavPort)
		if err != nil {
			internalErr(c, err)
			return
		}
		c.JSON(http.StatusOK, jsend.Success(ips))
	case webDavShutdown:
		webdavServer.Stop()
		c.JSON(http.StatusOK, jsend.Success(nil))
	default:
		c.JSON(http.StatusBadRequest, jsend.SimpleErr("unknown operation"))
	}
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
	logger.Debug(err)
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
