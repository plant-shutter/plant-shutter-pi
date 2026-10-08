package schedule

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"plant-shutter-pi/pkg/plugin"
	"plant-shutter-pi/pkg/storage"
	"plant-shutter-pi/pkg/storage/model"

	"plant-shutter-pi/pkg/utils"
)

// Match trial capture: two warmup frames plus one sensor-settle frame.
const startupDiscardFrames = 3

type Scheduler struct {
	warmupFrames int
	t            *time.Ticker
	input        <-chan []byte
	p            *model.Project
	stg          *storage.Storage
	plugins      []plugin.Plugin
	lock         sync.Mutex
	logger       *zap.SugaredLogger
}

func New(ctx context.Context, stg *storage.Storage, input <-chan []byte, plugins []plugin.Plugin) *Scheduler {
	t := time.NewTicker(time.Second)
	t.Stop()

	s := &Scheduler{
		t:            t,
		input:        input,
		warmupFrames: startupDiscardFrames,
		plugins:      plugins,
		stg:          stg,
		logger:       utils.GetLogger(),
	}
	s.startDeal(ctx)

	return s
}

func (s *Scheduler) Begin(p *model.Project) {
	if p == nil {
		s.Stop()
	}
	s.lock.Lock()
	s.p = p
	s.warmupFrames = startupDiscardFrames
	s.lock.Unlock()
	if p != nil {
		s.t.Reset(utils.MsToDuration(p.Interval))
	}
}

func (s *Scheduler) Stop() {
	s.logger.Info("scheduler: stopped")
	s.t.Stop()
	s.lock.Lock()
	s.p = nil
	s.lock.Unlock()
}

func (s *Scheduler) GetProject() *model.Project {
	if s.p == nil {
		return nil
	}

	return s.p
}

// SetInput replaces the capture stream after a camera mode transition.
func (s *Scheduler) SetInput(input <-chan []byte) {
	s.lock.Lock()
	s.input = input
	s.warmupFrames = startupDiscardFrames
	s.lock.Unlock()
}

func (s *Scheduler) startDeal(ctx context.Context) {
	go func(s *Scheduler) {
		for {
			select {
			case start := <-s.t.C:
				s.deal(ctx, start)
			case <-ctx.Done():
				s.logger.Info("scheduler: stopped!")
				return
			}
		}
	}(s)
}

func (s *Scheduler) deal(ctx context.Context, start time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.p == nil {
		s.logger.Warn("scheduler: should close when the project is nil!")
		return
	}
	if len(s.plugins) != 0 {
		for _, p := range s.plugins {
			err := p.BeforeCapture()
			if err != nil {
				s.logger.Warnf("scheduler: run plugin before failed, %s", err)
			}
		}
	}
	// Allow long exposures to finish, but never block the scheduler indefinitely.
	captureCtx, cancel := context.WithTimeout(ctx, time.Minute)
	frame, captureErr := s.captureFrame(captureCtx)
	cancel()
	if len(s.plugins) != 0 {
		for _, p := range s.plugins {
			err := p.AfterCapture()
			if err != nil {
				s.logger.Warnf("scheduler: run plugin before failed, %s", err)
			}
		}
	}
	if captureErr != nil {
		s.logger.Warnf("scheduler: capture failed: %s", captureErr)
		return
	}
	err := s.p.SaveImage(frame)
	if err != nil {
		s.logger.Warnf("scheduler: save image err: %s", err)
	}
	err = s.stg.UpdateProject(s.p)
	if err != nil {
		s.logger.Warnf("scheduler: save update image err: %s", err)
	}

	s.logger.Infof("scheduler: took %s to get the image", time.Now().Sub(start))
}

// captureFrame is called with s.lock held. The camera's bounded output retains
// old frames when full, so draining it cannot yield a current capture. Discard
// that backlog, then wait for a newly delivered non-empty frame. After a start
// or mode change, also consume the sensor's initial settling frames.
func (s *Scheduler) captureFrame(ctx context.Context) ([]byte, error) {
	if s.input == nil {
		return nil, fmt.Errorf("capture stream unavailable")
	}
	// Drain only the existing backlog, not frames arriving during warmup.
	for remaining := len(s.input); remaining > 0; remaining-- {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case _, ok := <-s.input:
			if !ok {
				return nil, fmt.Errorf("capture stream closed")
			}
		}
	}
	for discarded := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case frame, ok := <-s.input:
			if !ok {
				return nil, fmt.Errorf("capture stream closed")
			}
			if len(frame) == 0 {
				continue
			}
			if discarded < s.warmupFrames {
				discarded++
				continue
			}
			s.warmupFrames = 0
			return frame, nil
		}
	}
}
