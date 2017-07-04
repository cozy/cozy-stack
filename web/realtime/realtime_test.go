package realtime

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var inst *instance.Instance
var token string

type testDoc struct {
	id      string
	doctype string
}

func (t *testDoc) ID() string      { return t.id }
func (t *testDoc) DocType() string { return t.doctype }
func (t *testDoc) MarshalJSON() ([]byte, error) {
	j := `{"_id":"` + t.id + `", "_type":"` + t.doctype + `"}`
	return []byte(j), nil
}

func TestWSNoAuth(t *testing.T) {
	u := strings.Replace(ts.URL+"/realtime/", "http", "ws", 1)
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	assert.NoError(t, err)
	defer c.Close()

	msg := `{"method": "SUBSCRIBE", "payload": { "type": "io.cozy.foos" }}`
	err = c.WriteMessage(websocket.TextMessage, []byte(msg))
	assert.NoError(t, err)

	var res map[string]interface{}
	err = c.ReadJSON(&res)
	assert.NoError(t, err)
	assert.Equal(t, "error", res["event"])
	payload := res["payload"].(map[string]interface{})
	assert.Equal(t, "405 Method Not Allowed", payload["status"])
	assert.Equal(t, "method not allowed", payload["code"])
	assert.Equal(t, "The SUBSCRIBE method is not supported", payload["title"])
}

func TestWSInvalidToken(t *testing.T) {
	u := strings.Replace(ts.URL+"/realtime/", "http", "ws", 1)
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	assert.NoError(t, err)
	defer c.Close()

	auth := `{"method": "AUTH", "payload": "123456789"}`
	err = c.WriteMessage(websocket.TextMessage, []byte(auth))
	assert.NoError(t, err)

	var res map[string]interface{}
	err = c.ReadJSON(&res)
	assert.NoError(t, err)
	assert.Equal(t, "error", res["event"])
	payload := res["payload"].(map[string]interface{})
	assert.Equal(t, "401 Unauthorized", payload["status"])
	assert.Equal(t, "unauthorized", payload["code"])
	assert.Equal(t, "The authentication has failed", payload["title"])
}

func TestWSNoPermissionsForADoctype(t *testing.T) {
	u := strings.Replace(ts.URL+"/realtime/", "http", "ws", 1)
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	assert.NoError(t, err)
	defer c.Close()

	auth := fmt.Sprintf(`{"method": "AUTH", "payload": "%s"}`, token)
	err = c.WriteMessage(websocket.TextMessage, []byte(auth))
	assert.NoError(t, err)

	msg := `{"method": "SUBSCRIBE", "payload": { "type": "io.cozy.contacts" }}`
	err = c.WriteMessage(websocket.TextMessage, []byte(msg))
	assert.NoError(t, err)

	var res map[string]interface{}
	err = c.ReadJSON(&res)
	assert.NoError(t, err)
	assert.Equal(t, "error", res["event"])
	payload := res["payload"].(map[string]interface{})
	assert.Equal(t, "403 Forbidden", payload["status"])
	assert.Equal(t, "forbidden", payload["code"])
	assert.Equal(t, "The application can't subscribe to io.cozy.contacts", payload["title"])
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "realtime_test")
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient("io.cozy.foos io.cozy.bars")
	ts = setup.GetTestServer("/realtime", Routes)
	os.Exit(setup.Run())
}
