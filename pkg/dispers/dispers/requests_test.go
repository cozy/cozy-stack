package dispers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnion(t *testing.T) {

	m := make(map[string][]string)
	m["test1"] = []string{"abc", "bcd", "efc"}
	m["test2"] = []string{"hihi"}

	op := Union{
		ValueA: &Single{Value: "test1"},
		ValueB: &Single{Value: "test2"},
	}

	res, err := op.Compute(m)
	assert.NoError(t, err)
	assert.Equal(t, res, []string{"abc", "bcd", "efc", "hihi"})

}

func TestIntersection(t *testing.T) {

	m := make(map[string][]string)
	m["test1"] = []string{"joel", "claire", "caroline", "françois"}
	m["test2"] = []string{"paul", "claire", "françois"}

	op := Intersection{
		ValueA: &Single{Value: "test1"},
		ValueB: &Single{Value: "test2"},
	}

	res, err := op.Compute(m)
	assert.NoError(t, err)
	assert.Equal(t, res, []string{"claire", "françois"})

}

func TestIntersectionAndUnion(t *testing.T) {

	m := make(map[string][]string)
	m["test1"] = []string{"joel", "claire", "caroline", "françois"}
	m["test2"] = []string{"paul", "claire", "françois"}
	m["test3"] = []string{"paul", "claire", "françois"}
	m["test4"] = []string{"paul", "benjamin", "florent"}

	op := Union{
		ValueA: &Intersection{
			ValueA: &Single{Value: "test1"},
			ValueB: &Single{Value: "test2"},
		},
		ValueB: &Intersection{
			ValueA: &Single{Value: "test3"},
			ValueB: &Single{Value: "test4"},
		},
	}

	res, err := op.Compute(m)
	assert.NoError(t, err)
	assert.Equal(t, res, []string{"claire", "françois", "paul"})

}

func TestBlankLeaf(t *testing.T) {

	m := make(map[string][]string)
	m["test1"] = []string{""}
	m["test2"] = []string{"paul", "claire", "françois"}

	op := Intersection{
		ValueA: &Single{Value: "test1"},
		ValueB: &Single{Value: "test2"},
	}

	_, err := op.Compute(m)
	assert.NoError(t, err)

}

func TestUnknownConcept(t *testing.T) {

	m := make(map[string][]string)
	m["test1"] = []string{""}
	m["test2"] = []string{"paul", "claire", "françois"}

	op := Intersection{
		ValueA: &Single{Value: "test3"},
		ValueB: &Single{Value: "test2"},
	}

	_, err := op.Compute(m)
	assert.Error(t, err)

}
