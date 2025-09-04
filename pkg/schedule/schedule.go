package schedule

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"plant-shutter-pi/pkg/storage/project"
	"plant-shutter-pi/pkg/utils"
)

type Scheduler struct {
	t      *time.Ticker
	input  <-chan []byte
	p      *project.Project
	lock   sync.Mutex
	logger *zap.SugaredLogger
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
				s.lock.Lock()
				if s.p == nil {
					s.logger.Warn("scheduler: should close when the project is nil!")
					continue
				}
				frame := <-s.input
				err := s.p.SaveImage(frame)
				if err != nil {
					s.logger.Warnf("scheduler: save image err: %s", err)
				}
				s.lock.Unlock()
				s.logger.Infof("scheduler: took %s to get the image", time.Now().Sub(start))
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
