package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceImplems(t *testing.T) {
	assert.Implements(t, (*Service)(nil), new(SettingsService))
	assert.Implements(t, (*Service)(nil), new(Mock))
}
