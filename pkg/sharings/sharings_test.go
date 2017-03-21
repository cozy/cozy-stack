package sharings

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	webAuth "github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")
var instanceSecret = crypto.GenerateRandomBytes(64)
var in = &instance.Instance{
	OAuthSecret: instanceSecret,
	Domain:      "test-sharing.sparta",
}

var ts *httptest.Server

func createInstance(domain string) *instance.Instance {
	instance.Destroy(domain)
	testInstance, err := instance.Create(&instance.Options{
		Domain: domain,
		Locale: "en",
	})
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}
	return testInstance
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
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

func addPublicName(t *testing.T) {
	publicName := "El Shareto"
	doc := &couchdb.JSONDoc{
		Type: consts.Settings,
		M:    make(map[string]interface{}),
	}

	err := couchdb.GetDoc(in, consts.Settings, consts.InstanceSettingsID, doc)
	assert.NoError(t, err)
	doc.M["public_name"] = publicName

	err = couchdb.UpdateDoc(in, doc)
	assert.NoError(t, err)
}

func TestRegisterNoURL(t *testing.T) {
	recipient := &Recipient{}
	err := recipient.Register(in)
	assert.Error(t, err)
	assert.Equal(t, ErrRecipientHasNoURL, err)
}

func TestRegisterNoPublicName(t *testing.T) {
	recipient := &Recipient{
		URL: "toto.fr",
	}
	err := recipient.Register(in)
	assert.Error(t, err)
	assert.Equal(t, ErrPublicNameNotDefined, err)
}

func TestRegisterRecipientNotFound(t *testing.T) {
	recipient := &Recipient{
		URL: "toto.fr",
	}
	addPublicName(t)

	err := recipient.Register(in)
	assert.Error(t, err)
}

func TestRegisterSuccess(t *testing.T) {
	domain := "test-sharing.xerxes"
	testInstance := createInstance(domain)
	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	handler.Use(injectInstance(testInstance))
	webAuth.Routes(handler.Group("/auth"))
	ts = httptest.NewServer(handler)

	url := strings.Split(ts.URL, "http://")[1]

	recipient := &Recipient{
		URL:   url,
		Email: "xerxes@fr",
	}

	err := CreateRecipient(in, recipient)
	assert.NoError(t, err)

	err = recipient.Register(in)
	assert.NoError(t, err)
	assert.NotNil(t, recipient.Client)

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
	var rStatus *RecipientStatus
	sharing := &Sharing{}
	rec, err := sharing.GetSharingRecipientFromClientID(TestPrefix, "")
	assert.NoError(t, err)
	assert.Equal(t, rStatus, rec)
}

func TestGetSharingRecipientFromClientIDNoClient(t *testing.T) {
	clientID := "fake client"

	rStatus := &RecipientStatus{
		RefRecipient: jsonapi.ResourceIdentifier{ID: "id", Type: "type"},
	}
	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rStatus},
	}
	_, err := sharing.GetSharingRecipientFromClientID(TestPrefix, clientID)
	assert.Error(t, err)
}

func TestGetSharingRecipientFromClientIDSuccess(t *testing.T) {
	clientID := "fake client"

	client := &auth.Client{
		ClientID: clientID,
	}
	recipient := &Recipient{
		Client: client,
	}

	couchdb.CreateDoc(TestPrefix, recipient)
	rStatus := &RecipientStatus{
		RefRecipient: jsonapi.ResourceIdentifier{ID: recipient.RID},
	}

	sharing := &Sharing{
		RecipientsStatus: []*RecipientStatus{rStatus},
	}

	recStatus, err := sharing.GetSharingRecipientFromClientID(TestPrefix, clientID)
	assert.NoError(t, err)
	assert.Equal(t, rStatus, recStatus)

}

func TestSharingRefusedNoSharing(t *testing.T) {
	state := "fake state"
	clientID := "fake client"
	err := SharingRefused(TestPrefix, state, clientID)
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
	err = SharingRefused(TestPrefix, state, clientID)
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

	err = SharingRefused(TestPrefix, state, clientID)
	assert.Error(t, err)
}

