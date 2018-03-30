package sharing

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
)

func TestValidatesRules(t *testing.T) {
	s := Sharing{}
	assert.Equal(t, ErrNoRules, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:   "values is missing",
			DocType: "io.cozy.tests",
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:  "doctype is missing",
			Values: []string{"foo"},
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:   "doctype is blacklisted",
			DocType: consts.Jobs,
			Values:  []string{"foo"},
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:   "add is invalid",
			DocType: "io.cozy.tests",
			Values:  []string{"foo"},
			Add:     "flip",
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:   "update is invalid",
			DocType: "io.cozy.tests",
			Values:  []string{"foo"},
			Update:  "flip",
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:   "remove is invalid",
			DocType: "io.cozy.tests",
			Values:  []string{"foo"},
			Remove:  "flip",
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:    "selector is OK",
			DocType:  "io.cozy.tests",
			Selector: "qux",
			Values:   []string{"quux"},
		},
		{
			Title:   "add, update and remove are OK",
			DocType: consts.Contacts,
			Values:  []string{"id1", "id2", "id3"},
			Add:     "Sync",
			Update:  "none",
			Remove:  "revoke",
		},
		{
			Title:   "files is OK",
			DocType: consts.Files,
			Values:  []string{"foo", "bar"},
		},
	}
	assert.NoError(t, s.ValidateRules())
}
