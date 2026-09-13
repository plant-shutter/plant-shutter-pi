// Package cameramode contains the hardware-only camera mode state machine.
package cameramode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"plant-shutter-pi/pkg/utils"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

type Mode string

const (
	ModePreview Mode = "preview"
	ModeCapture Mode = "capture"
	ModeError   Mode = "error"
)

var ErrBusy = errors.New("camera mode transition in progress")

type Device interface {
	Start(context.Context) error
	Stop() error
	Close() error
	Output() <-chan []byte
}

type controlDevice interface {
	SetControlValue(v4l2.CtrlID, v4l2.CtrlValue) error
	GetControl(v4l2.CtrlID) (v4l2.Control, error)
}
type Factory func(path string, format v4l2.FourCCType, width, height, fps int) (Device, error)

type Manager struct {
	mu                 sync.Mutex
	ctx                context.Context
	factory            Factory
	h264Path           string
	jpegPath           string
	width, height, fps int
	mode               Mode
	dev                Device
	frames             <-chan []byte
	framesCancel       context.CancelFunc
	deviceCancel       context.CancelFunc
	logger             *zap.SugaredLogger
}

func NewManager(ctx context.Context, h264Path, jpegPath string, width, height, fps int) *Manager {
	return NewManagerWithFactory(ctx, h264Path, jpegPath, width, height, fps, defaultFactory)
}

func NewManagerWithFactory(ctx context.Context, h264Path, jpegPath string, width, height, fps int, factory Factory) *Manager {
	return &Manager{ctx: ctx, factory: factory, h264Path: h264Path, jpegPath: jpegPath, width: width, height: height, fps: fps, mode: ModeError, logger: utils.GetLogger()}
}

func defaultFactory(path string, format v4l2.FourCCType, width, height, fps int) (Device, error) {
	options := []device.Option{device.WithBufferSize(2), device.WithPixFormat(v4l2.PixFormat{PixelFormat: format, Width: uint32(width), Height: uint32(height)})}
	d, err := device.Open(path, append(options, device.WithFPS(uint32(fps)))...)
	if err != nil && fps > 0 {
		// Some V4L2 camera drivers reject VIDIOC_S_PARM even though capture works.
		// Retry with the driver's default frame interval before declaring the mode failed.
		d, err = device.Open(path, options...)
	}
	if err != nil {
		return nil, err
	}
	actual, err := v4l2.GetPixFormat(d.Fd())
	if err != nil || actual.PixelFormat != format {
		_ = d.Close()
		if err == nil {
			err = fmt.Errorf("requested pixel format %#x, got %#x", format, actual.PixelFormat)
		}
		return nil, fmt.Errorf("camera format negotiation: %w", err)
	}
	if format == v4l2.PixelFmtH264 {
		// Broadway, the browser-side decoder, supports H.264 Baseline only.
		// Configure the camera encoder before starting capture; the bitstream is
		// still forwarded raw and is never decoded on the Pi.
		if err := d.SetControlValue(0x00990a6b, 0); err != nil { // h264_profile: Baseline
			_ = d.Close()
			return nil, fmt.Errorf("set H.264 baseline profile: %w", err)
		}
		if err := d.SetControlValue(0x009909e2, 1); err != nil { // repeat_sequence_header
			_ = d.Close()
			return nil, fmt.Errorf("enable repeated H.264 sequence header: %w", err)
		}
	}
	return &deviceAdapter{d: d}, nil
}

type deviceAdapter struct{ d *device.Device }

func (d *deviceAdapter) Start(ctx context.Context) error { return d.d.Start(ctx) }
func (d *deviceAdapter) Stop() error                     { return d.d.Stop() }
func (d *deviceAdapter) Close() error                    { return d.d.Close() }
func (d *deviceAdapter) Output() <-chan []byte           { return d.d.GetOutput() }
func (d *deviceAdapter) SetControlValue(id v4l2.CtrlID, value v4l2.CtrlValue) error {
	return d.d.SetControlValue(id, value)
}
func (d *deviceAdapter) GetControl(id v4l2.CtrlID) (v4l2.Control, error) {
	return v4l2.GetControl(d.d.Fd(), id)
}

func (m *Manager) SetControlValue(id v4l2.CtrlID, value v4l2.CtrlValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		return errors.New("camera is not active")
	}
	d, ok := m.dev.(controlDevice)
	if !ok {
		return errors.New("camera controls are unavailable")
	}
	return d.SetControlValue(id, value)
}

func (m *Manager) GetControl(id v4l2.CtrlID) (v4l2.Control, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		return v4l2.Control{}, errors.New("camera is not active")
	}
	d, ok := m.dev.(controlDevice)
	if !ok {
		return v4l2.Control{}, errors.New("camera controls are unavailable")
	}
	return d.GetControl(id)
}

