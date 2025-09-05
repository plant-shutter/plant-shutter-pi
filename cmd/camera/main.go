package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

func captureOnce(camera *Camera, width, height int, outfile string) error {
	// 打开相机
	frames, err := camera.Start(context.Background(), PixelFmtJPEG, width, height)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer func(camera *Camera) {
		err := camera.Stop()
		if err != nil {
			fmt.Printf("close device: %v\n", err)
		}
	}(camera)

	select {
	case frame, ok := <-frames:
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
	camera := New(dev)

	// 第一次：1920x1080
	if err := captureOnce(camera, 1920, 1080, "photo_1080.jpg"); err != nil {
		log.Fatalf("capture 1080p failed: %v", err)
	}

	time.Sleep(3 * time.Second)

	// 切换到高分辨率：3280x2464
	if err := captureOnce(camera, 3280, 2464, "photo_full.jpg"); err != nil {
		log.Fatalf("capture full-res failed: %v", err)
	}
}

type Camera struct {
	devName string
	camera  *device.Device
	lock    sync.Mutex
}

func New(devName string) *Camera {
	return &Camera{devName: devName}
}

type PixelFormat v4l2.FourCCType

var (
	PixelFmtJPEG  PixelFormat = PixelFormat(v4l2.PixelFmtJPEG)
	PixelFmtMJPEG PixelFormat = PixelFormat(v4l2.PixelFmtMJPEG)
)

func (c *Camera) Start(ctx context.Context, format PixelFormat, width, height int) (<-chan []byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.camera != nil {
		return nil, errors.New("already started")
	}
	camera, err := device.Open(
		c.devName,
		device.WithBufferSize(1),
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: uint32(width), Height: uint32(height)}),
	)
	if err != nil {
		return nil, err
	}
	if err = camera.Start(ctx); err != nil {
		return nil, err
	}
	c.camera = camera
	return camera.GetOutput(), nil
}

func (c *Camera) Stop() error {
	if c.camera != nil {
		return c.camera.Close()
	}
	return nil
}
