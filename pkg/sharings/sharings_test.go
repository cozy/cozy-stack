package sharings

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	webAuth "github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")
var domainSharer = "test-sharing.sparta"
var domainRecipient = "test-sharing.xerxes"

var testInstance *instance.Instance
var ts *httptest.Server
var setup *testutils.TestSetup

var in *instance.Instance
var recipientIn *instance.Instance
var recipientURL string
var testDocType = "io.cozy.tests"

func routes(router *echo.Group) {
	router.Any("/doc/:doctype/:id", func(c echo.Context) error {
		return c.JSON(http.StatusOK, nil)
	})
}

func createServer() *httptest.Server {
	createSettings(recipientIn)
	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	handler.Use(injectInstance(recipientIn))
	webAuth.Routes(handler.Group("/auth"))
	data.Routes(handler.Group("/data"))
	files.Routes(handler.Group("/files"))
	routes(handler.Group("/sharings"))
	ts = httptest.NewServer(handler)

	return ts
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}

func createInstance(domain, publicName string) (*instance.Instance, error) {
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	settings.M["public_name"] = publicName
	opts := &instance.Options{
		Domain:   domain,
		Settings: settings,
	}
	return instance.Create(opts)
}

func createSettings(instance *instance.Instance) {
	settingsDoc := &couchdb.JSONDoc{
		Type: consts.Settings,
		M:    make(map[string]interface{}),
	}
	settingsDoc.SetID(consts.InstanceSettingsID)
	err := couchdb.CreateNamedDocWithDB(instance, settingsDoc)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func createDoc(t *testing.T, ins *instance.Instance, doctype string, m map[string]interface{}) *couchdb.JSONDoc {
	doc := &couchdb.JSONDoc{
		Type: doctype,
		M:    m,
	}

	err := couchdb.CreateDoc(ins, doc)
	assert.NoError(t, err)
	return doc
}

func createOAuthClient(t *testing.T) *oauth.Client {
	client := &oauth.Client{
		RedirectURIs: []string{utils.RandomString(10)},
		ClientName:   utils.RandomString(10),
		SoftwareID:   utils.RandomString(10),
	}
	crErr := client.Create(testInstance)
	assert.Nil(t, crErr)

	return client
}

func insertSharingIntoDB(t *testing.T, sharingID, sharingType string, owner bool, slug string, recipients []*Recipient, rule permissions.Rule) *Sharing {
	sharing := &Sharing{
		SharingType:      sharingType,
		Owner:            owner,
		Permissions:      permissions.Set{rule},
		RecipientsStatus: []*RecipientStatus{},
	}

	if slug == "" {
		sharing.AppSlug = utils.RandomString(15)
	} else {
		sharing.AppSlug = slug
	}

	if sharingID == "" {
		sharing.SharingID = utils.RandomString(32)
	} else {
		sharing.SharingID = sharingID
	}

	scope, err := rule.MarshalScopeString()
	assert.NoError(t, err)

	for _, recipient := range recipients {
		if recipient.ID() == "" {
			recipient.URL = strings.TrimPrefix(recipient.URL, "http://")
			err = CreateRecipient(testInstance, recipient)
			assert.NoError(t, err)
		}

		client := createOAuthClient(t)
		client.CouchID = client.ClientID
		accessToken, errc := client.CreateJWT(testInstance,
			permissions.AccessTokenAudience, scope)
		assert.NoError(t, errc)

		rs := &RecipientStatus{
			Status: consts.SharingStatusAccepted,
			RefRecipient: couchdb.DocReference{
				ID:   recipient.ID(),
				Type: recipient.DocType(),
			},
			Client: auth.Client{
				ClientID: client.ClientID,
			},
			AccessToken: auth.AccessToken{
				AccessToken: accessToken,
				Scope:       scope,
			},
		}

		if sharingType == consts.MasterMasterSharing {
			hostClient := createOAuthClient(t)
			rs.HostClientID = hostClient.ClientID
		}

		if owner {
			sharing.RecipientsStatus = append(sharing.RecipientsStatus, rs)
		} else {
			sharing.Sharer = Sharer{
				SharerStatus: rs,
				URL:          recipient.URL,
			}
			break
		}
	}

	err = couchdb.CreateDoc(testInstance, sharing)
	assert.NoError(t, err)

	return sharing
}

func insertClientDocumentInDB(db couchdb.Database, clientID, url string) error {
	client := couchdb.JSONDoc{
		Type: consts.OAuthClients,
		M: map[string]interface{}{
			"_id":        clientID,
			"client_uri": url,
		},
	}

	return couchdb.CreateNamedDocWithDB(db, client)
}

func createFile(t *testing.T, fs vfs.VFS, name, content string, refs []couchdb.DocReference) *vfs.FileDoc {
	doc, err := vfs.NewFileDoc(name, "", -1, nil, "foo/bar", "foo", time.Now(),
		false, false, []string{"this", "is", "spartest"})
	assert.NoError(t, err)
	doc.ReferencedBy = refs

	body := bytes.NewReader([]byte(content))

	file, err := fs.CreateFile(doc, nil)
	assert.NoError(t, err)

	n, err := io.Copy(file, body)
	assert.NoError(t, err)
	assert.Equal(t, len(content), int(n))

	err = file.Close()
	assert.NoError(t, err)

	return doc
}

func createDir(t *testing.T, fs vfs.VFS, name string, refs []couchdb.DocReference) *vfs.DirDoc {
	dirDoc, err := vfs.NewDirDoc(fs, name, "", []string{"It's", "me", "again"})
	assert.NoError(t, err)
	dirDoc.CreatedAt = time.Now()
	dirDoc.UpdatedAt = time.Now()
	err = fs.CreateDir(dirDoc)
	assert.NoError(t, err)

	return dirDoc
}

func addPublicName(t *testing.T, instance *instance.Instance) {
	publicName := "El Shareto"
	doc, err := instance.SettingsDocument()
	assert.NoError(t, err)

	doc.M["public_name"] = publicName
	doc.SetID(doc.M["_id"].(string))
	doc.SetRev(doc.M["_rev"].(string))
	doc.Type = consts.Settings

	err = couchdb.UpdateDoc(in, doc)
	assert.NoError(t, err)
}

func TestGetAccessTokenNoAuth(t *testing.T) {
	code := "sesame"
	rs := &RecipientStatus{
		recipient: &Recipient{URL: recipientURL},
		Client:    auth.Client{},
	}
	_, err := rs.getAccessToken(in, code)
	assert.Error(t, err)
}

func TestGetAccessTokenNoURL(t *testing.T) {
	code := "dummy"
	rs := &RecipientStatus{
		recipient: &Recipient{},
		Client:    auth.Client{},
	}

	_, err := rs.getAccessToken(in, code)
	assert.Equal(t, ErrRecipientHasNoURL, err)
}

func TestRegisterNoURL(t *testing.T) {
	rs := &RecipientStatus{
		recipient: &Recipient{
			RID: "dummyid",
		},
	}
	err := rs.Register(in)
	assert.Error(t, err)
	assert.Equal(t, ErrRecipientHasNoURL, err)
}

func TestRegisterSuccess(t *testing.T) {
	// In Go 1.8 url.Parse returns the following error if we try to parse an
	// url that looks like "127.0.0.1:46473": "first path segment in URL cannot
	// contain colon".
	// Adding a scheme fixes this.
	rURL := recipientURL
	if !strings.HasPrefix(rURL, "http://") {
		rURL = "http://" + rURL
	}

	rs := &RecipientStatus{
		recipient: &Recipient{
			URL:   rURL,
			Email: "xerxes@fr",
			RID:   "dummyid",
		},
	}

	addPublicName(t, in)

	err := rs.Register(in)
	assert.NoError(t, err)
	assert.NotNil(t, rs.Client)
}

func TestGetRecipient(t *testing.T) {
	recipient := &Recipient{}

	_, err := GetRecipient(TestPrefix, "maurice")
	assert.Error(t, err)
	assert.Equal(t, ErrRecipientDoesNotExist, err)

	err = couchdb.CreateDoc(TestPrefix, recipient)
	assert.NoError(t, err)

	doc, err := GetRecipient(TestPrefix, recipient.RID)
	assert.NoError(t, err)
	assert.Equal(t, recipient, doc)
}

func TestCheckSharingTypeBadType(t *testing.T) {
	sharingType := "mybad"
	err := CheckSharingType(sharingType)
	assert.Error(t, err)
}

func TestCheckSharingTypeSuccess(t *testing.T) {
	sharingType := consts.OneShotSharing
	err := CheckSharingType(sharingType)
	assert.NoError(t, err)
}

func TestGetSharingRecipientFromClientIDNoRecipient(t *testing.T) {
	sharing := &Sharing{}
	_, err := sharing.GetSharingRecipientFromClientID(TestPrefix, "")
	assert.Equal(t, ErrRecipientDoesNotExist, err)
}

func TestGetSharingRecipientFromClientIDNoClient(t *testing.T) {
	clientID := "fake client"

	rStatus := &RecipientStatus{
		RefRecipient: couchdb.DocReference{ID: "id", Type: "type"},
		Client: auth.Client{
			ClientID: "fakeid",
		},
	}
	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rStatus},
	}
	recStatus, err := sharing.GetSharingRecipientFromClientID(TestPrefix,
		clientID)

	assert.Equal(t, ErrRecipientDoesNotExist, err)
	assert.Nil(t, recStatus)
}

