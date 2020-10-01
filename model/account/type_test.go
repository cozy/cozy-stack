package account

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterByContext(t *testing.T) {
	bar := &AccountType{
		DocID:     "bar.example",
		Slug:      "my-konnector",
		GrantMode: SecretGrant,
		Secret:    "bar",
	}
	foobar := &AccountType{
		DocID:     "foo/bar.example",
		Slug:      "my-konnector",
		GrantMode: SecretGrant,
		Secret:    "foobar",
	}
	qux := &AccountType{
		DocID:     "qux.example",
		Slug:      "my-konnector",
		GrantMode: SecretGrant,
		Secret:    "qux",
	}
	types := []*AccountType{bar, foobar, qux}

	filtered := filterByContext(types, "foo")
	assert.Len(t, filtered, 2)
	assert.Contains(t, filtered, foobar)
	assert.Contains(t, filtered, qux)

	filtered = filterByContext(types, "courge")
	assert.Len(t, filtered, 2)
	assert.Contains(t, filtered, bar)
	assert.Contains(t, filtered, qux)

	filtered = filterByContext([]*AccountType{foobar}, "courge")
	assert.Len(t, filtered, 0)
}
