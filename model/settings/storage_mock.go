package settings

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/mock"
)

type storageMock struct {
	mock.Mock
}

func newStorageMock(t *testing.T) *storageMock {
	m := new(storageMock)
	m.Test(t)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

func (m *storageMock) setInstanceSettings(inst prefixer.Prefixer, doc *couchdb.JSONDoc) error {
	return m.Called(inst, doc).Error(0)
}

func (m *storageMock) getInstanceSettings(inst prefixer.Prefixer) (*couchdb.JSONDoc, error) {
	args := m.Called(inst)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*couchdb.JSONDoc), args.Error(1)
}
