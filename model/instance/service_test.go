package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceImplementations(t *testing.T) {
	assert.Implements(t, (*Service)(nil), new(Mock))
	assert.Implements(t, (*Service)(nil), new(InstanceService))
}
