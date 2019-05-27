package enclave

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
	"github.com/stretchr/testify/assert"
)

func TestTargetFinder(t *testing.T) {

	m := make(map[string][]string)
	m["test1"] = []string{"joel", "claire", "caroline", "françois"}
	m["test2"] = []string{"paul", "claire", "françois"}
	m["test3"] = []string{"paul", "claire", "françois"}
	m["test4"] = []string{"paul", "benjamin", "florent"}

	in := dispers.InputTF{
		ListsOfAddresses: m,
		TargetProfile: &dispers.Union{
			ValueA: &dispers.Intersection{
				ValueA: &dispers.Single{Value: "test1"},
				ValueB: &dispers.Single{Value: "test2"},
			},
			ValueB: &dispers.Intersection{
				ValueA: &dispers.Single{Value: "test3"},
				ValueB: &dispers.Single{Value: "test4"},
			},
		},
	}

	res, err := SelectAddresses(in)
	assert.NoError(t, err)
	assert.Equal(t, res, []string{"claire", "françois", "paul"})

}
