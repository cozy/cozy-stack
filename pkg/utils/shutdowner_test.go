package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type appStub struct {
	mock.Mock
}

func newAppStub(t *testing.T) *appStub {
	s := new(appStub)

	t.Cleanup(func() { s.AssertExpectations(t) })

	return s
}

func (s *appStub) Shutdown(ctx context.Context) error {
	return s.Called().Error(0)
}

func TestShutdowner_ok(t *testing.T) {
	s1 := newAppStub(t)
	s2 := newAppStub(t)
	s3 := newAppStub(t)

	s1.On("Shutdown").Return(nil).Once()
	s2.On("Shutdown").Return(nil).Once()
	s3.On("Shutdown").Return(nil).Once()

	group := NewGroupShutdown(s1, s2, s3)

	err := group.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestShutdowner_return_errors(t *testing.T) {
	s1 := newAppStub(t)
	s2 := newAppStub(t)
	s3 := newAppStub(t)

	s1.On("Shutdown").Return(nil).Once()
	s2.On("Shutdown").Return(errors.New("some-error")).Once()
	s3.On("Shutdown").Return(nil).Once()

	group := NewGroupShutdown(s1, s2, s3)

	err := group.Shutdown(context.Background())
	require.EqualError(t, err, "some-error")
}

func TestShutdowner_are_run_in_parallel(t *testing.T) {
	s1 := newAppStub(t)
	s2 := newAppStub(t)
	s3 := newAppStub(t)

	s1.On("Shutdown").Return(nil).Once().WaitUntil(time.After(5 * time.Millisecond))
	s2.On("Shutdown").Return(nil).Once().WaitUntil(time.After(5 * time.Millisecond))
	s3.On("Shutdown").Return(nil).Once().WaitUntil(time.After(5 * time.Millisecond))

	group := NewGroupShutdown(s1, s2, s3)

	start := time.Now()
	err := group.Shutdown(context.Background())
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), start, 10*time.Millisecond)
}