func TestGetRecipientStatusFromRecipientNoRecipient(t *testing.T) {
	rs := &RecipientStatus{}
	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rs},
	}
	recStatus, err := sharing.GetRecipientStatusFromRecipientID(TestPrefix,
		"bad recipient")
	assert.Error(t, err)
	assert.Nil(t, recStatus)
}

func TestGetRecipientStatusFromRecipientNotFound(t *testing.T) {
	recID := "fake recipient"
	rec := &Recipient{}
	rec.SetID(recID)
	rs := &RecipientStatus{
		recipient: rec,
	}

	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rs},
	}

	recStatus, err := sharing.GetRecipientStatusFromRecipientID(TestPrefix,
		"bad recipient")
	assert.Equal(t, ErrRecipientDoesNotExist, err)
	assert.Nil(t, recStatus)
}

func TestGetSharingRecipientFromClientIDSuccess(t *testing.T) {
	clientID := "fake client"
	rs := &RecipientStatus{
		Client: auth.Client{
			ClientID: clientID,
		},
	}

	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rs},
	}

	recStatus, err := sharing.GetSharingRecipientFromClientID(TestPrefix,
		clientID)
	assert.NoError(t, err)
	assert.Equal(t, rs, recStatus)
}

func TestGetRecipientStatusFromRecipientIDSuccess(t *testing.T) {
	recID := "fake recipient"
	rec := &Recipient{}
	rec.SetID(recID)
	rs := &RecipientStatus{
		recipient: rec,
	}

	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rs},
	}

	recStatus, err := sharing.GetRecipientStatusFromRecipientID(TestPrefix,
		recID)
	assert.NoError(t, err)
	assert.Equal(t, rs, recStatus)
}

