package sharings

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")
var instanceSecret = crypto.GenerateRandomBytes(64)
var in = &instance.Instance{
	OAuthSecret: instanceSecret,
	Domain:      "test-sharing.sparta",
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

	client := &oauth.Client{
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

	client := &oauth.Client{
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
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	err = couchdb.ResetDB(TestPrefix, consts.Sharings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(in, consts.InstanceSettingsID)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	settingsDoc := &couchdb.JSONDoc{
		Type: consts.Settings,
		M:    make(map[string]interface{}),
	}
	settingsDoc.SetID(consts.InstanceSettingsID)
	err = couchdb.CreateNamedDocWithDB(in, settingsDoc)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndex(TestPrefix, mango.IndexOnFields(consts.Sharings, "sharing_id"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()
	couchdb.DeleteDB(TestPrefix, consts.Sharings)
	couchdb.DeleteDB(in, consts.Settings)
	os.Exit(res)
}
