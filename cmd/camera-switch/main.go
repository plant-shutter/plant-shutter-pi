// camera-switch validates the production camera.Manager on a Raspberry Pi.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"plant-shutter-pi/pkg/cameramode"
)

var (
	devPath  = flag.String("device", "/dev/video0", "V4L2 camera device")
	h264Path = flag.String("h264-device", "", "H.264 device; defaults to -device")
	jpegPath = flag.String("jpeg-device", "", "JPEG device; defaults to -device")
	width    = flag.Int("width", 1920, "frame width")
	height   = flag.Int("height", 1080, "frame height")
	fps      = flag.Int("fps", 0, "requested FPS; zero leaves the driver default")
	cycles   = flag.Int("cycles", 3, "switch cycles")
	interval = flag.Duration("interval", 0, "delay between cycles")
)

type resources struct{ rssKB, fds, goroutines int }

func snapshot() resources {
	r := resources{goroutines: runtime.NumGoroutine()}
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		r.fds = len(entries)
	}
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					r.rssKB, _ = strconv.Atoi(fields[1])
				}
			}
		}
	}
	return r
}

func main() {
	flag.Parse()
	h264, jpeg := *h264Path, *jpegPath
	if h264 == "" {
		h264 = *devPath
	}
	if jpeg == "" {
		jpeg = *devPath
	}
	m := cameramode.NewManager(context.Background(), h264, jpeg, *width, *height, *fps)
	start := snapshot()
	var max resources
	for i := 0; i < *cycles; i++ {
		cycleStart := time.Now()
		log.Printf("cycle %d/%d: H264 -> JPEG", i+1, *cycles)
		if err := m.Switch(cameramode.ModePreview); err != nil {
			log.Fatal(err)
		}
		if frame, ok := <-m.Frames(); !ok || len(frame) == 0 {
			log.Fatal("empty H264 frame")
		} else {
			log.Printf("h264 frame: %d bytes", len(frame))
		}
		if err := m.Switch(cameramode.ModeCapture); err != nil {
			log.Fatal(err)
		}
		if frame, ok := <-m.Frames(); !ok || len(frame) == 0 {
			log.Fatal("empty JPEG frame")
		} else {
			log.Printf("jpeg frame: %d bytes", len(frame))
		}
		r := snapshot()
		if r.rssKB > max.rssKB {
			max.rssKB = r.rssKB
		}
		if r.fds > max.fds {
			max.fds = r.fds
		}
		if r.goroutines > max.goroutines {
			max.goroutines = r.goroutines
		}
		log.Printf("cycle %d complete: switch=%s rss=%dKB fds=%d goroutines=%d", i+1, time.Since(cycleStart).Round(time.Millisecond), r.rssKB, r.fds, r.goroutines)
		if *interval > 0 {
			time.Sleep(*interval)
		}
	}
	if err := m.Stop(); err != nil {
		log.Fatal(fmt.Errorf("stop manager: %w", err))
	}
	end := snapshot()
	log.Printf("camera mode switch validation passed (%d cycles); resources start=%+v max=%+v end=%+v", *cycles, start, max, end)
	if end.fds > start.fds+2 || end.goroutines > start.goroutines+2 || end.rssKB > start.rssKB+8192 {
		log.Fatalf("resource growth detected: start=%+v end=%+v", start, end)
	}
}
