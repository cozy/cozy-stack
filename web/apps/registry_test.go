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

func TestRegistryVersionHandler(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/apps/registries/Collect/3.0.3", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Host = testInstance.Domain
	res, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	var results map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&results)
	assert.NoError(t, err)
	data := results["data"].(map[string]interface{})
	id := data["id"].(string)
	assert.Equal(t, "Collect/3.0.3", id)
	typ := data["type"].(string)
	assert.Equal(t, "io.cozy.registry.versions", typ)

	attrs := data["attributes"].(map[string]interface{})
	name := attrs["name"].(string)
	assert.Equal(t, "Collect", name)
	version := attrs["version"].(string)
	assert.Equal(t, "3.0.3", version)
	u := attrs["url"].(string)
	assert.Equal(t, "https://github.com/cozy/cozy-collect/releases/download/v3.0.3/cozy-collect-v3.0.3.tgz", u)
	sha := attrs["sha256"].(string)
	assert.Equal(t, "1332d2301c2362f207cf35880725179157368a921253293b062946eb6d96e3ae", sha)
	size := attrs["size"].(string)
	assert.Equal(t, "3821149", size)

	man := attrs["manifest"].(map[string]interface{})
	slug := man["slug"].(string)
	assert.Equal(t, "collect", slug)
	perms := man["permissions"].(map[string]interface{})
	settings := perms["settings"].(map[string]interface{})
	assert.Equal(t, "io.cozy.settings", settings["type"].(string))
}
