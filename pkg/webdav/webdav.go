package webdav

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/webdav"

	"plant-shutter-pi/pkg/utils"
)

func Serve(ctx context.Context, port int, dir string) {
	logger := utils.GetLogger()

	h := &webdav.Handler{
		FileSystem: webdav.Dir(dir),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				logger.Errorf("WEBDAV [%s]: %s, err: %s\n", r.Method, r.URL, err)
			}
		},
	}
	svr := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h,
	}

	go func() {
		if err := svr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("webdav server err: %s", err)
		}
	}()
	go func() {
		<-ctx.Done()
		srcCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := svr.Shutdown(srcCtx); err != nil {
			logger.Errorf("shutdown webdav server err: %s", err)
		}
	}()
}
