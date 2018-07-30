package sharing

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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
			Values:  []string{"foo"},
		},
	}
	assert.NoError(t, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:    "referenced_by is OK",
			DocType:  consts.Files,
			Selector: couchdb.SelectorReferencedBy,
			Values:   []string{"io.cozy.tests/123"},
		},
	}
	assert.NoError(t, s.ValidateRules())
	s.Rules = []Rule{
		{
			Title:   "root cannot be shared",
			DocType: consts.Files,
			Values:  []string{consts.RootDirID},
		},
	}
	assert.Equal(t, ErrInvalidRule, s.ValidateRules())
}

func TestRuleAccept(t *testing.T) {
	doc := map[string]interface{}{
		"_id":    "foo",
		"bar":    "baz",
		"groups": []string{"group1", "group2"},
		"one": map[string]interface{}{
			"two": map[string]interface{}{
				"three": "123",
			},
		},
	}
	doctype := "io.cozy.test.foos"
	r := Rule{
		Title:   "test",
		DocType: doctype,
		Values:  []string{"foo"},
	}
	assert.True(t, r.Accept(doctype, doc))
	r.Local = true
	assert.False(t, r.Accept(doctype, doc))
	r.Local = false
	r.DocType = consts.Files
	assert.False(t, r.Accept(doctype, doc))

	// Nested
	r.DocType = doctype
	r.Selector = "bar"
	r.Values = []string{"hello", "baz"}
	assert.True(t, r.Accept(doctype, doc))
	r.Selector = "one.two.three"
	r.Values = []string{"123"}
	assert.True(t, r.Accept(doctype, doc))
	r.Selector = "foo.bar.baz"
	assert.False(t, r.Accept(doctype, doc))
	r.Selector = "one.four.nine"
	assert.False(t, r.Accept(doctype, doc))

	// Arrays
	r.Selector = "groups"
	r.Values = []string{"group1"}
	assert.True(t, r.Accept(doctype, doc))
	r.Values = []string{"group2", "group3"}
	assert.True(t, r.Accept(doctype, doc))
	r.Values = []string{"group4"}
	assert.False(t, r.Accept(doctype, doc))

	// Referenced_by
	file := map[string]interface{}{
		"_id": "84fa49e2-3409-11e8-86de-7fff926238b1",
		couchdb.SelectorReferencedBy: []map[string]interface{}{
			{"type": "io.cozy.playlists", "id": "list1"},
			{"type": "io.cozy.playlists", "id": "list2"},
		},
	}
	r = Rule{
		Title:    "test referenced_by",
		DocType:  consts.Files,
		Selector: couchdb.SelectorReferencedBy,
		Values:   []string{"io.cozy.playlists/list1"},
	}
	assert.True(t, r.Accept(consts.Files, file))
	r.Values = []string{"io.cozy.playlists/list3"}
	assert.False(t, r.Accept(consts.Files, file))
}

func TestTriggersArgs(t *testing.T) {
	r := Rule{
		Title:    "test triggers args",
		DocType:  consts.Files,
		Selector: couchdb.SelectorReferencedBy,
		Values:   []string{"io.cozy.playlists/list1"},
		Update:   "sync",
	}
	expected := "io.cozy.files:UPDATED:io.cozy.playlists/list1:referenced_by"
	assert.Equal(t, expected, r.TriggerArgs())

	doctype := "io.cozy.test.foos"
	r = Rule{
		Title:   "test",
		DocType: doctype,
		Values:  []string{"foo"},
		Add:     "push",
		Update:  "push",
		Remove:  "revoke",
	}
	expected = "io.cozy.test.foos:CREATED,UPDATED:foo"
	assert.Equal(t, expected, r.TriggerArgs())

	r.Local = true
	assert.Equal(t, "", r.TriggerArgs())
}

func TestClearAppInHost(t *testing.T) {
	host := clearAppInHost("example.mycozy.cloud")
	assert.Equal(t, "example.mycozy.cloud", host)
	host = clearAppInHost("example-drive.mycozy.cloud")
	assert.Equal(t, "example.mycozy.cloud", host)
	host = clearAppInHost("my-cozy.example.net")
	assert.Equal(t, "my-cozy.example.net", host)
}
