package settings

import (
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/mock"
)

// Mock implementation of [Service].
type Mock struct {
	mock.Mock
}

// NewServiceMock instantiates a new [Mock].
func NewServiceMock(t *testing.T) *Mock {
	m := new(Mock)
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

// PublicName mock method.
func (m *Mock) PublicName(db prefixer.Prefixer) (string, error) {
	args := m.Called(db)

	return args.String(0), args.Error(1)
}

// GetInstanceSettings mock method.
func (m *Mock) GetInstanceSettings(db prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	args := m.Called(db)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*couchdb.JSONDoc), args.Error(1)
}

// SetInstanceSettings mock method.
func (m *Mock) SetInstanceSettings(db prefixer.Prefixer, doc *couchdb.JSONDoc) error {
	return m.Called(db, doc).Error(0)
}

// StartEmailUpdate mock method.
func (m *Mock) StartEmailUpdate(inst *instance.Instance, cmd *UpdateEmailCmd) error {
	return m.Called(inst, cmd).Error(0)
}