func TestSharingRefusedSuccess(t *testing.T) {

	state := "stateoftheart2"
	clientID := "thriftshopclient"

	client := &auth.Client{
		ClientID: clientID,
	}
	recipient := &Recipient{
		Client: client,
	}
	err := couchdb.CreateDoc(TestPrefix, recipient)
	assert.NoError(t, err)
	rStatus := &RecipientStatus{
		RefRecipient: jsonapi.ResourceIdentifier{ID: recipient.RID},
	}
	sharing := &Sharing{
		SharingID:        state,
		RecipientsStatus: []*RecipientStatus{rStatus},
	}
	err = couchdb.CreateDoc(TestPrefix, sharing)
	assert.NoError(t, err)
	err = SharingRefused(TestPrefix, state, clientID)
	assert.NoError(t, err)

}

func TestRecipientRefusedSharingWhenSharingDoesNotExist(t *testing.T) {
	err := RecipientRefusedSharing(TestPrefix, "fakesharingid", "fakeclientid")
	assert.Error(t, err)
	assert.Equal(t, ErrSharingDoesNotExist, err)
}

func TestRecipientRefusedSharingWhenSharingIDIsNotUnique(t *testing.T) {
	testSharingID := "sameid"
	testClientID := "notused"

	_, err := insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID)
	if err != nil {
		t.FailNow()
	}
	_, err = insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID)
	if err != nil {
		t.FailNow()
	}

	err = RecipientRefusedSharing(TestPrefix, testSharingID, testClientID)
	assert.Error(t, err)
	assert.Equal(t, ErrSharingIDNotUnique, err)
}

func TestRecipientRefusedSharingWhenOAuthClientDoesNotExist(t *testing.T) {
	testSharingID := "SharingNoOAuthClient"
	testClientID := "ClientNoOAuthClient"

	_, err := insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID)
	if err != nil {
		t.Fail()
	}

	err = RecipientRefusedSharing(TestPrefix, testSharingID, testClientID)
	assert.Error(t, err)
	assert.Equal(t, ErrNoOAuthClient, err)
}

func TestRecipientRefusedSharingWhenPostFails(t *testing.T) {
	testSharingID := "testPostFails"
	testClientID := "clientPostFails"

	docSharingTestID, err := insertSharingDocumentInDB(TestPrefix,
		testSharingID, testClientID)
	if err != nil {
		t.Fail()
	}

	err = insertClientDocumentInDB(TestPrefix, testClientID, "nourl")
	if err != nil {
		t.Fail()
	}

	err = RecipientRefusedSharing(TestPrefix, testSharingID, testClientID)
	assert.Error(t, err)

	out := couchdb.JSONDoc{}
	err = couchdb.GetDoc(TestPrefix, docSharingTestID, consts.Sharings, out)
	assert.Error(t, err)
}

func TestRecipientRefusedSharingWhenResponseStatusCodeIsNotOK(t *testing.T) {
	testSharingID := "SharingStatusNotOK"
	testClientID := "ClientStatusNotOK"

	ts = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusAlreadyReported)
		},
	))
	defer ts.Close()

	err := insertClientDocumentInDB(TestPrefix, testClientID, ts.URL)
	if err != nil {
		t.Fail()
	}

	_, err = insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID)
	if err != nil {
		t.Fail()
	}

	err = RecipientRefusedSharing(TestPrefix, testSharingID, testClientID)
	assert.Error(t, err)
	assert.Equal(t, ErrSharerDidNotReceiveAnswer, err)
}

