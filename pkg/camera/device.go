package camera

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"plant-shutter-pi/pkg/ov"
	"plant-shutter-pi/pkg/types"
)

var (
	StartedErr        = errors.New("already started")
	PreviewStartedErr = errors.New("preview already started")
)

type Camera struct {
	devName string
	ctx     context.Context

	lock   sync.Mutex
	cancel context.CancelFunc
	camera *device.Device
}

func New(ctx context.Context, devName string) *Camera {
	return &Camera{ctx: ctx, devName: devName}
}

func (c *Camera) open(width, height int) error {
	if c.camera != nil {
		return StartedErr
	}
	camera, err := device.Open(
		c.devName,
		device.WithFPS(30),
		device.WithBufferSize(1),
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: v4l2.PixelFmtJPEG,
			Width:       uint32(width),
			Height:      uint32(height),
		}),
	)
	if err != nil {
		return err
	}
	c.camera = camera

	return nil
}

func (c *Camera) Start(width, height int) (<-chan []byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	logger.Infof("start camera in %d*%d", width, height)
	err := c.open(width, height)
	if err != nil {
		return nil, err
	}

	newCtx, cancel := context.WithCancel(c.ctx)
	c.cancel = cancel
	if err = c.camera.Start(newCtx); err != nil {
		return nil, err
	}

	return c.camera.GetOutput(), nil
}

func (c *Camera) Stop() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.cancel != nil {
		// 先取消上下文，让底层流处理 goroutine 走到 ctx.Done 分支并调用 d.Stop()
		c.cancel()
		// 短暂等待，避免我们随即调用 Close() 时与底层 goroutine 的 Stop() 并发执行
		time.Sleep(100 * time.Millisecond)
		c.cancel = nil
	}
	if c.camera != nil {
		err := c.camera.Close()
		c.camera = nil
		return err
	}

	return nil
}

func (c *Camera) ResetSettings() {
	c.UpdateSettings(initSettings)
}

func (c *Camera) AutoSettings() {
	c.UpdateSettings(autoSettings)
}

func (c *Camera) UpdateSettings(settings types.CameraSettings) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.applySettings(settings)
}

func (c *Camera) applySettings(settings types.CameraSettings) {
	if c.camera == nil {
		return
	}

	for k, v := range settings {
		if err := c.camera.SetControlValue(k, v); err != nil {
			logger.Warnf("set ctrl(%d) to %d, err: %s", k, v, err)
		}
	}
	configs, err := c.getKnownCtrlConfigs()
	if err != nil {
		logger.Warnf("failed to get known ctrl configs: %v", err)
	}
	var configStrings []string
	for _, cf := range configs {
		configStrings = append(configStrings, cf.String())
	}
	logger.Debugf("capture config:\n%s", strings.Join(configStrings, "\n"))
}

func (c *Camera) SetControlValue(key v4l2.CtrlID, value v4l2.CtrlValue) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.applySetting(key, value)
}

func (c *Camera) applySetting(k v4l2.CtrlID, v v4l2.CtrlValue) error {
	if c.camera == nil {
		return nil
	}

	return c.camera.SetControlValue(k, v)
}

func (c *Camera) GetKnownCtrlConfigs() ([]ov.Config, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.camera == nil {
		return nil, errors.New("camera not started")
	}

	var res []ov.Config
	for _, id := range knownCtrlID {
		ctrl, err := v4l2.GetControl(c.camera.Fd(), id)
		if err != nil {
			logger.Warnf("The device does not support control(%d)", id)
			continue
		}
		cfg, err := ctrlToConfig(ctrl)
		if err != nil {
			return nil, err
		}
		res = append(res, cfg)
	}

	return res, nil
}

func (c *Camera) getKnownCtrlConfigs() ([]ov.Config, error) {
	if c.camera == nil {
		return nil, errors.New("camera not started")
	}

	var res []ov.Config
	for _, id := range knownCtrlID {
		ctrl, err := v4l2.GetControl(c.camera.Fd(), id)
		if err != nil {
			logger.Warnf("The device does not support control(%d)", id)
			continue
		}
		cfg, err := ctrlToConfig(ctrl)
		if err != nil {
			return nil, err
		}
		res = append(res, cfg)
	}

	return res, nil
}

func (c *Camera) GetKnownCtrlSettings() (types.CameraSettings, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.camera == nil {
		return nil, errors.New("camera not started")
	}

	res := make(types.CameraSettings)
	for _, id := range knownCtrlID {
		ctrl, err := v4l2.GetControl(c.camera.Fd(), id)
		if err != nil {
			continue
		}
		res[ctrl.ID] = ctrl.Value
	}

	return res, nil
}

func (c *Camera) GetMaxSize() (width, height int, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.camera == nil {
		return 0, 0, errors.New("camera not started")
	}

	sizes, err := v4l2.GetAllFormatFrameSizes(c.camera.Fd())
	if err != nil {
		return
	}
	for _, size := range sizes {
		if size.PixelFormat == v4l2.PixelFmtJPEG {
			width = int(size.Size.MaxWidth)
			height = int(size.Size.MaxHeight)

			return
		}
	}
	err = fmt.Errorf("unable to determine the maximum pixels of the camera")

	return
}

func DrainLatest(c context.Context, first []byte, frames <-chan []byte) ([]byte, bool) {
	latest := first
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				return latest, ok
			}
			logger.Debug("drop old frame")
			latest = f // 覆盖为最新
			continue
		case <-c.Done():
			return latest, true
		default:
			// 没有更多积压帧了
			return latest, true
		}
	}
}
