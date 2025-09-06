package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/ffmpeg"
)

func main() {
	// 参数
	dev := flag.String("dev", "/dev/video0", "video device path")
	dir := flag.String("dir", ".", "output directory")
	out := flag.String("out", "output_%03d.mkv", "output mkv filename")
	fps := flag.Int("fps", 30, "output video framerate")
	interval := flag.Duration("interval", 2*time.Second, "capture interval")
	count := flag.Int("count", 0, "number of shots (0=until Ctrl-C)")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *dir, err)
	}

	// 固定分辨率：1640x1232（偶数，适合后续 H.264）
	const w, h = 1640, 1232

	// 打开相机（只一次）
	cam, err := device.Open(
		*dev,
		device.WithBufferSize(3), // 稍微大一点更稳
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: v4l2.PixelFmtJPEG,
			Width:       w,
			Height:      h,
		}),
	)
	if err != nil {
		log.Fatalf("open device: %v", err)
	}
	defer cam.Close()

	// 启动采集
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := cam.Start(ctx); err != nil {
		log.Fatalf("start: %v", err)
	}

	camOut := cam.GetOutput()

	// 捕获最新帧（用锁保护）
	var (
		mu        sync.RWMutex
		lastFrame []byte
	)
	// 背景协程：不断读取 channel，永远保留“最新帧”
	go func() {
		for b := range camOut {
			// 一定要 copy，避免持有 mmap 的底层切片
			cp := make([]byte, len(b))
			copy(cp, b)

			mu.Lock()
			lastFrame = cp
			mu.Unlock()
		}
		// chan 关闭（例如设备停了）就退出协程
	}()

	// 创建 ffmpeg 编码器
	outPath := filepath.Join(*dir, *out)
	enc, err := ffmpeg.NewEncoder(ctx, ffmpeg.Options{
		Output:    outPath,
		Framerate: *fps,
		Bitrate:   "12M",
	})
	if err != nil {
		log.Fatalf("start ffmpeg: %v", err)
	}
	defer func() {
		if err := enc.Close(); err != nil {
			log.Printf("close ffmpeg error: %v (stderr: %s)", err, enc.Stderr())
		} else {
			log.Printf("video written to %s", outPath)
		}
	}()

	// 计时与退出
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	shots := 0
	for {
		select {
		case <-sigCh:
			log.Println("received signal, exiting…")
			return

		case <-ticker.C:
			// 取出最新帧并写入视频
			mu.RLock()
			frame := lastFrame
			mu.RUnlock()

			if len(frame) == 0 {
				log.Println("no frame yet, skip this tick")
				continue
			}

			if err := enc.WriteImage(frame); err != nil {
				log.Printf("ffmpeg write frame failed: %v", err)
				continue
			}
			log.Printf("wrote frame #%d (%d bytes)", shots+1, len(frame))
			shots++

			if *count > 0 && shots >= *count {
				log.Printf("done. total frames: %d", shots)
				return
			}
		}
	}
}