func TestSharingAcceptedNoSharing(t *testing.T) {
	state := "fake state"
	clientID := "fake client"
	accessCode := "fake code"
	_, err := SharingAccepted(in, state, clientID, accessCode)
	assert.Error(t, err)
}

func TestSharingAcceptedNoClient(t *testing.T) {
	state := "stateoftheart"
	clientID := "fake client"
	accessCode := "fake code"

	sharing := &Sharing{
		SharingID: state,
	}
	err := couchdb.CreateDoc(in, sharing)
	assert.NoError(t, err)
	_, err = SharingAccepted(in, state, clientID, accessCode)
	assert.Error(t, err)
}

func TestSharingAcceptedStateNotUnique(t *testing.T) {
	state := "stateoftheart"
	clientID := "fake client"
	accessCode := "fake code"
	sharing1 := &Sharing{
		SharingID: state,
	}
	sharing2 := &Sharing{
		SharingID: state,
	}
	err := couchdb.CreateDoc(in, sharing1)
	assert.NoError(t, err)
	err = couchdb.CreateDoc(in, sharing2)
	assert.NoError(t, err)

	_, err = SharingAccepted(in, state, clientID, accessCode)
	assert.Error(t, err)
}

func TestSharingRefusedNoSharing(t *testing.T) {
	state := "fake state"
	clientID := "fake client"
	_, err := SharingRefused(TestPrefix, state, clientID)
	assert.Error(t, err)
}

