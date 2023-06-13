package emailer

import (
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of [Emailer].
type Mock struct {
	mock.Mock
}

// NewMock instantiates a new [Mock].
func NewMock(t *testing.T) *Mock {
	m := new(Mock)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

// SendEmail mock method.
func (m *Mock) SendEmail(inst *instance.Instance, cmd *SendEmailCmd) error {
	return m.Called(inst, cmd).Error(0)
}
