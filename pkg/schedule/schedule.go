package schedule

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"plant-shutter-pi/pkg/camera"
	"plant-shutter-pi/pkg/storage/consts"

	"plant-shutter-pi/pkg/storage/project"
	"plant-shutter-pi/pkg/utils"
)

type Scheduler struct {
	t          *time.Ticker
	controller *camera.Controller
	p          *project.Project
	lock       sync.Mutex
	logger     *zap.SugaredLogger
}

func New(ctx context.Context, controller *camera.Controller) *Scheduler {
	t := time.NewTicker(time.Second)
	t.Stop()

	s := &Scheduler{
		t:          t,
		controller: controller,
		logger:     utils.GetLogger(),
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
				s.logger.Debugf("scheduler: starting deal: %v", start)
				if s.p == nil {
					s.logger.Warn("scheduler: should close when the project is nil!")
					continue
				}
				frame, err := s.controller.Capture(consts.Width, consts.Height)
				if err != nil {
					s.logger.Errorf("get frame error: %s", err)
				} else {
					if err = s.p.SaveImage(frame); err != nil {
						s.logger.Errorf("scheduler: save image err: %s", err)
					}
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