func TestSharingRefusedNoClient(t *testing.T) {
	state := "stateoftheart"
	clientID := "fake client"

	sharing := &Sharing{
		SharingID: state,
	}
	err := couchdb.CreateDoc(TestPrefix, sharing)
	assert.NoError(t, err)
	_, err = SharingRefused(TestPrefix, state, clientID)
	assert.Error(t, err)
}

func TestSharingRefusedStateNotUnique(t *testing.T) {
	state := "stateoftheart"
	clientID := "fake client"
	sharing1 := &Sharing{
		SharingID: state,
	}
	sharing2 := &Sharing{
		SharingID: state,
	}
	err := couchdb.CreateDoc(TestPrefix, sharing1)
	assert.NoError(t, err)
	err = couchdb.CreateDoc(TestPrefix, sharing2)
	assert.NoError(t, err)

	_, err = SharingRefused(TestPrefix, state, clientID)
	assert.Error(t, err)
}

func TestSharingRefusedSuccess(t *testing.T) {
	state := "stateoftheart2"
	clientID := "thriftshopclient"

	recipient := &Recipient{
		URL:   "https://toto.fr",
		Email: "cpasbien@mail.fr",
	}
	err := couchdb.CreateDoc(TestPrefix, recipient)
	assert.NoError(t, err)

	rStatus := &RecipientStatus{
		RefRecipient: couchdb.DocReference{ID: recipient.RID},
		Client: auth.Client{
			ClientID: clientID,
		},
	}
	sharing := &Sharing{
		SharingID:        state,
		RecipientsStatus: []*RecipientStatus{rStatus},
	}
	err = couchdb.CreateDoc(TestPrefix, sharing)
	assert.NoError(t, err)

	redirect, err := SharingRefused(TestPrefix, state, clientID)
	assert.NoError(t, err)
	assert.Equal(t, recipient.URL, redirect)
}

func TestRecipientRefusedSharingWhenSharingDoesNotExist(t *testing.T) {
	_, err := RecipientRefusedSharing(TestPrefix, "fakesharingid")
	assert.Error(t, err)
	assert.Equal(t, ErrSharingDoesNotExist, err)
}

func TestRecipientRefusedSharingSuccess(t *testing.T) {
	rule := permissions.Rule{
		Selector: "",
		Type:     "io.cozy.events",
		Values:   []string{"123"},
		Verbs:    permissions.ALL,
	}

	sharing := insertSharingIntoDB(t, "", consts.MasterMasterSharing, false,
		"io.cozy.events", []*Recipient{}, rule)

	_, err := RecipientRefusedSharing(testInstance, sharing.SharingID)
	assert.NoError(t, err)

	// We also test that the sharing document is actually deleted.
	doc := couchdb.JSONDoc{}
	err = couchdb.GetDoc(TestPrefix, consts.Sharings, sharing.ID(), &doc)
	assert.Error(t, err)
}

