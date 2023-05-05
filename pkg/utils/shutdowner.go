package utils

import (
	"context"
	"sync"

	"github.com/hashicorp/go-multierror"
)

// NopShutdown implements the Shutdowner interface but does not execute any
// process on shutdown.
var NopShutdown = NewGroupShutdown()

// Shutdowner is an interface with a Shutdown method to gracefully shutdown
// a running process.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// GroupShutdown allow to group multiple Shutdowner into a single one.
type GroupShutdown struct {
	s []Shutdowner
}

// NewGroupShutdown returns a new GroupShutdown
func NewGroupShutdown(s ...Shutdowner) *GroupShutdown {
	return &GroupShutdown{s}
}

// Shutdown closes all the encapsulated [Shutdowner] in parallel an returns
// the concatenated errors.
func (g *GroupShutdown) Shutdown(ctx context.Context) error {
	var errm error
	l := sync.Mutex{}
	w := sync.WaitGroup{}

	for _, s := range g.s {
		// Shadow the variable to avoid a datarace
		s := s
		w.Add(1)

		go (func() {
			defer w.Done()

			err := s.Shutdown(ctx)
			if err != nil {
				l.Lock()
				defer l.Unlock()
				errm = multierror.Append(errm, err)
			}
		})()
	}

	w.Wait()

	return errm
}
