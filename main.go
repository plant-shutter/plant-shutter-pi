package main

import (
	"flag"
	"github.com/gin-gonic/gin"
	"github.com/vincent-vinf/go-jsend"
	"go.uber.org/zap"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"plant-shutter-pi/pkg/utils"
)

var (
	webdavPort = flag.Int("webdav-port", 9998, "webdav port")
	port       = flag.Int("port", 9999, "ui port")
	imageDir   = flag.String("dir", "./plant-shutter", "")

	logger *zap.SugaredLogger
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
	if err := registerStaticsDir(r, "./statics", "/"); err != nil {
		logger.Fatal(err)
	}
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, jsend.SimpleErr("page not found"))
	})

	apiRouter := r.Group("/api")
	apiRouter.GET("/project/:project/image")

	utils.ListenAndServe(r, *port)
	//ctx, cancel := context.WithCancel(context.Background())
	//webdav.Serve(ctx, 8000, "./", logger)
	//time.Sleep(10 * time.Second)
	//cancel()
	//logger.Info("shutdown")
}

func getImage(c *gin.Context) {
	//project := c.Param("project")
	//
	//c.File()
}

func registerStaticsDir(group gin.IRoutes, dir, relativeGroup string) error {
	dir = filepath.ToSlash(filepath.Clean(dir))
	logger.Info(dir)
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			group.StaticFile(path.Join(relativeGroup, strings.TrimLeft(filepath.ToSlash(p), dir)), p)
		}
		return nil
	})
}