func TestCreateSharingRequestBadParams(t *testing.T) {
	_, err := CreateSharingRequest(TestPrefix, "", "", "", "", "", "")
	assert.Error(t, err)

	state := "1234"
	_, err = CreateSharingRequest(TestPrefix, "", state, "", "", "", "")
	assert.Error(t, err)

	sharingType := consts.OneShotSharing
	_, err = CreateSharingRequest(TestPrefix, "", state, sharingType, "", "", "")
	assert.Error(t, err)

	scope := "io.cozy.sharings"
	_, err = CreateSharingRequest(TestPrefix, "", state, sharingType, scope, "", "")
	assert.Error(t, err)
	assert.Equal(t, ErrNoOAuthClient, err)
}

func TestCreateSharingRequestSuccess(t *testing.T) {
	state := "1234"
	sharingType := consts.OneShotSharing
	desc := "share cher"

	rule := permissions.Rule{
		Type:   "io.cozy.events",
		Verbs:  permissions.Verbs(permissions.POST),
		Values: []string{"1234"},
	}

	// TODO insert a client document in the database.
	clientID := "clientIDTestCreateSharingRequestSuccess"
	err := insertClientDocumentInDB(TestPrefix, clientID, "https://cozy.rocks")
	if err != nil {
		t.FailNow()
	}

	set := permissions.Set{rule}
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	sharing, err := CreateSharingRequest(TestPrefix, desc, state, sharingType, scope, clientID, "randomappslug")
	assert.NoError(t, err)
	assert.Equal(t, state, sharing.SharingID)
	assert.Equal(t, sharingType, sharing.SharingType)
	assert.Equal(t, set, sharing.Permissions)
}

func TestCreateSharingAndRegisterSharer(t *testing.T) {
	rec := &Recipient{
		Email: "test@test.fr",
	}

	recStatus := &RecipientStatus{
		RefRecipient: couchdb.DocReference{
			ID:   "123",
			Type: consts.Contacts,
		},
		recipient: rec,
	}

	sharing := &Sharing{
		SharingType:      "shotmedown",
		RecipientsStatus: []*RecipientStatus{recStatus},
	}

	// `SharingType` is wrong.
	err := CreateSharing(in, sharing)
	assert.Equal(t, ErrBadSharingType, err)

	// `SharingType` is correct.
	sharing.SharingType = consts.OneShotSharing
	// However the recipient is not persisted in the database.
	err = CreateSharing(in, sharing)
	assert.Equal(t, ErrRecipientDoesNotExist, err)

	// The CreateSharingAndRegisterSharer scenario that succeeds is already
	// tested in `createSharing`.
}

