package cloudery

import (
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/stretchr/testify/mock"
)

// Mock impelementation of [Service].
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

// SaveInstance mock method.
func (m *Mock) SaveInstance(inst *instance.Instance, cmd *SaveCmd) error {
	return m.Called(inst, cmd).Error(0)
}

func (m *Mock) BlockingSubscription(inst *instance.Instance) (*BlockingSubscription, error) {
	args := m.Called(inst)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*BlockingSubscription), args.Error(1)
}