func (m *Manager) Mode() Mode            { m.mu.Lock(); defer m.mu.Unlock(); return m.mode }
func (m *Manager) Frames() <-chan []byte { m.mu.Lock(); defer m.mu.Unlock(); return m.frames }

func (m *Manager) Switch(target Mode) error {
	started := time.Now()
	if m.logger != nil {
		m.logger.Infow("camera mode switch requested", "target", target, "current", m.Mode())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if target != ModePreview && target != ModeCapture {
		return fmt.Errorf("unsupported camera mode %q", target)
	}
	if m.mode == target && m.dev != nil {
		return nil
	}
	if m.dev != nil {
		if m.framesCancel != nil {
			m.framesCancel()
			m.framesCancel = nil
		}
		if m.deviceCancel != nil {
			m.deviceCancel()
		}
		stopErr := m.dev.Stop()
		m.deviceCancel = nil
		if err := errors.Join(stopErr, m.dev.Close()); err != nil {
			if m.logger != nil {
				m.logger.Errorw("camera mode stop failed", "target", target, "error", err)
			}
			return m.fail(err)
		}
		// Legacy V4L2 drivers can release the device asynchronously after close.
		time.Sleep(100 * time.Millisecond)
		m.dev, m.frames = nil, nil
	}
	path, format := m.jpegPath, v4l2.PixelFmtJPEG
	if target == ModePreview {
		path, format = m.h264Path, v4l2.PixelFmtH264
	}
	d, err := openWithRetry(m.factory, path, format, m.width, m.height, m.fps)
	if err != nil {
		if m.logger != nil {
			m.logger.Errorw("camera mode open failed", "target", target, "path", path, "error", err, "elapsed", time.Since(started))
		}
		return m.fail(err)
	}
	deviceCtx, deviceCancel := context.WithCancel(m.ctx)
	if err = d.Start(deviceCtx); err != nil {
		deviceCancel()
		_ = d.Close()
		return m.fail(err)
	}
	frames := d.Output()
	select {
	case first, ok := <-frames:
		if !ok || len(first) == 0 {
			_ = d.Stop()
			_ = d.Close()
			err := errors.New("camera produced no frame")
			if m.logger != nil {
				m.logger.Errorw("camera produced no frame", "target", target, "error", err)
			}
			return m.fail(err)
		}
		m.dev, m.frames, m.framesCancel, m.deviceCancel, m.mode = d, prepend(deviceCtx, first, frames), deviceCancel, deviceCancel, target
		return nil
	case <-time.After(5 * time.Second):
		_ = d.Stop()
		_ = d.Close()
		err := errors.New("timeout waiting for camera frame")
		if m.logger != nil {
			m.logger.Errorw("camera first frame timeout", "target", target, "error", err)
		}
		return m.fail(err)
	case <-m.ctx.Done():
		_ = d.Stop()
		_ = d.Close()
		if m.logger != nil {
			m.logger.Errorw("camera switch context cancelled", "target", target, "error", m.ctx.Err())
		}
		return m.fail(m.ctx.Err())
	}
}

func openWithRetry(factory Factory, path string, format v4l2.FourCCType, width, height, fps int) (Device, error) {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		var d Device
		d, err = factory(path, format, width, height, fps)
		if err == nil {
			return d, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "resource busy") {
			return nil, err
		}
		if attempt < 5 {
			time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
		}
	}
	return nil, fmt.Errorf("camera open retry exhausted: %w", err)
}

func (m *Manager) fail(err error) error {
	if m.framesCancel != nil {
		m.framesCancel()
		m.framesCancel = nil
	}
	m.mode = ModeError
	m.dev, m.frames = nil, nil
	return err
}

func prepend(ctx context.Context, first []byte, rest <-chan []byte) <-chan []byte {
	out := make(chan []byte, 1)
	out <- first
	go func() {
		defer close(out)
		canceled := false
		for {
			frame, ok := <-rest
			if !ok {
				return
			}
			if !canceled {
				select {
				case <-ctx.Done():
					canceled = true
				default:
				}
			}
			if !canceled {
				select {
				case out <- frame:
				case <-ctx.Done():
					canceled = true
				}
			}
		}
	}()
	return out
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		m.mode = ModeError
		return nil
	}
	err := m.dev.Stop()
	if m.deviceCancel != nil {
		m.deviceCancel()
		m.deviceCancel = nil
	}
	if m.framesCancel != nil {
		m.framesCancel()
		m.framesCancel = nil
	}
	cerr := m.dev.Close()
	m.dev, m.frames = nil, nil
	m.mode = ModeError
	if err != nil {
		return err
	}
	return cerr
}