func TestRemoveDocumentIfNotShared(t *testing.T) {
	// First set of tests: JSON documents.
	// Test: the document matches a permission in a sharing so it should NOT be
	// removed.
	docEvent := createDoc(t, testInstance, "io.cozy.events",
		map[string]interface{}{})
	rule1 := permissions.Rule{
		Selector: "_id",
		Type:     "io.cozy.events",
		Verbs:    permissions.ALL,
		Values:   []string{docEvent.ID()},
	}
	_ = insertSharingIntoDB(t, "", consts.MasterSlaveSharing, false,
		"io.cozy.events", []*Recipient{}, rule1)

	err := RemoveDocumentIfNotShared(testInstance, "io.cozy.events",
		docEvent.ID())
	assert.NoError(t, err)
	doc := couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, "io.cozy.events", docEvent.ID(), &doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, doc)

	// Test: the document doesn't match a permission in a sharing so it should
	// be removed.
	docToDelete := createDoc(t, testInstance, "io.cozy.events",
		map[string]interface{}{})

	err = RemoveDocumentIfNotShared(testInstance, "io.cozy.events",
		docToDelete.ID())
	assert.NoError(t, err)
	doc = couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, "io.cozy.events", docToDelete.ID(), &doc)
	assert.Error(t, err)
	assert.Empty(t, doc.M)

	// Second set of tests: files.
	rule2 := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     consts.Files,
		Verbs:    permissions.ALL,
		Values:   []string{"io.cozy.photos.albums/456"},
	}
	_ = insertSharingIntoDB(t, "", consts.MasterSlaveSharing, false,
		"io.cozy.events", []*Recipient{}, rule2)

	// Test: the file matches a permission in a sharing it should NOT be
	// removed.
	fileAlbum := createFile(t, testInstance.VFS(), "testRemoveIfNotSharedAlbum",
		"testRemoveIfNotSharedAlbumContent", []couchdb.DocReference{
			couchdb.DocReference{Type: "io.cozy.photos.albums", ID: "456"},
		})

	err = RemoveDocumentIfNotShared(testInstance, consts.Files, fileAlbum.ID())
	assert.NoError(t, err)
	fileDoc, err := testInstance.VFS().FileByID(fileAlbum.ID())
	assert.NoError(t, err)
	assert.False(t, fileDoc.Trashed)

	// Test: the file doesn't match a permission in any sharing it should be
	// removed.
	fileToDelete := createFile(t, testInstance.VFS(),
		"testRemoveIfNotSharedToDelete", "testRemoveIfNotSharedToDeleteContent",
		[]couchdb.DocReference{})

	err = RemoveDocumentIfNotShared(testInstance, consts.Files,
		fileToDelete.ID())
	assert.NoError(t, err)
	fileDoc, err = testInstance.VFS().FileByID(fileToDelete.ID())
	assert.NoError(t, err)
	assert.True(t, fileDoc.Trashed)
}

func TestRevokeSharing(t *testing.T) {
	sharingIDSharerMM := utils.RandomString(20)
	sharingIDRecipientMM := utils.RandomString(20)
	counterRevokeSharing, counterRevokeRecipient := 0, 0
	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.DELETE("/:id", func(c echo.Context) error {
				counterRevokeSharing = counterRevokeSharing + 1
				assert.Equal(t, sharingIDSharerMM, c.Param("id"))
				assert.Equal(t, "false",
					c.QueryParam(consts.QueryParamRecursive))
				return c.NoContent(http.StatusOK)
			})
			router.DELETE("/:id/recipient/:recipient-id",
				func(c echo.Context) error {
					counterRevokeRecipient = counterRevokeRecipient + 1
					assert.Equal(t, sharingIDRecipientMM, c.Param("id"))
					assert.Equal(t, "false",
						c.QueryParam(consts.QueryParamRecursive))
					return c.NoContent(http.StatusOK)
				},
			)
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)

	u := "http://" + ts.URL
	recipient1 := &Recipient{URL: u, Email: "recipient1@mail.cc"}
	recipient2 := &Recipient{URL: u, Email: "recipient2@mail.cc"}
	rule := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     "io.cozy.events",
		Values:   []string{"io.cozy.calendar/123"},
		Verbs:    permissions.ALL,
	}
	// We create a sharing where the user is the owner.
	sharingSharerMM := insertSharingIntoDB(t, sharingIDSharerMM,
		consts.MasterMasterSharing, true, "",
		[]*Recipient{recipient1, recipient2}, rule)
	// We add a trigger to this sharing.
	sched := stack.GetScheduler()
	triggers, _ := sched.GetAll(testInstance.Domain)
	nbTriggers := len(triggers)
	err := AddTrigger(testInstance, rule, sharingSharerMM.SharingID, false)
	assert.NoError(t, err)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers+1)

	// Test: we revoke a sharing being the owner.
	err = RevokeSharing(testInstance, sharingSharerMM, true)
	assert.NoError(t, err)
	// Check: all the recipients were asked to remove the sharing.
	assert.Equal(t, len(sharingSharerMM.RecipientsStatus), counterRevokeSharing)
	// Check: the trigger is deleted.
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers)
	// Check: the OAuth clients for all the recipients were deleted. Both calls
	// to `FindClient` should return errors.
	_, err = oauth.FindClient(testInstance,
		sharingSharerMM.RecipientsStatus[0].HostClientID)
	assert.Error(t, err)
	_, err = oauth.FindClient(testInstance,
		sharingSharerMM.RecipientsStatus[1].HostClientID)
	assert.Error(t, err)
	// Check: the sharing is revoked.
	doc := &Sharing{}
	err = couchdb.GetDoc(testInstance, consts.Sharings, sharingSharerMM.ID(),
		doc)
	assert.NoError(t, err)
	assert.True(t, doc.Revoked)

	// We create a sharing where the user is the recipient.
	sharingRecipientMM := insertSharingIntoDB(t, sharingIDRecipientMM,
		consts.MasterMasterSharing, false, "", []*Recipient{recipient1}, rule)
	// We add a trigger to this sharing.
	err = AddTrigger(testInstance, rule, sharingRecipientMM.SharingID, false)
	assert.NoError(t, err)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers+1)

	// Test: we revoke a sharing where we are a recipient.
	err = RevokeSharing(testInstance, sharingRecipientMM, true)
	assert.NoError(t, err)
	// Check: the sharer was asked to revoke us as a recipient.
	assert.Equal(t, 1, counterRevokeRecipient)
	// Check: the trigger is deleted.
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers)
	// Check: the OAuth client for the sharer is deleted.
	_, err = oauth.FindClient(testInstance,
		sharingRecipientMM.Sharer.SharerStatus.HostClientID)
	assert.Error(t, err)
	// Check: the sharing is revoked.
	doc = &Sharing{}
	err = couchdb.GetDoc(testInstance, consts.Sharings, sharingRecipientMM.ID(),
		doc)
	assert.NoError(t, err)
	assert.True(t, doc.Revoked)

}

