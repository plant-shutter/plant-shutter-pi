package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

var (
	frames <-chan []byte
)

func imageServ(w http.ResponseWriter, req *http.Request) {
	mimeWriter := multipart.NewWriter(w)
	w.Header().Set("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", mimeWriter.Boundary()))
	partHeader := make(textproto.MIMEHeader)
	partHeader.Add("Content-Type", "image/jpeg")

	var frame []byte
	start := time.Now()
	for frame = range frames {
		for {
			select {
			case f := <-frames:
				frame = f // 用最新的覆盖
				continue  // 继续尝试再吃一帧
			default:
				// 没有更多积压帧了，退出丢弃环节
			}
			break
		}

		end := time.Now()
		log.Println(end.Sub(start))
		start = end
		partWriter, err := mimeWriter.CreatePart(partHeader)
		if err != nil {
			log.Printf("failed to create multi-part writer: %s", err)
			return
		}

		if _, err := partWriter.Write(frame); err != nil {
			log.Printf("failed to write image: %s", err)
			return
		}
		err = http.NewResponseController(w).Flush()
		if err != nil {
			log.Printf("failed to Flush image: %s", err)
		}
	}
}

func main() {
	port := ":80"
	devName := "/dev/video0"
	flag.StringVar(&devName, "d", devName, "device name (path)")
	flag.StringVar(&port, "p", port, "webcam service port")

	camera, err := device.Open(
		devName,
		device.WithBufferSize(2),
		//device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: 1280, Height: 720}),
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: 1920, Height: 1080}),
	)
	// device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: 1920, Height: 1080}),
	// 延迟2秒
	// device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: 3280, Height: 2464}),
	// 延迟8秒

	if err != nil {
		log.Fatalf("failed to open device: %s", err)
	}
	defer camera.Close()

	if err := camera.Start(context.TODO()); err != nil {
		log.Fatalf("camera start: %s", err)
	}

	frames = camera.GetOutput()

	log.Printf("Serving images: [%s/stream]", port)
	http.HandleFunc("/stream", imageServ)
	log.Fatal(http.ListenAndServe(port, nil))
}
