package middlewares

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendCSPRule(t *testing.T) {
	r := appendCSPRule("", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors new-rule;", r)

	r = appendCSPRule("frame-ancestors;", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors new-rule;", r)

	r = appendCSPRule("frame-ancestors 1 2 3 ;", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors 1 2 3 new-rule;", r)

	r = appendCSPRule("frame-ancestors 1 2 3 ;", "frame-ancestors", "new-rule", "new-rule-2")
	assert.Equal(t, "frame-ancestors 1 2 3 new-rule new-rule-2;", r)

	r = appendCSPRule("frame-ancestors 'none';", "frame-ancestors", "new-rule")
	assert.Equal(t, "frame-ancestors new-rule;", r)

	r = appendCSPRule("script '*'; frame-ancestors 'self';", "frame-ancestors", "new-rule")
	assert.Equal(t, "script '*'; frame-ancestors 'self' new-rule;", r)

	r = appendCSPRule("script '*'; frame-ancestors 'self'; plop plop;", "frame-ancestors", "new-rule")
	assert.Equal(t, "script '*'; frame-ancestors 'self' new-rule; plop plop;", r)

	r = appendCSPRule("script '*'; toto;", "frame-ancestors", "new-rule")
	assert.Equal(t, "script '*'; toto;frame-ancestors new-rule;", r)
}
