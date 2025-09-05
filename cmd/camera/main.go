package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"plant-shutter-pi/pkg/camera"
)

func captureOnce(device *camera.Camera, width, height int, outfile string) error {
	// 打开相机
	frames, err := device.Start(context.Background(), width, height)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer device.Stop()

	select {
	case frame, ok := <-frames:
		if !ok {
			return fmt.Errorf("channel closed")
		}
		err = os.WriteFile(outfile, frame, 0644)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}

		log.Printf("Saved file: %s", outfile)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for frame")
	}

	return nil
}

func main() {
	dev := "/dev/video0"
	device := camera.New(dev)

	// 第一次：1920x1080
	if err := captureOnce(device, 1920, 1080, "photo_1080.jpg"); err != nil {
		log.Fatalf("capture 1080p failed: %v", err)
	}

	// 切换到高分辨率：3280x2464
	if err := captureOnce(device, 3280, 2464, "photo_full.jpg"); err != nil {
		log.Fatalf("capture full-res failed: %v", err)
	}

	// 切换到高分辨率：3280x2464
	if err := captureOnce(device, 2560, 1440, "photo_1440.jpg"); err != nil {
		log.Fatalf("capture full-res failed: %v", err)
	}
}
