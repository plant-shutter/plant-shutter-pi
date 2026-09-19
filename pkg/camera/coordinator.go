package camera

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/go4vl/v4l2"
	"plant-shutter-pi/pkg/cameramode"
	"plant-shutter-pi/pkg/ov"
	"plant-shutter-pi/pkg/storage/model"
)

// ModeCoordinator serializes the legacy control/capture camera and the native
// H.264/JPEG mode manager so the V4L2 node is never opened twice.
type ModeCoordinator struct {
	legacy            *Camera
	manager           *cameramode.Manager
	ctx               context.Context
	width, height     int
	trialWarmupFrames int
	OnCapture         func(<-chan []byte)
	mu                sync.Mutex
	mode              cameramode.Mode
	captureFrames     <-chan []byte
}

func (c *ModeCoordinator) GetKnownCtrlConfigs() ([]ov.Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]ov.Config, 0)
	for _, id := range KnownControlIDs() {
		var ctrl v4l2.Control
		var err error
		if c.mode == cameramode.ModePreview {
			ctrl, err = c.manager.GetControl(id)
		} else {
			ctrl, err = c.legacy.getControl(id)
		}
		if err != nil {
			logger.Warnw("camera control unavailable", "id", id, "error", err)
			continue
		}
		cfg, err := ctrlToConfig(ctrl)
		if err != nil {
			return nil, err
		}
		result = append(result, cfg)
	}
	return result, nil
}
func (c *ModeCoordinator) GetKnownCtrlSettings() (model.CameraSettings, error) {
	configs, err := c.GetKnownCtrlConfigs()
	if err != nil {
		return nil, err
	}
	out := make(model.CameraSettings, len(configs))
	for _, cfg := range configs {
		out[cfg.ID] = cfg.Value
	}
	return out, nil
}

func NewModeCoordinator(ctx context.Context, legacy *Camera, manager *cameramode.Manager, width, height int) *ModeCoordinator {
	return &ModeCoordinator{ctx: ctx, legacy: legacy, manager: manager, width: width, height: height, trialWarmupFrames: 2, mode: cameramode.ModeCapture}
}

// SetTrialWarmupFrames controls how many valid JPEG frames are discarded after
// switching from H.264 before a trial shot is returned. A small warmup lets
// auto-exposure and white-balance settle after the camera is reopened.
func (c *ModeCoordinator) SetTrialWarmupFrames(frames int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if frames < 0 {
		frames = 0
	}
	c.trialWarmupFrames = frames
}

// TrialWarmupFrames returns the number of JPEG frames discarded after a mode
// transition before exposing trial images.
func (c *ModeCoordinator) TrialWarmupFrames() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.trialWarmupFrames
}
func (c *ModeCoordinator) Switch(mode cameramode.Mode) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == mode {
		return nil
	}
	if mode == cameramode.ModePreview {
		if err := c.legacy.Stop(); err != nil {
			return err
		}
		if err := c.manager.Switch(mode); err != nil {
			return err
		}
		c.mode = mode
		return nil
	}
	if err := c.manager.Stop(); err != nil {
		return err
	}
	// The H.264 V4L2 stream releases the device asynchronously on some
	// Raspberry Pi drivers. Give the kernel a moment before reopening the same
	// node through the JPEG path for a trial shot or scheduled capture.
	time.Sleep(750 * time.Millisecond)
	frames, err := c.legacy.Start(c.width, c.height)
	if errors.Is(err, StartedErr) {
		// A preview client can disconnect while the legacy camera is still
		// finishing its asynchronous shutdown. Complete that shutdown and retry
		// once before returning a mode-switch failure to the UI.
		_ = c.legacy.Stop()
		frames, err = c.legacy.Start(c.width, c.height)
	}
	if err == nil && c.OnCapture != nil {
		c.OnCapture(frames)
	}
	if err == nil {
		c.captureFrames = frames
		c.mode = mode
	}
	return err
}
func (c *ModeCoordinator) Frames() <-chan []byte { return c.manager.Frames() }

// SetCaptureFrames records the already-running JPEG stream used when the
// coordinator starts in capture mode during process initialization.
func (c *ModeCoordinator) SetCaptureFrames(frames <-chan []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.captureFrames = frames
}

// CaptureFrames returns the currently active JPEG frame stream. The caller
// must already have switched the coordinator to capture mode and is expected
// to stop consuming the stream when its request is canceled.
func (c *ModeCoordinator) CaptureFrames() <-chan []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.captureFrames
}

// CaptureOnce switches to the JPEG source, waits for one complete image, and
// restores the H.264 source before returning. It is used for a tuning trial
// shot and deliberately does not persist the image in a shooting project.
func (c *ModeCoordinator) CaptureOnce(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.Switch(cameramode.ModeCapture); err != nil {
		return nil, err
	}
	defer func() {
		if err := c.Switch(cameramode.ModePreview); err != nil {
			logger.Warnw("restore preview after trial shot failed", "error", err)
		}
	}()
	c.mu.Lock()
	frames := c.captureFrames
	warmupFrames := c.trialWarmupFrames
	c.mu.Unlock()
	if frames == nil {
		return nil, errors.New("jpeg capture stream is unavailable")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	started := time.Now()
	discarded := 0
	for discarded < warmupFrames {
		select {
		case frame, ok := <-frames:
			if !ok {
				return nil, errors.New("camera closed during trial shot warmup")
			}
			if len(frame) == 0 {
				continue
			}
			discarded++
		case <-waitCtx.Done():
			return nil, fmt.Errorf("trial shot warmup timeout after discarding %d/%d frames: %w", discarded, warmupFrames, waitCtx.Err())
		}
	}
	if warmupFrames > 0 {
		logger.Infow("trial shot warmup frames discarded", "requested", warmupFrames, "discarded", discarded, "elapsed", time.Since(started).String())
	}
	var frame []byte
	select {
	case frame = <-frames:
		if len(frame) == 0 {
			return nil, errors.New("camera returned an empty trial shot")
		}
	case <-waitCtx.Done():
		return nil, fmt.Errorf("trial shot timeout: %w", waitCtx.Err())
	}
	return frame, nil
}

// SetControlValue applies a camera control to whichever camera mode is
// currently active. The preview page opens an H.264 stream while the tuning
// controls are visible, so writing only to the legacy JPEG camera would be a
// silent no-op after that mode switch.
func (c *ModeCoordinator) SetControlValue(id v4l2.CtrlID, value v4l2.CtrlValue) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == cameramode.ModePreview {
		return c.manager.SetControlValue(id, value)
	}
	return c.legacy.SetControlValue(id, value)
}
