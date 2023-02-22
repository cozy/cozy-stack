package sharings_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/revision"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/gavv/httpexpect/v2"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplicator(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	// Things for the replicator tests
	var tsR *httptest.Server
	var replInstance *instance.Instance
	var replSharingID, replAccessToken string
	var fileSharingID, fileAccessToken string
	var dirID string
	var xorKey []byte

	const replDoctype = "io.cozy.replicator.tests"

	config.UseTestFile()
	build.BuildMode = build.ModeDev
	config.GetConfig().Assets = "../../assets"
	_ = web.LoadSupportedLocales()
	testutils.NeedCouchdb(t)
	render, _ := statik.NewDirRenderer("../../assets")
	middlewares.BuildTemplates()

	// Prepare Alice's instance
	setup := testutils.NewSetup(t, t.Name()+"_alice")
	aliceInstance = setup.GetTestInstance(&lifecycle.Options{
		Email:      "alice@example.net",
		PublicName: "Alice",
	})
	aliceAppToken = generateAppToken(aliceInstance, "testapp", iocozytests)
	aliceAppTokenWildcard = generateAppToken(aliceInstance, "testapp2", iocozytestswildcard)
	charlieContact = createContact(t, aliceInstance, "Charlie", "charlie@example.net")
	daveContact = createContact(t, aliceInstance, "Dave", "dave@example.net")
	tsA = setup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/sharings":    sharings.Routes,
		"/permissions": permissions.Routes,
	})
	tsA.Config.Handler.(*echo.Echo).Renderer = render
	tsA.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	// Prepare Bob's browser
	jar := setup.GetCookieJar()
	bobUA = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	// Prepare another instance for the replicator tests
	replSetup := testutils.NewSetup(t, t.Name()+"_replicator")
	replInstance = replSetup.GetTestInstance()
	tsR = replSetup.GetTestServerMultipleRoutes(map[string]func(*echo.Group){
		"/sharings": sharings.Routes,
	})

	require.NoError(t, dynamic.InitDynamicAssetFS(), "Could not init dynamic FS")
	t.Run("CreateSharingForReplicatorTest", func(t *testing.T) {
		rule := sharing.Rule{
			Title:    "tests",
			DocType:  replDoctype,
			Selector: "foo",
			Values:   []string{"bar", "baz"},
			Add:      "sync",
			Update:   "sync",
			Remove:   "sync",
		}
		s := sharing.Sharing{
			Description: "replicator tests",
			Rules:       []sharing.Rule{rule},
		}
		assert.NoError(t, s.BeOwner(replInstance, ""))
		s.Members = append(s.Members, sharing.Member{
			Status:   sharing.MemberStatusReady,
			Name:     "J. Doe",
			Email:    "j.doe@example.net",
			Instance: "https://j.example.net/",
		})
		s.Credentials = append(s.Credentials, sharing.Credentials{})
		_, err := s.Create(replInstance)
		assert.NoError(t, err)
		replSharingID = s.SID

		cli, err := sharing.CreateOAuthClient(replInstance, &s.Members[1])
		assert.NoError(t, err)
		s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
		token, err := sharing.CreateAccessToken(replInstance, cli, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials[0].AccessToken = token
		assert.NoError(t, couchdb.UpdateDoc(replInstance, &s))
		replAccessToken = token.AccessToken
		assert.NoError(t, couchdb.CreateDB(replInstance, replDoctype))
	})

	t.Run("Permissions", func(t *testing.T) {
		assert.NotNil(t, replSharingID)
		assert.NotNil(t, replAccessToken)

		id := replDoctype + "/" + uuidv4()
		createShared(t, id, []string{"111111111"}, replInstance, replSharingID)

		t.Run("WithoutBearerToken", func(t *testing.T) {
			e := httpexpect.Default(t, tsR.URL)

			e.POST("/sharings/"+replSharingID+"/_revs_diff").
				WithHeader("Content-Type", "application/json").
				WithHeader("Accept", "application/json").
				WithBytes([]byte(`{"id": ["1-111111111"]}`)).
				Expect().Status(401)
		})

		t.Run("OK", func(t *testing.T) {
			e := httpexpect.Default(t, tsR.URL)

			e.POST("/sharings/"+replSharingID+"/_revs_diff").
				WithHeader("Content-Type", "application/json").
				WithHeader("Authorization", "Bearer "+replAccessToken).
				WithHeader("Accept", "application/json").
				WithBytes([]byte(`{"id": ["1-111111111"]}`)).
				Expect().Status(200)
		})
	})

	t.Run("RevsDiff", func(t *testing.T) {
		assert.NotEmpty(t, replSharingID)
		assert.NotEmpty(t, replAccessToken)

		sid1 := replDoctype + "/" + uuidv4()
		createShared(t, sid1, []string{"1a", "1a", "1a"}, replInstance, replSharingID)
		sid2 := replDoctype + "/" + uuidv4()
		createShared(t, sid2, []string{"2a", "2a", "2a"}, replInstance, replSharingID)
		sid3 := replDoctype + "/" + uuidv4()
		createShared(t, sid3, []string{"3a", "3a", "3a"}, replInstance, replSharingID)
		sid4 := replDoctype + "/" + uuidv4()
		createShared(t, sid4, []string{"4a", "4a", "4a"}, replInstance, replSharingID)
		sid5 := replDoctype + "/" + uuidv4()
		createShared(t, sid5, []string{"5a", "5a", "5a"}, replInstance, replSharingID)
		sid6 := replDoctype + "/" + uuidv4()

		e := httpexpect.Default(t, tsR.URL)

		obj := e.POST("/sharings/"+replSharingID+"/_revs_diff").
			WithHeader("Authorization", "Bearer "+replAccessToken).
			WithHeader("Accept", "application/json").
			WithJSON(sharing.Changed{
				sid1: []string{"3-1a"},
				sid2: []string{"2-2a"},
				sid3: []string{"5-3b"},
				sid4: []string{"2-4b", "2-4c", "4-4d"},
				sid6: []string{"1-6b"},
			}).
			Expect().Status(200).
			JSON().Object()

		// sid1 is the same on both sides
		obj.NotContainsKey(sid1)

		// sid2 was updated on the target
		obj.NotContainsKey(sid2)

		// sid3 was updated on the source
		obj.Value(sid3).Object().Value("missing").Array().Equal([]string{"5-3b"})

		// sid4 is a conflict
		obj.Value(sid4).Object().Value("missing").Array().Equal([]string{"2-4b", "2-4c", "4-4d"})

		// sid5 has been created on the target
		obj.NotContainsKey(sid5)

		// sid6 has been created on the source
		obj.Value(sid6).Object().Value("missing").Array().Equal([]string{"1-6b"})
	})

	t.Run("BulkDocs", func(t *testing.T) {
		assert.NotEmpty(t, replSharingID)
		assert.NotEmpty(t, replAccessToken)

		id1 := uuidv4()
		sid1 := replDoctype + "/" + id1
		createShared(t, sid1, []string{"aaa", "bbb"}, replInstance, replSharingID)
		id2 := uuidv4()
		sid2 := replDoctype + "/" + id2

		e := httpexpect.Default(t, tsR.URL)

		e.POST("/sharings/"+replSharingID+"/_bulk_docs").
			WithHeader("Authorization", "Bearer "+replAccessToken).
			WithHeader("Accept", "application/json").
			WithJSON(sharing.DocsByDoctype{
				replDoctype: {
					{
						"_id":  id1,
						"_rev": "3-ccc",
						"_revisions": map[string]interface{}{
							"start": 3,
							"ids":   []string{"ccc", "bbb"},
						},
						"this": "is document " + id1 + " at revision 3-ccc",
						"foo":  "bar",
					},
					{
						"_id":  id2,
						"_rev": "3-fff",
						"_revisions": map[string]interface{}{
							"start": 3,
							"ids":   []string{"fff", "eee", "dd"},
						},
						"this": "is document " + id2 + " at revision 3-fff",
						"foo":  "baz",
					},
				},
			}).
			Expect().Status(200)

		assertSharedDoc(t, sid1, "3-ccc", replInstance)
		assertSharedDoc(t, sid2, "3-fff", replInstance)
	})

	t.Run("CreateSharingForUploadFileTest", func(t *testing.T) {
		dirID = uuidv4()
		ruleOne := sharing.Rule{
			Title:    "file one",
			DocType:  "io.cozy.files",
			Selector: "",
			Values:   []string{dirID},
			Add:      "sync",
			Update:   "sync",
			Remove:   "sync",
		}
		s := sharing.Sharing{
			Description: "upload files tests",
			Rules:       []sharing.Rule{ruleOne},
		}
		assert.NoError(t, s.BeOwner(replInstance, ""))

		s.Members = append(s.Members, sharing.Member{
			Status:   sharing.MemberStatusReady,
			Name:     "J. Doe",
			Email:    "j.doe@example.net",
			Instance: "https://j.example.net/",
		})

		s.Credentials = append(s.Credentials, sharing.Credentials{})
		_, err := s.Create(replInstance)
		assert.NoError(t, err)
		fileSharingID = s.SID

		xorKey = sharing.MakeXorKey()
		s.Credentials[0].XorKey = xorKey

		cli, err := sharing.CreateOAuthClient(aliceInstance, &s.Members[0])
		assert.NoError(t, err)
		s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)

		token, err := sharing.CreateAccessToken(aliceInstance, cli, s.SID, permission.ALL)
		assert.NoError(t, err)
		s.Credentials[0].AccessToken = token

		cli2, err := sharing.CreateOAuthClient(replInstance, &s.Members[1])
		assert.NoError(t, err)
		s.Credentials[0].InboundClientID = cli2.ClientID

		token2, err := sharing.CreateAccessToken(replInstance, cli2, s.SID, permission.ALL)
		assert.NoError(t, err)
		fileAccessToken = token2.AccessToken
		assert.NoError(t, couchdb.UpdateDoc(replInstance, &s))
	})

	t.Run("UploadNewFile", func(t *testing.T) {
		e := httpexpect.Default(t, tsR.URL)

		assert.NotEmpty(t, fileSharingID)
		assert.NotEmpty(t, fileAccessToken)

		fileOneID := uuidv4()

		obj := e.PUT("/sharings/"+fileSharingID+"/io.cozy.files/"+fileOneID+"/metadata").
			WithHeader("Authorization", "Bearer "+fileAccessToken).
			WithHeader("Accept", "application/json").
			WithJSON(map[string]interface{}{
				"_id":  fileOneID,
				"_rev": "1-5f9ba207fefdc250e35f7cd866c84cc6",
				"_revisions": map[string]interface{}{
					"start": 1,
					"ids":   []string{"5f9ba207fefdc250e35f7cd866c84cc6"},
				},
				"type":       "file",
				"name":       "hello.txt",
				"created_at": "2018-04-23T18:11:42.343937292+02:00",
				"updated_at": "2018-04-23T18:11:42.343937292+02:00",
				"size":       "6",
				"md5sum":     "WReFt5RgHiErJg4lklY2/Q==",
				"mime":       "text/plain",
				"class":      "text",
				"executable": false,
				"trashed":    false,
				"tags":       []string{},
			}).
			Expect().Status(200).
			JSON().Object()

		key := obj.Value("key").String().NotEmpty().Raw()

		e.PUT("/sharings/"+fileSharingID+"/io.cozy.files/"+key).
			WithHeader("Authorization", "Bearer "+fileAccessToken).
			WithText("world\n"). // Must match the md5sum in the body just above
			Expect().Status(204)
	})

	t.Run("GetFolder", func(t *testing.T) {
		e := httpexpect.Default(t, tsR.URL)

		assert.NotEmpty(t, fileSharingID)
		assert.NotEmpty(t, fileAccessToken)

		fs := replInstance.VFS()
		folder, err := vfs.NewDirDoc(fs, "zorglub", dirID, nil)
		assert.NoError(t, err)
		assert.NoError(t, fs.CreateDir(folder))
		msg := sharing.TrackMessage{
			SharingID: fileSharingID,
			RuleIndex: 0,
			DocType:   consts.Files,
		}
		evt := sharing.TrackEvent{
			Verb: "CREATED",
			Doc: couchdb.JSONDoc{
				Type: consts.Files,
				M: map[string]interface{}{
					"type":   folder.Type,
					"_id":    folder.DocID,
					"_rev":   folder.DocRev,
					"name":   folder.DocName,
					"path":   folder.Fullpath,
					"dir_id": dirID,
				},
			},
		}
		assert.NoError(t, sharing.UpdateShared(replInstance, msg, evt))

		xoredID := sharing.XorID(folder.DocID, xorKey)

		obj := e.GET("/sharings/"+fileSharingID+"/io.cozy.files/"+xoredID).
			WithHeader("Authorization", "Bearer "+fileAccessToken).
			WithHeader("Accept", "application/json").
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("_id", xoredID)
		obj.ValueEqual("_rev", folder.DocRev)
		obj.ValueEqual("type", "directory")
		obj.ValueEqual("name", "zorglub")
		obj.NotContainsKey("dir_id")
		obj.Value("created_at").String().DateTime(time.RFC3339)
		obj.Value("updated_at").String().DateTime(time.RFC3339)
	})
}

