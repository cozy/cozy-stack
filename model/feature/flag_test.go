package feature

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/assert"
)

func uuidv4() string {
	id, _ := uuid.NewV4()
	return id.String()
}

func TestFeatureFlagRatio(t *testing.T) {
	inst := instance.Instance{
		DocID:       uuidv4(),
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
