package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/looplab/fsm"
	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

// ================= 分辨率 & 格式定义 =================

type Resolution struct{ W, H uint32 }

var (
	Res1080p = Resolution{1920, 1080}
	Res720p  = Resolution{1280, 720}
)

type PixelFmt string

const (
	PixMJPEG PixelFmt = "MJPEG"
	PixJPEG  PixelFmt = "JPEG"
)

// ================= 相机封装（go4vl） =================

type Cam struct {
	dev   *device.Device
	stop  context.CancelFunc
	outCh <-chan *device.Frame
}

func openCamera(dev string, res Resolution, pix PixelFmt) (*Cam, error) {
	fmtCode := v4l2.PixelFmtMJPEG
	if pix == PixJPEG {
		fmtCode = v4l2.PixelFmtJPEG
	}
	d, err := device.Open(
		dev,
		device.WithBufferSize(2),
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: fmtCode,
			Width:       res.W,
			Height:      res.H,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return &Cam{dev: d}, nil
}

func (c *Cam) start() error {
	if c.dev == nil {
		return errors.New("nil device")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.stop = cancel
	if err := c.dev.Start(ctx); err != nil {
		cancel()
		return err
	}
	c.outCh = c.dev.GetOutput()
	return nil
}

func (c *Cam) close() {
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	if c.dev != nil {
		_ = c.dev.Close()
		c.dev = nil
	}
}

func (c *Cam) nextFrame(timeout time.Duration) ([]byte, error) {
	if c.outCh == nil {
		return nil, errors.New("stream not started")
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case f, ok := <-c.outCh:
		if !ok {
			return nil, errors.New("stream closed")
		}
		b := f.GetBytes()
		cp := make([]byte, len(b))
		copy(cp, b)
		return cp, nil
	case <-timer.C:
		return nil, errors.New("frame timeout")
	}
}

// ================= 控制器 (FSM) =================

type Controller struct {
	FSM     *fsm.FSM
	devPath string
	pixfmt  PixelFmt

	previewCh chan []byte
	cam       *Cam
}

func NewController(dev string, pf PixelFmt) *Controller {
	c := &Controller{
		devPath:   dev,
		pixfmt:    pf,
		previewCh: nil,
	}

	c.FSM = fsm.NewFSM(
		"idle",
		fsm.Events{
			{Name: "start_preview", Src: []string{"idle"}, Dst: "previewing"},
			{Name: "stop_preview", Src: []string{"previewing"}, Dst: "idle"},
			{Name: "capture", Src: []string{"idle", "previewing"}, Dst: "capturing"},
			{Name: "resume_preview", Src: []string{"capturing"}, Dst: "previewing"},
			{Name: "shutdown", Src: []string{"idle", "previewing", "capturing"}, Dst: "idle"},
		},
		fsm.Callbacks{
			"leave_state": func(e *fsm.Event) {
				if c.cam != nil {
					c.cam.close()
					c.cam = nil
				}
			},

			"enter_previewing": func(e *fsm.Event) {
				c.startPreview()
			},

			"enter_capturing": func(e *fsm.Event) {
				c.doCapture()
			},

			"idle": func(e *fsm.Event) {
				log.Println("[FSM] now idle")
			},
		},
	)

	return c
}

// 对外 API：开启预览，返回 channel
func (c *Controller) StartPreview() (<-chan []byte, error) {
	if c.FSM.Current() != "idle" {
		return nil, errors.New("not idle")
	}
	if err := c.FSM.Event("start_preview"); err != nil {
		return nil, err
	}
	return c.previewCh, nil
}

func (c *Controller) StopPreview() {
	if c.FSM.Current() == "previewing" {
		_ = c.FSM.Event("stop_preview")
	}
}

// 对外 API：拍一张
func (c *Controller) Capture() {
	if err := c.FSM.Event("capture"); err != nil {
		log.Printf("[API] capture failed: %v", err)
	}
}

// ================= 内部动作 =================

func (c *Controller) startPreview() {
	log.Println("[PREVIEW] starting @720p...")
	cam, err := openCamera(c.devPath, Res720p, c.pixfmt)
	if err != nil {
		log.Printf("open err: %v", err)
		_ = c.FSM.Event("stop_preview")
		return
	}
	if err := cam.start(); err != nil {
		log.Printf("start err: %v", err)
		_ = c.FSM.Event("stop_preview")
		return
	}
	c.cam = cam
	c.previewCh = make(chan []byte, 8)

	go func() {
		for {
			if c.FSM.Current() != "previewing" {
				close(c.previewCh)
				return
			}
			frame, err := cam.nextFrame(2 * time.Second)
			if err != nil {
				log.Printf("preview frame err: %v", err)
				return
			}
			select {
			case c.previewCh <- frame:
			default:
				// 丢帧，防止阻塞
			}
		}
	}()
}

func (c *Controller) doCapture() {
	log.Println("[CAPTURE] taking one photo @1080p")
	cam, err := openCamera(c.devPath, Res1080p, c.pixfmt)
	if err != nil {
		log.Printf("capture open err: %v", err)
		return
	}
	defer cam.close()

	if err := cam.start(); err != nil {
		log.Printf("capture start err: %v", err)
		return
	}
	b, err := cam.nextFrame(3 * time.Second)
	if err != nil {
		log.Printf("capture frame err: %v", err)
		return
	}

	_ = os.MkdirAll("photos", 0755)
	path := filepath.Join("photos", time.Now().Format("20060102_150405")+".jpg")
	if err := os.WriteFile(path, b, 0644); err != nil {
		log.Printf("write err: %v", err)
	} else {
		log.Printf("[CAPTURE] saved -> %s", path)
	}

	// 拍完后，如果之前在预览，恢复预览，否则回 idle
	if c.previewCh != nil {
		_ = c.FSM.Event("resume_preview")
	} else {
		_ = c.FSM.Event("shutdown")
	}
}

// ================= DEMO =================

func main() {
	ctrl := NewController("/dev/video0", PixMJPEG)

	// 开启预览，拿到帧 channel
	ch, err := ctrl.StartPreview()
	if err != nil {
		log.Fatalf("preview failed: %v", err)
	}

	// 模拟读 5 帧
	for i := 0; i < 5; i++ {
		f := <-ch
		log.Printf("[MAIN] got preview frame (%d bytes)", len(f))
	}

	// 触发一次拍照
	ctrl.Capture()
	time.Sleep(2 * time.Second)

	// 关闭预览
	ctrl.StopPreview()
}
