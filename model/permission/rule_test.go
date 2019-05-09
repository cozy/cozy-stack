package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeRulesIdentical(t *testing.T) {
	rule1 := Rule{
		Title:       "myrule1",
		Type:        "io.cozy.files",
		Description: "description of rule1",
		Verbs:       Verbs(GET),
		Values:      []string{},
	}

	rule2 := Rule{
		Title:       "myrule1",
		Type:        "io.cozy.files",
		Description: "description of rule1",
		Verbs:       Verbs(GET),
		Values:      []string{},
	}

	newRule, err := rule1.Merge(rule2)
	assert.NoError(t, err)
	assert.Equal(t, &rule2, newRule)
}
func TestMergeRules(t *testing.T) {

	rule1 := Rule{
		Title:       "myrule1",
		Type:        "io.cozy.files",
		Description: "description of rule1",
		Verbs:       Verbs(GET),
		Values:      []string{},
	}

	rule2 := Rule{
		Title:       "myrule2",
		Type:        "io.cozy.files",
		Description: "description of rule2",
		Verbs:       Verbs(GET, POST),
		Values:      []string{"io"},
	}

	expectedRule := &Rule{
		Title:       "myrule1",
		Type:        "io.cozy.files",
		Description: "description of rule1",
		Verbs:       Verbs(GET, POST),
		Values:      []string{"io"},
	}

	newRule, err := rule1.Merge(rule2)
	assert.NoError(t, err)
	assert.Equal(t, expectedRule, newRule)
}

func TestMergeRulesBadType(t *testing.T) {
	rule1 := Rule{
		Title:       "myrule1",
		Type:        "io.cozy.files",
		Description: "description of rule1",
		Verbs:       Verbs(GET),
		Values:      []string{},
	}

	rule2 := Rule{
		Title:       "myrule2",
		Type:        "io.cozy.contacts",
		Description: "description of rule2",
		Verbs:       Verbs(GET, POST),
		Values:      []string{"io"},
	}

	newRule, err := rule1.Merge(rule2)
	assert.Error(t, err)
	assert.Nil(t, newRule)
	assert.Contains(t, err.Error(), "type is different")
}
