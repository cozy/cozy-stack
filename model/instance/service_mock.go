package instance

import (
	"testing"

	"github.com/stretchr/testify/mock"
)

// Mock implementation of
type Mock struct {
	mock.Mock
}

// NewMock instantiates a new [Mock].
func NewMock(t *testing.T) *Mock {
	m := new(Mock)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

// Get mock method.
func (m *Mock) Get(domain string) (*Instance, error) {
	args := m.Called(domain)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*Instance), args.Error(1)
}

// GetFromCouch mock method.
func (m *Mock) GetFromCouch(domain string) (*Instance, error) {
	args := m.Called(domain)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*Instance), args.Error(1)
}

// Update mock method.
func (m *Mock) Update(inst *Instance) error {
	return m.Called(inst).Error(1)
}

// Delete mock method.
func (m *Mock) Delete(inst *Instance) error {
	return m.Called(inst).Error(1)
}
