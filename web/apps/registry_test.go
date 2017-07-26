package apps_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistryListHandler(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/apps/registries", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Host = testInstance.Domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	var results map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&results)
	assert.NoError(t, err)
	data := results["data"].([]interface{})
	assert.NotZero(t, len(data))

	obj := data[0].(map[string]interface{})
	id := obj["id"].(string)
	assert.Equal(t, "Onboarding", id)
	typ := obj["type"].(string)
	assert.Equal(t, "io.cozy.registry.webapps", typ)

	attrs := obj["attributes"].(map[string]interface{})
	name := attrs["name"].(string)
	assert.Equal(t, "Onboarding", name)
	editor := attrs["editor"].(string)
	assert.Equal(t, "Cozy", editor)
	repos := attrs["repository"].(string)
	assert.Equal(t, "https://github.com/cozy/cozy-onboarding-v3", repos)

	descriptions := attrs["description"].(map[string]interface{})
	descEn := descriptions["en"].(string)
	assert.Equal(t, "Register application for Cozy v3", descEn)
	descFr := descriptions["fr"].(string)
	assert.Equal(t, "Application pour l'embarquement de Cozy v3", descFr)

	tags := attrs["tags"].([]interface{})
	assert.Len(t, tags, 1)
	assert.Equal(t, "welcome", tags[0].(string))

	versions := attrs["versions"].(map[string]interface{})
	stable := versions["stable"].([]interface{})
	assert.Len(t, stable, 1)
	assert.Equal(t, "3.0.0", stable[0].(string))
}