func TestRevokeRecipient(t *testing.T) {
	// Test: we try to revoke a recipient from a sharing while we are not the
	// owner of the sharing.
	sharingNotSharer := insertSharingIntoDB(t, "", consts.MasterMasterSharing,
		false, "", []*Recipient{}, permissions.Rule{})
	err := RevokeRecipient(testInstance, sharingNotSharer, "wontbeused", false)
	assert.Error(t, err)
	assert.Equal(t, err, ErrOnlySharerCanRevokeRecipient)

	// We create a sharing with 2 recipients.
	sharingID := utils.RandomString(20)
	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.DELETE("/:id", func(c echo.Context) error {
				assert.Equal(t, sharingID, c.Param("id"))
				assert.Equal(t, "false",
					c.QueryParam(consts.QueryParamRecursive))
				return c.NoContent(http.StatusOK)
			})
		},
	}
	if ts != nil {
		ts.Close()
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)

	u := "http://" + ts.URL
	recipient1 := &Recipient{URL: "recipient1.url.cc", Email: "recipient@1"}
	recipient2 := &Recipient{URL: u, Email: "recipient@2"}
	rule := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     "io.cozy.events",
		Values:   []string{"io.cozy.calendar/123"},
		Verbs:    permissions.ALL,
	}
	sharing := insertSharingIntoDB(t, sharingID, consts.MasterMasterSharing,
		true, "", []*Recipient{recipient1, recipient2}, rule)

	// We add a trigger to this sharing.
	sched := stack.GetScheduler()
	triggers, _ := sched.GetAll(testInstance.Domain)
	nbTriggers := len(triggers)
	err = AddTrigger(testInstance, rule, sharing.SharingID, false)
	assert.NoError(t, err)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers+1)

	// Test: we try to remove a recipient that does not belong to this sharing.
	err = RevokeRecipient(testInstance, sharing, "randomhostclientid", false)
	assert.Error(t, err)
	assert.Equal(t, ErrRecipientDoesNotExist, err)

	// Test: we remove the first recipient.
	err = RevokeRecipient(testInstance, sharing,
		sharing.RecipientsStatus[0].Client.ClientID, false)
	assert.NoError(t, err)
	// We check that the first recipient was revoked, that the sharing is not
	// revoked, and that the trigger still exists.
	doc := Sharing{}
	err = couchdb.GetDoc(testInstance, consts.Sharings, sharing.ID(), &doc)
	assert.NoError(t, err)
	assert.False(t, doc.Revoked)
	assert.Equal(t, consts.SharingStatusRevoked, doc.RecipientsStatus[0].Status)
	assert.Empty(t, doc.RecipientsStatus[0].HostClientID)
	assert.Empty(t, doc.RecipientsStatus[0].Client.ClientID)
	assert.Empty(t, doc.RecipientsStatus[0].AccessToken.AccessToken)
	_, err = oauth.FindClient(testInstance,
		doc.RecipientsStatus[0].HostClientID)
	assert.Error(t, err)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers+1)

	// Test: we remove the second recipient.
	// By setting `recursive` to true we ask the recipient to revoke the sharing
	// as well, hence the test server above.
	err = RevokeRecipient(testInstance, sharing,
		sharing.RecipientsStatus[1].Client.ClientID, true)
	assert.NoError(t, err)
	// We check that the second recipient was revoked, that the sharing is
	// revoked, and that the trigger was deleted.
	doc = Sharing{}
	err = couchdb.GetDoc(testInstance, consts.Sharings, sharing.ID(), &doc)
	assert.NoError(t, err)
	assert.True(t, doc.Revoked)
	assert.Equal(t, consts.SharingStatusRevoked, doc.RecipientsStatus[1].Status)
	assert.Empty(t, doc.RecipientsStatus[1].HostClientID)
	assert.Empty(t, doc.RecipientsStatus[1].Client.ClientID)
	assert.Empty(t, doc.RecipientsStatus[1].AccessToken.AccessToken)
	_, err = oauth.FindClient(testInstance,
		doc.RecipientsStatus[1].HostClientID)
	assert.Error(t, err)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers)
}

