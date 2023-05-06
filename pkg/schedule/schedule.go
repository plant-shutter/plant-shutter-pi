package schedule

import "time"

type Scheduler struct {
	t *time.Ticker
}

func New(duration time.Duration) Scheduler {
	return Scheduler{t: time.NewTicker(duration)}
}

func (s Scheduler) Channel() <-chan time.Time {
	return s.t.C
}