func TestRecipientRefusedSharingSuccess(t *testing.T) {
	testSharingID := "SharingSuccess"
	testClientID := "ClientSucess"

	ts = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				fmt.Printf(`Error occurred while trying to read request body:
					%v\n`, err)
				t.Fail()
			}
			defer r.Body.Close()
			data := SharingAnswer{}
			_ = json.Unmarshal(body, &data)
			assert.Equal(t, testSharingID, data.SharingID)
			assert.Equal(t, testClientID, data.ClientID)
			assert.Empty(t, data.AccessToken)
			assert.Empty(t, data.RefreshToken)

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
		},
	))
	defer ts.Close()

	err := insertClientDocumentInDB(TestPrefix, testClientID, ts.URL)
	if err != nil {
		t.Fail()
	}

	docSharingTestID, err := insertSharingDocumentInDB(TestPrefix, testSharingID, testClientID)
	if err != nil {
		t.Fail()
	}

	err = RecipientRefusedSharing(TestPrefix, testSharingID, testClientID)
	assert.NoError(t, err)

	// We also test that the sharing document is actually deleted.
	sharing := couchdb.JSONDoc{}
	err = couchdb.GetDoc(TestPrefix, consts.Sharings, docSharingTestID, &sharing)
	assert.Error(t, err)
}

func TestCreateSharingRequestBadParams(t *testing.T) {
	_, err := CreateSharingRequest(TestPrefix, "", "", "", "")
	assert.Error(t, err)

	state := "1234"
	_, err = CreateSharingRequest(TestPrefix, "", state, "", "")
	assert.Error(t, err)

	sharingType := consts.OneShotSharing
	_, err = CreateSharingRequest(TestPrefix, "", state, sharingType, "")
	assert.Error(t, err)

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

	set := permissions.Set{rule}
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	sharing, err := CreateSharingRequest(TestPrefix, desc, state, sharingType, scope)
	assert.NoError(t, err)
	assert.Equal(t, state, sharing.SharingID)
	assert.Equal(t, sharingType, sharing.SharingType)
	assert.Equal(t, set, sharing.Permissions)
}

func TestCreateSuccess(t *testing.T) {
	sharing := &Sharing{
		SharingType: consts.OneShotSharing,
	}
	err := Create(TestPrefix, sharing)
	assert.NoError(t, err)
	assert.NotEmpty(t, sharing.ID())
	assert.NotEmpty(t, sharing.Rev())
	assert.NotEmpty(t, sharing.DocType())
}

func TestCheckSharingCreation(t *testing.T) {

	rec := &Recipient{
		Email: "test@test.fr",
	}

	recStatus := &RecipientStatus{
		RefRecipient: jsonapi.ResourceIdentifier{
			ID:   "123",
			Type: consts.Recipients,
		},
	}

	sharing := &Sharing{
		SharingType:      "shotmedown",
		RecipientsStatus: []*RecipientStatus{recStatus},
	}

	err := CheckSharingCreation(TestPrefix, sharing)
	assert.Error(t, err)

	sharing.SharingType = consts.OneShotSharing
	err = CheckSharingCreation(TestPrefix, sharing)
	assert.Error(t, err)

	err = couchdb.CreateDoc(TestPrefix, rec)
	assert.NoError(t, err)

	recStatus.RefRecipient.ID = rec.RID
	err = CheckSharingCreation(TestPrefix, sharing)
	assert.NoError(t, err)
	assert.Equal(t, true, sharing.Owner)
	assert.NotEmpty(t, sharing.SharingID)

	rStatus := sharing.RecipientsStatus
	for _, rec := range rStatus {
		assert.Equal(t, consts.PendingSharingStatus, rec.Status)
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test needs couchdb to run.")
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

	createSettings(in)

	err = couchdb.DefineIndex(TestPrefix, mango.IndexOnFields(consts.Sharings, "sharing_id"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()
	couchdb.DeleteDB(TestPrefix, consts.Sharings)
	couchdb.DeleteDB(TestPrefix, consts.Recipients)
	couchdb.DeleteDB(in, consts.Settings)
	ts.Close()

	os.Exit(res)
}

func insertSharingDocumentInDB(db couchdb.Database, sharingID, clientID string) (string, error) {
	sharing := couchdb.JSONDoc{
		Type: consts.Sharings,
		M:    make(map[string]interface{}),
	}
	sharing.M["sharing_id"] = sharingID
	sharing.M["client_id"] = clientID
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
		M:    make(map[string]interface{}),
	}
	client.SetID(clientID)
	client.M["client_uri"] = url

	return couchdb.CreateNamedDocWithDB(db, client)
}
