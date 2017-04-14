package sharings

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	webAuth "github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")
var domainSharer = "test-sharing.sparta"
var domainRecipient = "test-sharing.xerxes"

var in *instance.Instance
var recipientIn *instance.Instance
var ts *httptest.Server
var recipientURL string
var testDocType = "io.cozy.tests"

func createServer() *httptest.Server {
	createSettings(recipientIn)
	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	handler.Use(injectInstance(recipientIn))
	webAuth.Routes(handler.Group("/auth"))
	data.Routes(handler.Group("/data"))
	files.Routes(handler.Group("/files"))
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

func createRecipient(t *testing.T) (*Recipient, error) {
	recipient := &Recipient{
		Email: "test.fr",
		URL:   "http://" + recipientURL,
	}
	err := CreateRecipient(in, recipient)
	assert.NoError(t, err)
	return recipient, err
}

func createTestDoc(t *testing.T) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    make(map[string]interface{}),
	}
	doc.M["test"] = "hello there"
	doc.M["dyn"] = "amic"
	err := couchdb.CreateDoc(in, doc)
	assert.NoError(t, err)
	return doc, err
}

func createTestFile(t *testing.T) (*vfs.FileDoc, *vfs.File, error) {
	fileContent := "hello !"
	fs := in.VFS()

	fileDoc, err := vfs.NewFileDoc("testfile", "", -1, nil, "", "", time.Now(), false, []string{})
	if err != nil {
		return nil, nil, err
	}
	body := bytes.NewReader([]byte(fileContent))

	f, err := fs.CreateFile(fileDoc, nil)
	if err != nil {
		return nil, nil, err
	}

	n, err := io.Copy(f, body)
	assert.NoError(t, err)
	assert.Equal(t, len(fileContent), int(n))

	if err = f.Close(); err != nil {
		return nil, nil, err
	}
	return fileDoc, &f, nil
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
	recipient, err := createRecipient(t)
	assert.NoError(t, err)

	recStatus := &RecipientStatus{
		RefRecipient: couchdb.DocReference{
			ID:   recipient.RID,
			Type: consts.Recipients,
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

	err = CreateSharingAndRegisterSharer(in, sharing)
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
	doc := &couchdb.JSONDoc{}

	err := couchdb.GetDoc(instance, consts.Settings,
		consts.InstanceSettingsID, doc)
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
		testDoc, err = createTestDoc(t)
		assert.NoError(t, err)
		assert.NotNil(t, testDoc)
		sharing, err = createSharing(t, sharingType, testDoc.ID(), isFile, withSelector)
		assert.NoError(t, err)
		assert.NotNil(t, sharing)

		// share file
	} else {
		testDocFile, _, err = createTestFile(t)
		assert.NoError(t, err)
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
	assert.Equal(t, consts.AcceptedSharingStatus, recStatus.Status)
	assert.NotNil(t, recStatus.AccessToken.AccessToken)
	assert.NotNil(t, recStatus.AccessToken.RefreshToken)

	// Check updates in case of master-* sharing
	if sharingType == consts.MasterSlaveSharing ||
		sharingType == consts.MasterMasterSharing {

		// Wait for the document to arrive and check it
		time.Sleep(2000 * time.Millisecond)

		if !isFile {
			if withSelector {
				var testDoc2 *couchdb.JSONDoc
				testDoc2, err = createTestDoc(t)
				assert.NoError(t, err)
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
		Client:    &auth.Client{},
	}
	_, err := rs.getAccessToken(in, code)
	assert.Error(t, err)
}

func TestGetAccessTokenNoURL(t *testing.T) {
	code := "dummy"
	rs := &RecipientStatus{
		recipient: &Recipient{},
		Client:    &auth.Client{},
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

func TestRegisterNoPublicName(t *testing.T) {
	rs := &RecipientStatus{
		recipient: &Recipient{
			URL: "http://toto.fr",
			RID: "dummyid",
		},
	}
	err := rs.Register(in)
	assert.Equal(t, ErrPublicNameNotDefined, err)
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
		Client: &auth.Client{
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

func TestGetSharingRecipientFromClientIDSuccess(t *testing.T) {
	clientID := "fake client"
	rs := &RecipientStatus{
		Client: &auth.Client{
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

func TestSharingAcceptedBadCode(t *testing.T) {
	s, err := createSharing(t, consts.OneShotSharing, "fakeid", false, false)
	assert.NoError(t, err)
	assert.NotNil(t, s)

	// `createSharing` creates only one recipient.
	clientID := s.RecipientsStatus[0].Client.ClientID

	_, err = SharingAccepted(in, s.SharingID, clientID, "fakeaccessCode")
	assert.Error(t, err)
}

func TestOneShotSharingAcceptedSuccess(t *testing.T) {
	acceptedSharing(t, consts.OneShotSharing, false, false)
}

func TestMasterSlaveSharingAcceptedSuccess(t *testing.T) {
	acceptedSharing(t, consts.MasterSlaveSharing, false, false)
}

func TestOneShotFileSharingAcceptedSuccess(t *testing.T) {
	acceptedSharing(t, consts.OneShotSharing, true, false)
}

func TestMasterSlaveSharingSelectorAcceptedSuccess(t *testing.T) {
	acceptedSharing(t, consts.MasterSlaveSharing, false, true)
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
		Client: &auth.Client{
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

func TestRecipientRefusedSharingWhenSharingIDIsNotUnique(t *testing.T) {
	testSharingID := "sameid"
	testClientID := "notUsedClientID"
	testURL := "notUsedURL"

	_, err := insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID, testURL)
	if err != nil {
		t.FailNow()
	}
	_, err = insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID, testURL)
	if err != nil {
		t.FailNow()
	}

	_, err = RecipientRefusedSharing(TestPrefix, testSharingID)
	assert.Error(t, err)
	assert.Equal(t, ErrSharingIDNotUnique, err)
}

func TestRecipientRefusedSharingSuccess(t *testing.T) {
	testSharingID := "SharingSuccess"
	testClientID := "ClientSuccess"

	docSharingTestID, err := insertSharingDocumentInDB(TestPrefix,
		testSharingID, testClientID, "randomurl")
	if err != nil {
		t.Fail()
	}

	_, err = RecipientRefusedSharing(TestPrefix, testSharingID)
	assert.NoError(t, err)

	// We also test that the sharing document is actually deleted.
	sharing := couchdb.JSONDoc{}
	err = couchdb.GetDoc(TestPrefix, consts.Sharings, docSharingTestID, &sharing)
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
			Type: consts.Recipients,
		},
		recipient: rec,
	}

	sharing := &Sharing{
		SharingType:      "shotmedown",
		RecipientsStatus: []*RecipientStatus{recStatus},
	}

	// `SharingType` is wrong.
	err := CreateSharingAndRegisterSharer(in, sharing)
	assert.Equal(t, ErrBadSharingType, err)

	// `SharingType` is correct.
	sharing.SharingType = consts.OneShotSharing
	// However the recipient is not persisted in the database.
	err = CreateSharingAndRegisterSharer(in, sharing)
	assert.Equal(t, ErrRecipientDoesNotExist, err)

	// The CreateSharingAndRegisterSharer scenario that succeeds is already
	// tested in `createSharing`.
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test needs couchdb to run.")
		os.Exit(1)
	}

	// Change the default config to persist the vfs
	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}
	config.GetConfig().Fs.URL = fmt.Sprintf("file://localhost%s", tempdir)

	err = jobs.StartSystem()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// The instance must be created in db in order to retrieve it from
	// the share_data worker
	_, _ = instance.Destroy(domainSharer)
	in, err = createInstance(domainSharer, "Alice")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	_, _ = instance.Destroy(domainRecipient)
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
	err = couchdb.ResetDB(TestPrefix, consts.Recipients)
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
	err = jobs.StartSystem()
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
	couchdb.DeleteDB(TestPrefix, consts.Recipients)
	ts.Close()

	os.Exit(res)
}

func insertSharingDocumentInDB(db couchdb.Database, sharingID, clientID, URL string) (string, error) {
	sharing := couchdb.JSONDoc{
		Type: consts.Sharings,
		M: map[string]interface{}{
			"sharing_id": sharingID,
			"sharer": map[string]interface{}{
				"client_id": clientID,
				"url":       URL,
			},
		},
	}
	err := couchdb.CreateDoc(db, sharing)
	if err != nil {
		fmt.Printf("Error occurred while trying to insert document: %v\n", err)
		return "", err
	}

	return sharing.ID(), nil
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
