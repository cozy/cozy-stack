package emailer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmailerImplems(t *testing.T) {
	assert.Implements(t, (*Emailer)(nil), new(EmailerService))
	assert.Implements(t, (*Emailer)(nil), new(Mock))
}
