package webdav

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/webdav"

	"plant-shutter-pi/pkg/utils"
)

type Webdav struct {
	lock   sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	port   int
	dir    string
}

func New(ctx context.Context, port int, dir string) *Webdav {
	return &Webdav{
		ctx:  ctx,
		port: port,
		dir:  dir,
	}
}

func (w *Webdav) Start() {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.cancel != nil {
		return
	}
	newCtx, cancel := context.WithCancel(w.ctx)
	w.cancel = cancel
	Serve(newCtx, w.port, w.dir)
}

func (w *Webdav) Stop() {
	w.lock.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	w.lock.Unlock()
}

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
