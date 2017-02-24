package sharings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var testInstance *instance.Instance

type jsonData struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Attrs map[string]interface{} `json:"attributes,omitempty"`
	Rels  map[string]interface{} `json:"relationships,omitempty"`
}

func TestSharingRequestNoScope(t *testing.T) {
	urlVal := url.Values{}
	res, err := requestGET(urlVal, "/sharings/request")
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoState(t *testing.T) {
	urlVal := url.Values{}
	urlVal["scope"] = []string{"dummyscope"}
	res, err := requestGET(urlVal, "/sharings/request")
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoSharingType(t *testing.T) {
	urlVal := url.Values{}
	urlVal["scope"] = []string{"dummyscope"}
	urlVal["state"] = []string{"dummystate"}
	res, err := requestGET(urlVal, "/sharings/request")
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestSharingRequestBadScope(t *testing.T) {
	urlVal := url.Values{}
	urlVal["scope"] = []string{":"}
	urlVal["state"] = []string{"dummystate"}
	urlVal["sharing_type"] = []string{consts.OneShotSharing}

	_, err := requestGET(urlVal, "/sharings/request")
	assert.NoError(t, err)
	//assert.Equal(t, 422, res.StatusCode)

}

func TestSharingRequestSuccess(t *testing.T) {
	rule := permissions.Rule{
		Type:        "io.cozy.events",
		Title:       "event",
		Description: "My event",
		Verbs:       permissions.VerbSet{permissions.POST: {}},
		Values:      []string{"1234"},
	}
	set := permissions.Set{rule}
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	state := "sharing_id"

	urlVal := url.Values{
		"state":        []string{state},
		"scope":        []string{scope},
		"sharing_type": []string{consts.OneShotSharing},
	}
	res, err := requestGET(urlVal, "/sharings/request")
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)

	var obj map[string]interface{}
	err = extractJSONRes(res, &obj)
	assert.NoError(t, err)
	data := obj["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	owner := attrs["owner"].(bool)
	assert.Equal(t, false, owner)
	sharingType := attrs["sharing_type"].(string)
	assert.Equal(t, consts.OneShotSharing, sharingType)
	sharingID := attrs["sharing_id"].(string)
	assert.Equal(t, state, sharingID)

}

func TestCreateSharingWithBadType(t *testing.T) {
	res, err := postJSON("/sharings/", echo.Map{
		"sharing_type": "shary pie",
	})
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestCreateSharingWithNonExistingRecipient(t *testing.T) {
	type recipient map[string]map[string]string

	rec := recipient{
		"recipient": {
			"id": "hodor",
		},
	}
	recipients := []recipient{rec}

	res, err := postJSON("/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
		"recipients":   recipients,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateSharingSuccess(t *testing.T) {
	res, err := postJSON("/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
	})
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test needs couchdb to run.")
		os.Exit(1)
	}

	instance.Destroy("test-sharings")
	testInstance, err = instance.Create(&instance.Options{
		Domain: "test-sharings",
		Locale: "en",
	})
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}

	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	handler.Use(injectInstance(testInstance))
	Routes(handler.Group("/sharings"))

	ts = httptest.NewServer(handler)

	res := m.Run()
	ts.Close()
	instance.Destroy("test-sharings")

	os.Exit(res)
}

func postJSON(u string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	return http.Post(ts.URL+u, "application/json", bytes.NewReader(body))
}

func requestGET(v url.Values, u string) (*http.Response, error) {
	reqURL := v.Encode()
	fmt.Printf("url : %v\n", reqURL)
	return http.Get(ts.URL + u + "?" + reqURL)
}

func extractJSONRes(res *http.Response, mp *map[string]interface{}) error {
	if res.StatusCode >= 300 {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(mp)
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}
