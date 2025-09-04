package utils

import (
	"context"
	"fmt"
	"log"
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
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := srv.Shutdown(ctx)
	if err != nil {
		log.Println(err)
	}
	log.Println("server shutdown")
	cancel()
}
