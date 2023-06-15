package settings

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of [Service].
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

// GetInstanceSettings mock method.
func (m *Mock) GetInstanceSettings(inst prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	args := m.Called(inst)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*couchdb.JSONDoc), args.Error(1)
}

// SetInstanceSettings mock method.
func (m *Mock) SetInstanceSettings(inst prefixer.Prefixer, doc *couchdb.JSONDoc) error {
	return m.Called(inst, doc).Error(0)
}
