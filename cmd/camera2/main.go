package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

func captureOnce(dev string, width, height uint32, outfile string) error {
	// 打开相机
	cam, err := device.Open(
		dev,
		device.WithBufferSize(1),
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: v4l2.PixelFmtJPEG, // 可以改成 v4l2.PixelFmtJPEG
			Width:       width,
			Height:      height,
		}),
	)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer cam.Close()

	// 启动采集
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cam.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// 取一帧保存
	select {
	case frame, ok := <-cam.GetOutput():
		if !ok {
			return fmt.Errorf("channel closed")
		}
		f, err := os.Create(outfile)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		defer f.Close()

		if _, err := f.Write(frame); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		log.Printf("Saved file: %s", outfile)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for frame")
	}

	return nil
}

func main() {
	dev := "/dev/video0"

	// 第一次：1920x1080
	if err := captureOnce(dev, 1920, 1080, "photo_1080.jpg"); err != nil {
		log.Fatalf("capture 1080p failed: %v", err)
	}

	// 切换到高分辨率：3280x2464
	if err := captureOnce(dev, 3280, 2464, "photo_full.jpg"); err != nil {
		log.Fatalf("capture full-res failed: %v", err)
	}
}
