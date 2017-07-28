package utils

import (
	"context"

	multierror "github.com/hashicorp/go-multierror"
)

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

// Shutdown implement the Shutdown of all the encapsulated Shutdowner contained
// in the group.
func (g *GroupShutdown) Shutdown(ctx context.Context) error {
	var errm error
	for _, s := range g.s {
		if err := s.Shutdown(ctx); err != nil {
			// no need to continue if the context has errored
			if err == ctx.Err() {
				return err
			}
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}
