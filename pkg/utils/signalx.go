package utils

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func ListenAndServe(ctx context.Context, h http.Handler, port int) {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			GetLogger().Errorw("http server listen failed", "address", srv.Addr, "error", err)
		}
	}()

	<-ctx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := srv.Shutdown(ctx)
	if err != nil {
		GetLogger().Errorw("http server shutdown failed", "error", err)
	}
	GetLogger().Info("server shutdown")
	cancel()
}
