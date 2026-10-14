package sharings_test

import (
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharingAPIIgnoresReference(t *testing.T) {
	if testing.Short() {
		t.Skip("requires CouchDB")
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			setup := testutils.NewSetup(t, t.Name())
			inst := setup.GetTestInstance()
			ts := setup.GetTestServer("/sharings", sharings.Routes)
			dir, err := vfs.Mkdir(inst.VFS(), "/Drive", nil)
			require.NoError(t, err)
			token := generateAppToken(inst, "drive", consts.Files)
			path := "/sharings/"
			if method == http.MethodPut {
				path += "incoming-drive"
			}
			obj := httpexpect.Default(t, ts.URL).Request(method, path).
				WithHeader("Authorization", "Bearer "+token).
				WithHeader("Content-Type", "application/vnd.api+json").
				WithJSON(map[string]any{"data": map[string]any{
					"id": "incoming-drive", "type": consts.Sharings,
					"attributes": map[string]any{
						"drive":     true,
						"reference": sharing.Reference{Source: "app", ID: "object", Revision: 1},
						"rules":     []sharing.Rule{{Title: "Drive", DocType: consts.Files, Values: []string{dir.ID()}}},
					},
				}}).
				Expect().Status(http.StatusCreated).
				JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).Object()
			id := obj.Path("$.data.id").String().Raw()
			stored, err := sharing.FindSharing(inst, id)
			require.NoError(t, err)
			assert.Nil(t, stored.Reference)
		})
	}
}
