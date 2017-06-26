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
var in *instance.Instance
var recipientIn *instance.Instance
var ts *httptest.Server
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

func createRecipient(t *testing.T, email, url string) *Recipient {
	recipient := &Recipient{
		Email: email,
		URL:   url,
	}
	err := CreateRecipient(in, recipient)
	assert.NoError(t, err)
	return recipient
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

func insertSharingIntoDB(t *testing.T, sharingType string, owner bool, doctype string, recipients []*Recipient, rule permissions.Rule) Sharing {
	sharing := Sharing{
		SharingType:      sharingType,
		Owner:            owner,
		SharingID:        utils.RandomString(32),
		Permissions:      permissions.Set{rule},
		RecipientsStatus: []*RecipientStatus{},
	}

	for _, recipient := range recipients {
		if recipient.ID() == "" {
			recipient = createRecipient(t, recipient.Email, recipient.URL)
		}

		client := createOAuthClient(t)

		sharing.RecipientsStatus = append(sharing.RecipientsStatus,
			&RecipientStatus{
				Status: consts.SharingStatusAccepted,
				RefRecipient: couchdb.DocReference{
					ID:   recipient.ID(),
					Type: recipient.DocType(),
				},
				recipient:    recipient,
				HostClientID: client.ClientID,
			})
	}

	err := couchdb.CreateDoc(testInstance, &sharing)
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

func updateTestDoc(t *testing.T, doc *couchdb.JSONDoc, k, v string) {
	doc.M[k] = v
	err := couchdb.UpdateDoc(in, doc)
	assert.NoError(t, err)
}

func updateTestFile(t *testing.T, fileDoc *vfs.FileDoc, patch *vfs.DocPatch) {
	fs := in.VFS()
	_, err := vfs.ModifyFileMetadata(fs, fileDoc, patch)
	assert.NoError(t, err)
}

func createSharing(t *testing.T, sharingType string, docID string, withFile, withSelector bool) (*Sharing, error) {
	recipient := createRecipient(t, "hey@mail.fr", recipientURL)

	recStatus := &RecipientStatus{
		RefRecipient: couchdb.DocReference{
			ID:   recipient.RID,
			Type: consts.Contacts,
		},
		recipient: recipient,
	}

	var set permissions.Set
	var rule permissions.Rule
	if docID != "" && !withFile {
		if !withSelector {
			rule = permissions.Rule{
				Type:   "io.cozy.tests",
				Verbs:  permissions.Verbs(permissions.POST, permissions.PUT, permissions.GET),
				Values: []string{docID},
			}
		} else {
			rule = permissions.Rule{
				Type:     "io.cozy.tests",
				Verbs:    permissions.Verbs(permissions.POST, permissions.PUT, permissions.GET),
				Selector: "dyn",
				Values:   []string{"amic"},
			}
		}
		set = permissions.Set{rule}

	} else if docID != "" && withFile {
		rule = permissions.Rule{
			Type:   consts.Files,
			Verbs:  permissions.Verbs(permissions.POST, permissions.PUT, permissions.GET),
			Values: []string{docID, consts.RootDirID},
		}
		set = permissions.Set{rule}
	}

	sharing := &Sharing{
		SharingType:      sharingType,
		Permissions:      set,
		RecipientsStatus: []*RecipientStatus{recStatus},
	}

	err := CreateSharing(in, sharing)
	assert.NoError(t, err)

	return sharing, err
}

func generateAccessCode(t *testing.T, clientID, scope string) (*oauth.AccessCode, error) {
	access, err := oauth.CreateAccessCode(recipientIn, clientID, scope)
	assert.NoError(t, err)
	return access, err
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

func acceptedSharing(t *testing.T, sharingType string, isFile, withSelector bool) {
	var err error
	var testDocFile *vfs.FileDoc
	var testDoc *couchdb.JSONDoc
	var sharing *Sharing

	// share doc
	if !isFile {
		testDoc = createDoc(t, in, testDocType, map[string]interface{}{
			"dyn": "amic",
		})
		assert.NotNil(t, testDoc)
		sharing, err = createSharing(t, sharingType, testDoc.ID(), isFile, withSelector)
		assert.NoError(t, err)
		assert.NotNil(t, sharing)

		// share file
	} else {
		testDocFile = createFile(t, in.VFS(), "testFileAccepted",
			"testFileAcceptedContent", []couchdb.DocReference{})
		assert.NotNil(t, testDocFile)
		sharing, err = createSharing(t, sharingType, testDocFile.ID(), isFile, withSelector)
		assert.NoError(t, err)
		assert.NotNil(t, sharing)
	}

	// `createSharing` only creates one recipient.
	clientID := sharing.RecipientsStatus[0].Client.ClientID

	set := sharing.Permissions
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	access, err := generateAccessCode(t, clientID, scope)
	assert.NoError(t, err)

	domain, err := SharingAccepted(in, sharing.SharingID, clientID, access.Code)
	assert.NoError(t, err)
	assert.NotNil(t, domain)

	// Check sharing status on the sharer side
	doc := &Sharing{}
	err = couchdb.GetDoc(in, consts.Sharings, sharing.SID, doc)
	assert.NoError(t, err)
	recStatuses, err := doc.RecStatus(in)
	assert.NoError(t, err)
	recStatus := recStatuses[0]
	assert.Equal(t, consts.SharingStatusAccepted, recStatus.Status)
	assert.NotNil(t, recStatus.AccessToken.AccessToken)
	assert.NotNil(t, recStatus.AccessToken.RefreshToken)

	// TODO Refactoring necessary: it is no longer possible to actually share
	// between two tests servers as the route to send the documents is declared
	// in web/sharings. Hence checking if the documents do exist will only
	// result in failures.
	t.SkipNow()

	// Check updates in case of master-* sharing
	if sharingType == consts.MasterSlaveSharing ||
		sharingType == consts.MasterMasterSharing {

		// Wait for the document to arrive and check it
		time.Sleep(2000 * time.Millisecond)

		if !isFile {
			if withSelector {
				testDoc2 := createDoc(t, in, testDocType,
					map[string]interface{}{
						"dyn": "amic",
					})
				assert.NotNil(t, testDoc2)

				// Wait for the document to arrive and check it
				time.Sleep(2000 * time.Millisecond)
				recDoc := &couchdb.JSONDoc{}
				err = couchdb.GetDoc(recipientIn, testDocType, testDoc2.ID(), recDoc)
				assert.NoError(t, err)
				recDoc.Type = testDocType
				assert.Equal(t, testDoc2, recDoc)
			} else {
				updKey := "test"
				updVal := "update me!"
				updateTestDoc(t, testDoc, updKey, updVal)
				// Wait for the document to arrive and check it
				time.Sleep(2000 * time.Millisecond)
				recDoc := &couchdb.JSONDoc{}
				err = couchdb.GetDoc(recipientIn, testDocType, testDoc.ID(), recDoc)
				assert.NoError(t, err)
				assert.Equal(t, updVal, recDoc.M[updKey])
			}
		} else {
			newFileName := "mamajustchangedmyname"
			patch := &vfs.DocPatch{
				Name: &newFileName,
			}
			updateTestFile(t, testDocFile, patch)

			// TODO check the file on the recipient side when we'll be able
			// to create files with fixed id
		}
	}
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

	sharing := insertSharingIntoDB(t, consts.MasterMasterSharing, false,
		"io.cozy.events", []*Recipient{}, rule)

	_, err := RecipientRefusedSharing(testInstance, sharing.SharingID)
	assert.NoError(t, err)

	// We also test that the sharing document is actually deleted.
	doc := couchdb.JSONDoc{}
	err = couchdb.GetDoc(TestPrefix, consts.Sharings, sharing.ID(), &doc)
	assert.Error(t, err)
}

func TestCreateSharingRequestBadParams(t *testing.T) {
	_, err := CreateSharingRequest(TestPrefix, "", "", "", "", "")
	assert.Error(t, err)

	state := "1234"
	_, err = CreateSharingRequest(TestPrefix, "", state, "", "", "")
	assert.Error(t, err)

	sharingType := consts.OneShotSharing
	_, err = CreateSharingRequest(TestPrefix, "", state, sharingType, "", "")
	assert.Error(t, err)

	scope := "io.cozy.sharings"
	_, err = CreateSharingRequest(TestPrefix, "", state, sharingType, scope, "")
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

	sharing, err := CreateSharingRequest(TestPrefix, desc, state, sharingType, scope, clientID)
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

func TestDeleteOAuthClient(t *testing.T) {
	client := createOAuthClient(t)

	err := deleteOAuthClient(testInstance, "fakeid")
	assert.Error(t, err)
	err = deleteOAuthClient(testInstance, client.ClientID)
	assert.NoError(t, err)

	_, err = oauth.FindClient(testInstance, client.ClientID)
	assert.Error(t, err)
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
	_ = insertSharingIntoDB(t, consts.MasterSlaveSharing, false,
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
	_ = insertSharingIntoDB(t, consts.MasterSlaveSharing, false,
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

func TestRevokeRecipient(t *testing.T) {
	// We create a sharing with 2 recipients.
	recipient1 := &Recipient{URL: "recipient1.url.cc", Email: "recipient@1"}
	recipient2 := &Recipient{URL: "recipient2.url.cc", Email: "recipient@2"}
	rule := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     "io.cozy.events",
		Values:   []string{"io.cozy.calendar/123"},
		Verbs:    permissions.ALL,
	}
	sharing := insertSharingIntoDB(t, consts.MasterMasterSharing, true,
		"io.cozy.events", []*Recipient{recipient1, recipient2}, rule)

	// We add a trigger to this sharing.
	sched := stack.GetScheduler()
	triggers, _ := sched.GetAll(testInstance.Domain)
	nbTriggers := len(triggers)
	err := AddTrigger(testInstance, rule, sharing.SharingID)
	assert.NoError(t, err)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers+1)

	// Test: we try to remove a recipient that does not belong to this sharing.
	err = RevokeRecipient(testInstance, &sharing, "randomhostclientid")
	assert.Error(t, err)
	assert.Equal(t, ErrRecipientDoesNotExist, err)

	// Test: we remove the first recipient.
	err = RevokeRecipient(testInstance, &sharing,
		sharing.RecipientsStatus[0].HostClientID)
	assert.NoError(t, err)
	// We check that the first recipient was deleted, that the sharing is not
	// revoked, and that the trigger still exists.
	doc := Sharing{}
	err = couchdb.GetDoc(testInstance, consts.Sharings, sharing.ID(), &doc)
	assert.NoError(t, err)
	assert.False(t, doc.Revoked)
	assert.Empty(t, doc.RecipientsStatus[0].HostClientID)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers+1)

	// Test: we remove the second recipient.
	err = RevokeRecipient(testInstance, &sharing,
		sharing.RecipientsStatus[1].HostClientID)
	assert.NoError(t, err)
	// We check that the second recipient was deleted, that the sharing is
	// revoked, and that the trigger was deleted.
	doc = Sharing{}
	err = couchdb.GetDoc(testInstance, consts.Sharings, sharing.ID(), &doc)
	assert.NoError(t, err)
	assert.True(t, doc.Revoked)
	triggers, _ = sched.GetAll(testInstance.Domain)
	assert.Len(t, triggers, nbTriggers)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	testSetup := testutils.NewSetup(m, "pkg_sharings_test")
	testInstance = testSetup.GetTestInstance()

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

	err = stack.Start()
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
	err = stack.Start()
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