func uuidv4() string {
	id, _ := uuid.NewV4()
	return id.String()
}

func createShared(t *testing.T, sid string, revisions []string, replInstance *instance.Instance, replSharingID string) *sharing.SharedRef {
	rev := fmt.Sprintf("%d-%s", len(revisions), revisions[0])
	parts := strings.SplitN(sid, "/", 2)
	doctype := parts[0]
	id := parts[1]
	start := revision.Generation(rev)
	docs := []map[string]interface{}{
		{
			"_id":  id,
			"_rev": rev,
			"_revisions": map[string]interface{}{
				"start": start,
				"ids":   revisions,
			},
			"this": "is document " + id + " at revision " + rev,
		},
	}
	err := couchdb.BulkForceUpdateDocs(replInstance, doctype, docs)
	assert.NoError(t, err)
	var tree *sharing.RevsTree
	for i, r := range revisions {
		old := tree
		tree = &sharing.RevsTree{
			Rev: fmt.Sprintf("%d-%s", start-i, r),
		}
		if old != nil {
			tree.Branches = []sharing.RevsTree{*old}
		}
	}
	ref := sharing.SharedRef{
		SID:       sid,
		Revisions: tree,
		Infos: map[string]sharing.SharedInfo{
			replSharingID: {Rule: 0},
		},
	}
	err = couchdb.CreateNamedDocWithDB(replInstance, &ref)
	assert.NoError(t, err)
	return &ref
}

func assertSharedDoc(t *testing.T, sid, rev string, replInstance *instance.Instance) {
	parts := strings.SplitN(sid, "/", 2)
	doctype := parts[0]
	id := parts[1]
	var doc couchdb.JSONDoc
	assert.NoError(t, couchdb.GetDoc(replInstance, doctype, id, &doc))
	assert.Equal(t, doc.ID(), id)
	assert.Equal(t, doc.Rev(), rev)
	assert.Equal(t, doc.M["this"], "is document "+id+" at revision "+rev)
}
