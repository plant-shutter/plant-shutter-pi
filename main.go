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
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/vincent-vinf/go-jsend"
	"go.uber.org/zap"

	"plant-shutter-pi/pkg/ov"
	"plant-shutter-pi/pkg/storage"
	"plant-shutter-pi/pkg/storage/consts"
	"plant-shutter-pi/pkg/utils"
	"plant-shutter-pi/pkg/webdav"
)

const (
	webDavStart    = "start"
	webDavShutdown = "shutdown"
)

var (
	webdavPort = flag.Int("webdav-port", 9998, "webdav port")
	port       = flag.Int("port", 9999, "ui port")
	storageDir = flag.String("dir", "./plant-shutter", "")
	staticsDir = flag.String("statics", "./statics", "")

	logger *zap.SugaredLogger

	cancelWebdav context.CancelFunc
	cancelLock   sync.Mutex

	stg *storage.Storage
)

func init() {
	flag.Parse()
	logger = utils.NewLogger()
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
	deviceRouter.GET("/runtime/video", runtimeVideo)
	deviceRouter.PUT("/webdav", ctlWebdav)

	projectRouter := apiRouter.Group("/project")
	projectRouter.GET("/:name", getProject)
	projectRouter.GET("", listProject)
	projectRouter.POST("", createProject)
	projectRouter.PUT("", updateProject)
	projectRouter.DELETE("/:name", deleteProject)

	projectRouter.GET("/:name/images/latest", projectLatestImage)
	projectRouter.GET("/:name/images/:name")
	projectRouter.GET("/:name/images")

	utils.ListenAndServe(r, *port)
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

func listProject(c *gin.Context) {
	projects, err := stg.ListProjects()
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(projects))
	return
}

func createProject(c *gin.Context) {
	var p ov.Project
	err := c.Bind(&p)
	if err != nil {
		return
	}
	if p.Interval < consts.MinInterval {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("interval %s less than %s", p.Interval, consts.MinInterval)))
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

	pj, err = stg.NewProject(p.Name, p.Info, p.Interval)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(pj))
	return
}

func updateProject(c *gin.Context) {
	var p ov.Project
	err := c.Bind(&p)
	if err != nil {
		return
	}

	if p.Interval < consts.MinInterval {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr(fmt.Sprintf("interval %s less than %s", p.Interval, consts.MinInterval)))
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
	pj.Info = p.Info
	pj.Interval = p.Interval

	err = stg.UpdateProject(pj)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(pj))
	return
}

func deleteProject(c *gin.Context) {
	name := c.Param("name")

	pj, err := stg.GetProject(name)
	if err != nil {
		internalErr(c, err)
		return
	}
	if pj == nil {
		c.JSON(http.StatusBadRequest, jsend.SimpleErr("project does not exist"))
		return
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
	image, err := p.LatestImageName()
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(http.StatusOK, jsend.Success(image))
	return
}

func runtimeVideo(c *gin.Context) {
	mimeWriter := multipart.NewWriter(c.Writer)
	c.Header("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", mimeWriter.Boundary()))
	partHeader := make(textproto.MIMEHeader)
	partHeader.Add("Content-Type", "image/jpeg")

	//for _ = range camera.GetOutput() {
	//partWriter, err := mimeWriter.CreatePart(partHeader)
	//if err != nil {
	//	logger.Warnf("failed to create multi-part writer: %s", err)
	//	return
	//}
	//i := image.DecodeRGB(frame, int(format.BytesPerLine), 640, 480)
	//if err != nil {
	//	log.Println(err)
	//	return
	//}
	//b := bytes.Buffer{}
	//if err := image.EncodeJPEG(i, &b, 95); err != nil {
	//	log.Println(err)
	//	return
	//}
	//if _, err := partWriter.Write(b.Bytes()); err != nil {
	//	logger.Warnf("failed to write image: %s", err)
	//}
	//}
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
	webdav.Serve(ctx, *webdavPort, *storageDir, logger)
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

func getImage(c *gin.Context) {
	//project := c.Param("project")
	//
	//c.File()
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
