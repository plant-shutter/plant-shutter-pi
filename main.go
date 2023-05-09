package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
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

	"plant-shutter-pi/pkg/camera"
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
	imageDir   = flag.String("dir", "./plant-shutter", "")
	staticsDir = flag.String("statics", "./statics", "")

	logger *zap.SugaredLogger

	cancelWebdav context.CancelFunc
	cancelLock   sync.Mutex
)

func init() {
	flag.Parse()
	logger = utils.NewLogger()
}

func main() {
	defer logger.Sync()

	r := gin.New()
	//gin.SetMode(gin.ReleaseMode)
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(utils.Cors())
	// todo fix
	if err := registerStaticsDir(r, *staticsDir, "/"); err != nil {
		logger.Fatal(err)
	}
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("page not found"))
	})

	apiRouter := r.Group("/api")

	apiRouter.GET("/device/runtime/video", runtimeVideo)
	apiRouter.PUT("/device/webdav", ctlWebdav)
	// todo call cancelWebdav()

	if err := camera.Init(camera.DefaultDevice); err != nil {
		logger.Fatal(err)
	}
	defer camera.Close()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	if err := camera.Start(ctx); err != nil {
		log.Fatalln(err)
	}

	utils.ListenAndServe(r, *port)
}

func runtimeVideo(c *gin.Context) {
	mimeWriter := multipart.NewWriter(c.Writer)
	c.Header("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", mimeWriter.Boundary()))
	partHeader := make(textproto.MIMEHeader)
	partHeader.Add("Content-Type", "image/jpeg")

	for _ = range camera.GetOutput() {
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
	webdav.Serve(ctx, *webdavPort, *imageDir, logger)
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
