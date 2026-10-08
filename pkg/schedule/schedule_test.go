package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"plant-shutter-pi/pkg/plugin"
	"plant-shutter-pi/pkg/storage/model"
)

func TestCaptureFrameDiscardsCachedAndStartupFrames(t *testing.T) {
	frames := make(chan []byte, 1)
	frames <- []byte("cached dark frame")
	s := &Scheduler{}
	s.SetInput(frames)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		// Wait until capture has drained the cached frame before delivering new frames.
		for len(frames) != 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond):
			}
		}
		for _, frame := range []string{"warming 1", "warming 2", "warming 3", "stable"} {
			select {
			case frames <- []byte(frame):
			case <-ctx.Done():
				return
			}
		}
	}()
	frame, err := s.captureFrame(ctx)
	if err != nil || string(frame) != "stable" {
		t.Fatalf("got %q, %v; want stable frame", frame, err)
	}
	if s.warmupFrames != 0 {
		t.Fatal("warmup was not completed")
	}
}

func TestCaptureFrameWaitsForFreshFrameOnLaterSamples(t *testing.T) {
	frames := make(chan []byte, 1)
	frames <- []byte("previous interval")
	s := &Scheduler{input: frames}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		for len(frames) != 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond):
			}
		}
		select {
		case frames <- nil:
		case <-ctx.Done():
			return
		}
		select {
		case frames <- []byte("current interval"):
		case <-ctx.Done():
		}
	}()
	frame, err := s.captureFrame(ctx)
	if err != nil || string(frame) != "current interval" {
		t.Fatalf("got %q, %v", frame, err)
	}
}

func TestCaptureFrameFailureDoesNotReturnCachedImage(t *testing.T) {
	for _, closed := range []bool{false, true} {
		frames := make(chan []byte, 1)
		frames <- []byte("stale")
		if closed {
			close(frames)
		}
		s := &Scheduler{input: frames, warmupFrames: 3}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		frame, err := s.captureFrame(ctx)
		cancel()
		if err == nil || len(frame) != 0 {
			t.Fatalf("closed=%v: got %q, %v", closed, frame, err)
		}
		if !closed && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected timeout, got %v", err)
		}
		if s.warmupFrames != 3 {
			t.Fatal("failed warmup must be retried")
		}
	}
}

func TestBeginResetsWarmup(t *testing.T) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	s := &Scheduler{t: ticker}
	s.Begin(&model.Project{ProjectEntity: model.ProjectEntity{Interval: 60000}})
	if s.warmupFrames != 3 {
		t.Fatalf("warmup = %d, want 3", s.warmupFrames)
	}
}

type captureHooks struct{ before, after bool }

func (h *captureHooks) BeforeCapture() error { h.before = true; return nil }
func (h *captureHooks) AfterCapture() error  { h.after = true; return nil }

func TestFailedCaptureCleansUpWithoutSaving(t *testing.T) {
	project, err := model.New("capture-error", "", 1000, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	frames := make(chan []byte, 1)
	frames <- []byte("stale")
	close(frames)
	hooks := &captureHooks{}
	s := &Scheduler{p: project, input: frames, plugins: []plugin.Plugin{hooks}, logger: zap.NewNop().Sugar()}
	s.deal(context.Background(), time.Now())
	if !hooks.before || !hooks.after {
		t.Fatal("capture hooks were not paired on failure")
	}
	if project.ImageCount != 0 {
		t.Fatal("failed capture saved an image")
	}
}
