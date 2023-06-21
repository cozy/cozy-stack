package token

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of [Service].
type Mock struct {
	mock.Mock
}

// NewMock instantiates a new Mock.
func NewMock(t *testing.T) *Mock {
	m := new(Mock)
	m.Test(t)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

// GenerateAndSave mock method.
func (m *Mock) GenerateAndSave(db prefixer.Prefixer, op Operation, resource string, lifetime time.Duration) (string, error) {
	args := m.Called(db, op, resource, lifetime)

	return args.String(0), args.Error(1)
}

// Validate mock method.
func (m *Mock) Validate(db prefixer.Prefixer, op Operation, resource, token string) error {
	return m.Called(db, op, resource, token).Error(0)
}