func TestDeleteOAuthClient(t *testing.T) {
	client := createOAuthClient(t)

	err := deleteOAuthClient(testInstance, &RecipientStatus{
		HostClientID: "fakeid",
	})
	assert.Error(t, err)
	err = deleteOAuthClient(testInstance, &RecipientStatus{
		HostClientID: client.ClientID,
	})
	assert.NoError(t, err)

	_, err = oauth.FindClient(testInstance, client.ClientID)
	assert.Error(t, err)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup = testutils.NewSetup(m, "pkg_sharings_test")
	testInstance = setup.GetTestInstance()

	// Change the default config to persist the vfs
	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}
	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempdir,
	}

	_, err = stack.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// The instance must be created in db in order to retrieve it from
	// the share_data worker
	instance.Destroy(domainSharer)
	in, err = createInstance(domainSharer, "Alice")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	instance.Destroy(domainRecipient)
	recipientIn, err = createInstance(domainRecipient, "Bob")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(TestPrefix, consts.Sharings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, consts.Settings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, consts.Sharings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, testDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(recipientIn, consts.Settings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(recipientIn, consts.Sharings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(recipientIn, testDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(TestPrefix, consts.OAuthClients)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(TestPrefix, consts.Contacts)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, consts.Triggers)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	createSettings(in)
	_, err = stack.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ts = createServer()
	recipientURL = strings.Split(ts.URL, "http://")[1]

	err = couchdb.DefineIndex(TestPrefix, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.DefineIndex(in, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()
	couchdb.DeleteDB(TestPrefix, consts.Sharings)
	couchdb.DeleteDB(TestPrefix, consts.Contacts)
	ts.Close()

	os.Exit(res)
}
