package schedule

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"plant-shutter-pi/pkg/camera"
	"plant-shutter-pi/pkg/plugin"

	"plant-shutter-pi/pkg/storage/project"
	"plant-shutter-pi/pkg/utils"
)

type Scheduler struct {
	t       *time.Ticker
	input   <-chan []byte
	p       *project.Project
	plugins []plugin.Plugin
	lock    sync.Mutex
	logger  *zap.SugaredLogger
}

func New(ctx context.Context, input <-chan []byte) *Scheduler {
	t := time.NewTicker(time.Second)
	t.Stop()

	s := &Scheduler{
		t:      t,
		input:  input,
		logger: utils.GetLogger(),
	}
	s.startDeal(ctx)

	return s
}

func (s *Scheduler) Begin(p *project.Project) {
	if p == nil {
		s.Stop()
	}
	s.lock.Lock()
	s.p = p
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

func (s *Scheduler) GetProject() *project.Project {
	if s.p == nil {
		return nil
	}

	return &*s.p
}

func (s *Scheduler) startDeal(ctx context.Context) {
	go func(s *Scheduler) {
		for {
			select {
			case start := <-s.t.C:
				s.deal(ctx, start)
			case <-ctx.Done():
				s.lock.Lock()
				if s.p != nil {
					_ = s.p.Close()
				}
				s.lock.Unlock()
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
	frame, ok := camera.DrainLatest(ctx, nil, s.input)
	if !ok {
		s.logger.Warnf("scheduler: input channel closed")
		return
	}
	if len(s.plugins) != 0 {
		for _, p := range s.plugins {
			err := p.AfterCapture()
			if err != nil {
				s.logger.Warnf("scheduler: run plugin before failed, %s", err)
			}
		}
	}
	err := s.p.SaveImage(frame)
	if err != nil {
		s.logger.Warnf("scheduler: save image err: %s", err)
	}

	s.logger.Infof("scheduler: took %s to get the image", time.Now().Sub(start))
}
