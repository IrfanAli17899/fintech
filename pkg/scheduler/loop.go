package scheduler

import (
	"context"
	"log"
	"time"
)

// Loop runs fn every interval until Stop. Panics in fn are recovered and
// logged so a bad run never takes the process down.
type Loop struct {
	name     string
	interval time.Duration
	fn       func(context.Context)
	cancel   context.CancelFunc
	done     chan struct{}
}

func New(name string, interval time.Duration, fn func(context.Context)) *Loop {
	return &Loop{name: name, interval: interval, fn: fn}
}

func (l *Loop) Start(ctx context.Context) {
	ctx, l.cancel = context.WithCancel(ctx)
	l.done = make(chan struct{})
	go func() {
		defer close(l.done)
		t := time.NewTicker(l.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.run(ctx)
			}
		}
	}()
}

func (l *Loop) run(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("scheduler %s: panic: %v", l.name, p)
		}
	}()
	l.fn(ctx)
}

// Stop cancels the loop and waits for any in-progress run to finish.
func (l *Loop) Stop() {
	if l.cancel == nil {
		return
	}
	l.cancel()
	<-l.done
}
