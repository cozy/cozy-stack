package job

import (
	"context"
	"testing"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/mock"
)

// BrokerMock is a mock implementation of [Broker].
type BrokerMock struct {
	mock.Mock
}

func NewBrokerMock(t *testing.T) *BrokerMock {
	m := new(BrokerMock)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

// StartWorkers mock method.
func (m *BrokerMock) StartWorkers(workersList WorkersList) error {
	return m.Called(workersList).Error(0)
}

// ShutdownWorkers mock method.
func (m *BrokerMock) ShutdownWorkers(ctx context.Context) error {
	return m.Called().Error(0)
}

// PushJob mock method.
func (m *BrokerMock) PushJob(db prefixer.Prefixer, request *JobRequest) (*Job, error) {
	args := m.Called(db, request)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*Job), args.Error(1)
}

// WorkerQueueLen mock method.
func (m *BrokerMock) WorkerQueueLen(workerType string) (int, error) {
	args := m.Called(workerType)

	return args.Int(0), args.Error(1)
}

// WorkerIsReserved mock method.
func (m *BrokerMock) WorkerIsReserved(workerType string) (bool, error) {
	args := m.Called(workerType)

	return args.Bool(0), args.Error(1)
}

// WorkersTypes mock method.
func (m *BrokerMock) WorkersTypes() []string {
	args := m.Called()

	if args.Get(0) == nil {
		return nil
	}

	return args.Get(0).([]string)
}
