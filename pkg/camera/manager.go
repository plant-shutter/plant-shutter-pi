package camera

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
}

func NewManager(ctx context.Context, h264Path, jpegPath string, width, height, fps int) *Manager {
	return NewManagerWithFactory(ctx, h264Path, jpegPath, width, height, fps, defaultFactory)
}

func NewManagerWithFactory(ctx context.Context, h264Path, jpegPath string, width, height, fps int, factory Factory) *Manager {
	return &Manager{ctx: ctx, factory: factory, h264Path: h264Path, jpegPath: jpegPath, width: width, height: height, fps: fps, mode: ModeError}
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
	return &deviceAdapter{d: d}, nil
}

type deviceAdapter struct{ d *device.Device }

func (d *deviceAdapter) Start(ctx context.Context) error { return d.d.Start(ctx) }
func (d *deviceAdapter) Stop() error                     { return d.d.Stop() }
func (d *deviceAdapter) Close() error                    { return d.d.Close() }
func (d *deviceAdapter) Output() <-chan []byte           { return d.d.GetOutput() }

func (m *Manager) Mode() Mode            { m.mu.Lock(); defer m.mu.Unlock(); return m.mode }
func (m *Manager) Frames() <-chan []byte { m.mu.Lock(); defer m.mu.Unlock(); return m.frames }

func (m *Manager) Switch(target Mode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if target != ModePreview && target != ModeCapture {
		return fmt.Errorf("unsupported camera mode %q", target)
	}
	if m.mode == target && m.dev != nil {
		return nil
	}
	if m.dev != nil {
		stopErr := m.dev.Stop()
		if err := errors.Join(stopErr, m.dev.Close()); err != nil {
			return m.fail(err)
		}
		m.dev, m.frames = nil, nil
	}
	path, format := m.jpegPath, v4l2.PixelFmtJPEG
	if target == ModePreview {
		path, format = m.h264Path, v4l2.PixelFmtH264
	}
	d, err := m.factory(path, format, m.width, m.height, m.fps)
	if err != nil {
		return m.fail(err)
	}
	if err = d.Start(m.ctx); err != nil {
		_ = d.Close()
		return m.fail(err)
	}
	frames := d.Output()
	select {
	case first, ok := <-frames:
		if !ok || len(first) == 0 {
			_ = d.Stop()
			_ = d.Close()
			return m.fail(errors.New("camera produced no frame"))
		}
		m.dev, m.frames, m.mode = d, prepend(first, frames), target
		return nil
	case <-time.After(5 * time.Second):
		_ = d.Stop()
		_ = d.Close()
		return m.fail(errors.New("timeout waiting for camera frame"))
	case <-m.ctx.Done():
		_ = d.Stop()
		_ = d.Close()
		return m.fail(m.ctx.Err())
	}
}

func (m *Manager) fail(err error) error { m.mode = ModeError; m.dev, m.frames = nil, nil; return err }

func prepend(first []byte, rest <-chan []byte) <-chan []byte {
	out := make(chan []byte, 1)
	out <- first
	go func() {
		defer close(out)
		for frame := range rest {
			out <- frame
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
	cerr := m.dev.Close()
	m.dev, m.frames = nil, nil
	m.mode = ModeError
	if err != nil {
		return err
	}
	return cerr
}
