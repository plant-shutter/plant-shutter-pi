package camera

import (
	"context"
	"errors"
	"github.com/vladimirvivien/go4vl/v4l2"
	"testing"
	"time"
)

type fakeDevice struct {
	frames  chan []byte
	started bool
	closed  bool
}

func (d *fakeDevice) Start(context.Context) error {
	d.started = true
	d.frames <- []byte{1}
	return nil
}
func (d *fakeDevice) Stop() error           { return nil }
func (d *fakeDevice) Close() error          { d.closed = true; return nil }
func (d *fakeDevice) Output() <-chan []byte { return d.frames }

func TestManagerSwitchesModesAndWaitsForFirstFrame(t *testing.T) {
	var paths []string
	f := func(path string, format v4l2.FourCCType, _, _, _ int) (Device, error) {
		name := "JPEG"
		if format == v4l2.PixelFmtH264 {
			name = "H264"
		}
		paths = append(paths, path+":"+name)
		return &fakeDevice{frames: make(chan []byte, 2)}, nil
	}
	m := NewManagerWithFactory(context.Background(), "h264", "jpeg", 10, 10, 5, f)
	if err := m.Switch(ModePreview); err != nil {
		t.Fatal(err)
	}
	if m.Mode() != ModePreview {
		t.Fatalf("mode=%s", m.Mode())
	}
	if err := m.Switch(ModeCapture); err != nil {
		t.Fatal(err)
	}
	if m.Mode() != ModeCapture {
		t.Fatalf("mode=%s", m.Mode())
	}
	if len(paths) != 2 || paths[0] != "h264:H264" || paths[1] != "jpeg:JPEG" {
		t.Fatalf("paths=%v", paths)
	}
	select {
	case <-m.Frames():
	case <-time.After(time.Second):
		t.Fatal("no frame")
	}
}

func TestManagerPropagatesStopFailure(t *testing.T) {
	stopErr := errors.New("stream off failed")
	opens := 0
	m := NewManagerWithFactory(context.Background(), "camera", "camera", 10, 10, 5,
		func(string, uint32, int, int, int) (Device, error) {
			opens++
			return &stopFailDevice{fakeDevice: fakeDevice{frames: make(chan []byte, 2)}, err: stopErr}, nil
		})
	if err := m.Switch(ModePreview); err != nil {
		t.Fatal(err)
	}
	if err := m.Switch(ModeCapture); !errors.Is(err, stopErr) {
		t.Fatalf("want stop failure, got %v", err)
	}
	if m.Mode() != ModeError || opens != 1 {
		t.Fatalf("mode=%s opens=%d", m.Mode(), opens)
	}
}

type stopFailDevice struct {
	fakeDevice
	err error
}

func (d *stopFailDevice) Stop() error { return d.err }
