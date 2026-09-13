package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	"plant-shutter-pi/pkg/cameramode"
	"plant-shutter-pi/pkg/ov"
	"plant-shutter-pi/pkg/preview"
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
	webdavPort = flag.Int("webdav-port", 8080, "webdav port")
	port       = flag.Int("port", 80, "ui port")
	storageDir = flag.String("dir", "./plant-project", "")
	staticsDir = flag.String("statics", "./statics", "")
	devName    = flag.String("dev", "/dev/video0", "")
	// A zero capture dimension means "use the largest JPEG size reported by
	// the camera". This keeps the default still image at the sensor's maximum
	// resolution while still allowing an explicit -width/-height override.
	width         = flag.Int("width", 0, "JPEG capture width (0 uses camera maximum)")
	height        = flag.Int("height", 0, "JPEG capture height (0 uses camera maximum)")
	previewWidth  = flag.Int("preview-width", 1280, "H.264 preview width")
	previewHeight = flag.Int("preview-height", 720, "H.264 preview height")

	flashPin           = flag.String("flash-pin", "", "// \"11\": gpio number\n// \"GPIO11\": gpio name as defined per the bcm238x CPU driver\n// \"P1_23\": board header P1 position 23 name as defined by the rpi board driver")
	flashTriggerOnHigh = flag.Bool("flash-trigger-on-high", true, "")

	logger       *zap.SugaredLogger
	webdavServer *webdav.Webdav

	stg               *storage.Storage
	dev               *camera.Camera
	sch               *schedule.Scheduler
	frames            <-chan []byte
	modeManager       *cameramode.Manager
	previewController *preview.Controller
	previewHandler    *preview.Handler
	modeCoordinator   *camera.ModeCoordinator
	trialShotMu       sync.Mutex
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
	var err error

	webdavServer = webdav.New(ctx, *webdavPort, *storageDir)

	// init storage
	stg, err = storage.New(*storageDir)
	if err != nil {
		logger.Fatal(err)
	}

	// init gin
	r := gin.New()
	//gin.SetMode(gin.ReleaseMode)
	r.Use(requestLogger())
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
	deviceRouter.PUT("/mode", func(c *gin.Context) { previewController.ServeHTTP(c.Writer, c.Request) })
	deviceRouter.GET("/preview", func(c *gin.Context) {
		if previewHandler == nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		previewHandler.ServeHTTP(c.Writer, c.Request)
	})
	deviceRouter.PUT("/webdav", ctlWebdav)
	deviceRouter.GET("/config", listConfig)
	deviceRouter.PUT("/config", updateConfig)
	deviceRouter.PUT("/config/reset", resetConfig)
	deviceRouter.PUT("/date", updateDate)
	deviceRouter.GET("/disk", getDiskUsage)
	deviceRouter.GET("/memory", getMemUsage)
	deviceRouter.GET("/camera", getCameraStatus)
	deviceRouter.GET("/resolution", getResolution)
	deviceRouter.POST("/trial-shot", trialShot)

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

	// init camera
	if err = initDevice(ctx, *devName, *width, *height, *flashPin, *flashTriggerOnHigh); err != nil {
		logger.Error(fmt.Sprintf("camera %s is not ready, related functions will not be available, err: %s", *devName, err))
	}
	// Keep the H.264 preview resolution separate from the still-capture
	// resolution configured by -width/-height.
	modeManager = cameramode.NewManager(ctx, *devName, *devName, *previewWidth, *previewHeight, 0)
	modeCoordinator = camera.NewModeCoordinator(ctx, dev, modeManager, consts.Width, consts.Height)
	modeCoordinator.SetCaptureFrames(frames)
	modeCoordinator.OnCapture = func(input <-chan []byte) {
		frames = input
		if sch != nil {
			sch.SetInput(input)
		}
	}
	previewController = &preview.Controller{Manager: modeCoordinator, Hub: preview.New(), IdleTimeout: 10 * time.Minute, ProjectRunning: func() bool { return sch != nil && sch.GetProject() != nil }}
	previewHandler = &preview.Handler{Manager: modeCoordinator, Hub: previewController.Hub, Width: *previewWidth, Height: *previewHeight, ProjectRunning: previewController.ProjectRunning, Upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	go previewController.Monitor(ctx.Done())
	if sch != nil {
		if last, resumeErr := stg.GetLastRunningProject(); resumeErr != nil {
			logger.Warnw("could not inspect last running project", "error", resumeErr)
		} else if last != nil && last.EndedAt.IsZero() {
			logger.Infow("resuming shooting project after restart", "project", last.Name)
			if dev != nil {
				dev.UpdateSettings(last.CameraSettings)
			}
			sch.Begin(last)
		}
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
		logger.Infof("capture resolution not specified; using camera maximum %dx%d", w, h)
	}
	consts.Width = w
	consts.Height = h

	frames, err = dev.Start(consts.Width, consts.Height)
	if err != nil {
		// Some bcm2835-v4l2 firmware builds reject 1920x1080 JPEG while
		// accepting the camera's stable 640x480 mode. Retry after Start has
		// cleaned up the failed device handle so preview mode can still work.
		if consts.Width != 640 || consts.Height != 480 {
			logger.Warn("camera resolution unavailable, retrying at 640x480")
			consts.Width, consts.Height = 640, 480
			frames, err = dev.Start(consts.Width, consts.Height)
		}
		if err != nil {
			return err
		}
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
	configs, err := modeCoordinator.GetKnownCtrlConfigs()
	if err != nil {
		internalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, jsend.Success(configs))
}

func updateConfig(c *gin.Context) {
	if p := sch.GetProject(); p != nil {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s is running", p.Name)))
		return
	}
	configs := make([]ov.UpdateConfig, 0)
	err := c.Bind(&configs)
	if err != nil {
		return
	}
	for _, cfg := range configs {
		if err = modeCoordinator.SetControlValue(cfg.ID, cfg.Value); err != nil {
			internalErr(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, jsend.Success("set ctrls config"))
}

func resetConfig(c *gin.Context) {
	configs, err := modeCoordinator.GetKnownCtrlConfigs()
	if err != nil {
		internalErr(c, err)
		return
	}
	for _, cfg := range configs {
		if err = modeCoordinator.SetControlValue(cfg.ID, cfg.Default); err != nil {
			internalErr(c, err)
			return
		}
	}
	configs, err = modeCoordinator.GetKnownCtrlConfigs()
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

func getResolution(c *gin.Context) {
	c.JSON(http.StatusOK, jsend.Success(map[string]any{
		"capture": map[string]int{"width": consts.Width, "height": consts.Height},
		"preview": map[string]int{"width": *previewWidth, "height": *previewHeight},
	}))
}

func trialShot(c *gin.Context) {
	// Serialize this request with project transitions as well as other trial
	// shots. The active-project check must cover the complete camera switch.
	trialShotMu.Lock()
	defer trialShotMu.Unlock()
	if sch != nil && sch.GetProject() != nil {
		c.JSON(http.StatusConflict, jsend.SimpleErr("pause the shooting project before taking a trial shot"))
		return
	}
	if modeCoordinator == nil {
		c.JSON(http.StatusServiceUnavailable, jsend.SimpleErr("camera is unavailable"))
		return
	}
	// A trial shot switches the shared V4L2 device away from the live H.264
	// stream. The lock above serializes the complete switch/capture/restore
	// lifecycle so two browser requests cannot consume each other's JPEG frame.
	if previewController != nil && previewController.Hub != nil {
		// Release every H.264 websocket before reopening the V4L2 node for the
		// full-resolution JPEG trial shot.
		previewController.Hub.CloseAll()
		time.Sleep(300 * time.Millisecond)
	}
	frame, err := modeCoordinator.CaptureOnce(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, jsend.SimpleErr(err.Error()))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "image/jpeg")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(frame)
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
	o.DiskUsage = humanize.Bytes(uint64(usage))
	if runningP != nil && runningP.Name == p.Name {
		o.Running = true
	}
	switch {
	case !p.EndedAt.IsZero():
		o.State = "completed"
	case o.Running:
		o.State = "shooting"
	case !p.StartedAt.IsZero():
		o.State = "paused"
	default:
		o.State = "not_started"
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

// startProject switches the camera to capture mode and starts the scheduler.
// Project creation uses the same path as the explicit "continue shooting"
// action so a newly created project is immediately active.
func startProject(pj *model.Project) error {
	if previewController != nil {
		if err := previewController.SetMode(cameramode.ModeCapture); err != nil {
			return err
		}
	}
	logger.Info("restore camera settings")
	dev.UpdateSettings(pj.CameraSettings)
	if pj.StartedAt.IsZero() {
		pj.StartedAt = time.Now()
	}
	pj.EndedAt = time.Time{}
	sch.Begin(pj)
	if err := stg.SetLastRunningProject(pj.Name); err != nil {
		sch.Stop()
		return err
	}
	return nil
}

func createProject(c *gin.Context) {
	var p ov.NewProject
	err := c.Bind(&p)
	if err != nil {
		return
	}
	trialShotMu.Lock()
	defer trialShotMu.Unlock()
	if running := sch.GetProject(); running != nil {
		c.JSON(http.StatusConflict, jsend.SimpleErr(fmt.Sprintf("project %s is running, please pause or end it first", running.Name)))
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

	settings := p.Camera
	if settings == nil {
		settings = make(model.CameraSettings)
	}
	pj, err = stg.NewProject(p.Name, p.Info, *p.Interval, settings)
	if err != nil {
		internalErr(c, err)
		return
	}
	if err = startProject(pj); err != nil {
		_ = stg.DeleteProject(pj.Name)
		_ = stg.ClearLastRunningProject()
		internalErr(c, err)
		return
	}
	if err = stg.UpdateProject(pj); err != nil {
		sch.Stop()
		_ = stg.ClearLastRunningProject()
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
	trialShotMu.Lock()
	defer trialShotMu.Unlock()
	logger.Info(p)

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

	if p.Camera != nil {
		runningP := sch.GetProject()
		if (runningP != nil && runningP.Name == pj.Name) || !pj.Cleaned() {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s has been run, please reset first", pj.Name)))
			return
		}
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
		if *p.Running && !pj.EndedAt.IsZero() {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s has ended; reset it before restarting", pj.Name)))
			return
		}
		runningP := sch.GetProject()
		if runningP != nil && runningP.Name != p.Name {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s is running, please stop first", runningP.Name)))
			return
		}
		if *p.Running {
			if err = startProject(pj); err != nil {
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
	if p.Completed != nil && *p.Completed {
		runningP := sch.GetProject()
		if runningP != nil && runningP.Name != pj.Name {
			c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("project %s is running, please pause it first", runningP.Name)))
			return
		}
		if runningP != nil {
			sch.Stop()
		}
		if err = stg.ClearLastRunningProject(); err != nil {
			internalErr(c, err)
			return
		}
		pj.EndedAt = time.Now()
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

	p, err = stg.NewProject(p.Name, p.Info, p.Interval, p.CameraSettings)
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
	if p.ImageCount == 0 || p.LatestImageName == "" {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("project has no captured images"))
		return
	}
	image, err := p.GetLatestImage()
	if err != nil {
		internalErr(c, err)
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "no-store")
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
	if err == nil {
		logger.Errorw("request failed with nil error", "method", c.Request.Method, "path", c.Request.URL.Path)
		err = fmt.Errorf("internal server error")
	} else {
		logger.Errorw("request failed", "error", err, "method", c.Request.Method, "path", c.Request.URL.Path, "query", c.Request.URL.RawQuery)
	}
	c.JSON(http.StatusInternalServerError, jsend.SimpleErr(err.Error()))
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		fields := []any{"method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "latency", time.Since(started), "client", c.ClientIP()}
		if c.Request.URL.RawQuery != "" {
			fields = append(fields, "query", c.Request.URL.RawQuery)
		}
		if len(c.Errors) > 0 {
			fields = append(fields, "errors", c.Errors.Errors())
		}
		if c.Writer.Status() >= 500 {
			logger.Errorw("http request completed", fields...)
		} else if c.Writer.Status() >= 400 {
			logger.Warnw("http request completed", fields...)
		} else {
			logger.Infow("http request completed", fields...)
		}
	}
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
