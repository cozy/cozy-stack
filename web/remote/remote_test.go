package remote

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var testInstance *instance.Instance
var ts *httptest.Server
var token string

func TestRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "remote_test")

	testInstance = setup.GetTestInstance()
	token = generateAppToken(testInstance, "answers", "org.wikidata.entity")

	ts = setup.GetTestServer("/remote", Routes)

	t.Run("RemoteGET", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/remote/org.wikidata.entity?entity=Q42&comment=foo", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Host = testInstance.Domain
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		body, _ := io.ReadAll(res.Body)
		assert.Equal(t, `{"entities":`, string(body[:12]))

		var results []map[string]interface{}
		allReq := &couchdb.AllDocsRequest{
			Descending: true,
			Limit:      1,
		}
		err = couchdb.GetAllDocs(testInstance, consts.RemoteRequests, allReq, &results)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		logged := results[0]
		assert.Equal(t, "org.wikidata.entity", logged["doctype"].(string))
		assert.Equal(t, "GET", logged["verb"].(string))
		assert.Equal(t, "https://www.wikidata.org/wiki/Special:EntityData/Q42.json", logged["url"].(string))
		assert.Equal(t, float64(200), logged["response_code"].(float64))
		assert.Equal(t, "application/json", logged["content_type"].(string))
		assert.NotNil(t, logged["created_at"])
		vars := logged["variables"].(map[string]interface{})
		assert.Equal(t, "Q42", vars["entity"].(string))
		assert.Equal(t, "foo", vars["comment"].(string))
		meta, _ := logged["cozyMetadata"].(map[string]interface{})
		assert.Equal(t, "answers", meta["createdByApp"])
	})

}

func generateAppToken(inst *instance.Instance, slug, doctype string) string {
	rules := permission.Set{
		permission.Rule{
			Type:  doctype,
			Verbs: permission.ALL,
		},
	}
	permReq := permission.Permission{
		Permissions: rules,
		Type:        permission.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}
	err := couchdb.CreateDoc(inst, &permReq)
	if err != nil {
		return ""
	}
	manifest := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":         consts.Apps + "/" + slug,
			"slug":        slug,
			"permissions": rules,
		},
	}
	err = couchdb.CreateNamedDocWithDB(inst, manifest)
	if err != nil {
		return ""
	}
	return inst.BuildAppToken(slug, "")
}
