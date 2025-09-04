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

func captureOnceFull(dev string, width, height uint32, outfile string) error {
	const (
		bufSize     = 4                // 环形缓冲区大小
		warmupDrop  = 2                // 预热：丢弃前几帧
		waitTimeout = 20 * time.Second // 等待帧的超时
	)

	// 先确认设备支持 JPEG
	// 以及该分辨率是否在 JPEG 格式下受支持，避免因为不支持而一直等不到帧
	// 打开设备做能力查询
	camProbe, err := device.Open(dev)
	if err != nil {
		return fmt.Errorf("open for probe: %w", err)
	}
	defer camProbe.Close()

	descs, err := camProbe.GetFormatDescriptions()
	if err != nil {
		return fmt.Errorf("get format desc: %w", err)
	}

	var jpegSupported bool
	for _, d := range descs {
		if d.PixelFormat == v4l2.PixelFmtJPEG {
			jpegSupported = true
			break
		}
	}
	if !jpegSupported {
		return fmt.Errorf("device does not support JPEG format")
	}

	// 重新以目标参数打开（可直接复用上面的 camProbe 也行，这里更直观）
	cam, err := device.Open(
		dev,
		device.WithBufferSize(bufSize),
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: v4l2.PixelFmtJPEG,
			Width:       width,
			Height:      height,
			Field:       v4l2.FieldNone,
		}),
	)
	if err != nil {
		return fmt.Errorf("open with JPEG %dx%d: %w", width, height, err)
	}
	defer cam.Close()

	// 启动流
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := cam.Start(ctx); err != nil {
		return fmt.Errorf("start stream: %w", err)
	}

	// 预热：丢弃前几帧（有的相机刚开流会给空帧/不稳定帧）
	for i := 0; i < warmupDrop; i++ {
		select {
		case <-cam.GetOutput():
			// drop
		case <-time.After(waitTimeout / 2):
			return fmt.Errorf("warmup timeout: no frames under JPEG %dx%d", width, height)
		}
	}

	// 抓一帧并保存（JPEG 已经是压缩后的字节流，直接写文件即可）
	select {
	case frame, ok := <-cam.GetOutput():
		if !ok || len(frame) == 0 {
			return fmt.Errorf("empty or closed frame channel")
		}
		f, err := os.Create(outfile)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		if _, err := f.Write(frame); err != nil {
			f.Close()
			return fmt.Errorf("write file: %w", err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close file: %w", err)
		}
		log.Printf("Saved JPEG: %s", outfile)
	case <-time.After(waitTimeout):
		return fmt.Errorf("timeout waiting for JPEG frame at %dx%d", width, height)
	}

	return nil
}
func captureOnce(dev string, width, height uint32, outfile string) error {
	// 打开相机
	cam, err := device.Open(
		dev,
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: v4l2.PixelFmtMJPEG, // 可以改成 v4l2.PixelFmtJPEG
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
	//if err := captureOnce(dev, 1920, 1080, "photo_1080.jpg"); err != nil {
	//	log.Fatalf("capture 1080p failed: %v", err)
	//}
	//
	//time.Sleep(3 * time.Second)

	// 切换到高分辨率：3280x2464
	if err := captureOnceFull(dev, 3280, 2464, "photo_full.jpg"); err != nil {
		log.Fatalf("capture full-res failed: %v", err)
	}
}
