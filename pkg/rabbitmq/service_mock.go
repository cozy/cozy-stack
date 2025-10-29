package rabbitmq

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of [RabbitMQService].
type Mock struct {
	mock.Mock
}

// NewMock instantiates a new [Mock].
func NewMock(t *testing.T) *Mock {
	m := new(Mock)
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

func (m *Mock) StartManagers() ([]utils.Shutdowner, error) {
	args := m.Called()

	return args.Get(0).([]utils.Shutdowner), args.Error(1)
}
