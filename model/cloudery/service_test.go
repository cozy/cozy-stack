package cloudery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImplementations(t *testing.T) {
	assert.Implements(t, (*Service)(nil), new(Mock))
	assert.Implements(t, (*Service)(nil), new(ClouderyService))
	assert.Implements(t, (*Service)(nil), new(NoopService))
}
