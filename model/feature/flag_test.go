package feature

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uuidv7() string {
	return uuid.Must(uuid.NewV7()).String()
}

func TestFeatureFlagRatio(t *testing.T) {
	inst := instance.Instance{
		DocID:       uuidv7(),
		ContextName: "testing",
	}
	var data []interface{}
	err := json.Unmarshal([]byte(`[
	{"ratio": 0.1, "value": 1},
	{"ratio": 0.2, "value": 2},
	{"ratio": 0.4, "value": 4}
]`), &data)
	assert.NoError(t, err)

	results := make(map[interface{}]int)
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("%d-feature", i)
		value := applyRatio(&inst, key, data)
		results[value]++
	}
	assert.InDelta(t, 1000, results[float64(1)], 100)
	assert.InDelta(t, 2000, results[float64(2)], 100)
	assert.InDelta(t, 4000, results[float64(4)], 100)
	assert.InDelta(t, 3000, results[nil], 100)
}

func TestFeatureFlagList(t *testing.T) {
	var flags Flags
	err := json.Unmarshal(
		[]byte(`{
		  "flag1": { "list": ["other", "val"] },
		  "flag2": { "list": ["other"] },
		  "flag3": { "list": [] },
		  "flag4": ["val"],
		  "flag5": "val"
		}`),
		&flags,
	)
	require.NoError(t, err)

	// GetList
	list, err := flags.GetList("flag1")
	require.NoError(t, err)
	assert.EqualValues(t, []interface{}{"other", "val"}, list)
	_, err = flags.GetList("flag4")
	assert.Error(t, err)
	_, err = flags.GetList("flag5")
	assert.Error(t, err)

	// HasListItem
	hasVal, err := flags.HasListItem("flag1", "val")
	require.NoError(t, err)
	assert.True(t, hasVal)
	hasVal, _ = flags.HasListItem("flag2", "val")
	assert.False(t, hasVal)
	hasVal, _ = flags.HasListItem("flag3", "val")
	assert.False(t, hasVal)
	_, err = flags.HasListItem("flag4", "val")
	assert.Error(t, err)
	_, err = flags.HasListItem("flag5", "val")
	assert.Error(t, err)
}
