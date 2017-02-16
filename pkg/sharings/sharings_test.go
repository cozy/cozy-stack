package sharings

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")

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

func TestCreate(t *testing.T) {
	sharing := &Sharing{
		SharingType: consts.OneShotSharing,
	}
	_, err := Create(TestPrefix, sharing)
	assert.NoError(t, err)
	assert.NotEmpty(t, sharing.ID())
	assert.NotEmpty(t, sharing.Rev())
	assert.NotEmpty(t, sharing.DocType())
}

func TestCheckSharingCreation(t *testing.T) {

	rec := &Recipient{
		Email: "test@test.fr",
	}

	sRec := &SharingRecipient{
		RefRecipient: jsonapi.ResourceIdentifier{
			ID:   "123",
			Type: consts.Recipients,
		},
	}

	sharing := &Sharing{
		SharingType: "shotmedown",
		SRecipients: []*SharingRecipient{sRec},
	}

	err := CheckSharingCreation(TestPrefix, sharing)
	assert.Error(t, err)

	sharing.SharingType = consts.OneShotSharing
	err = CheckSharingCreation(TestPrefix, sharing)
	assert.Error(t, err)

	err = couchdb.CreateDoc(TestPrefix, rec)
	assert.NoError(t, err)

	sRec.RefRecipient.ID = rec.RID
	err = CheckSharingCreation(TestPrefix, sharing)
	assert.NoError(t, err)
	assert.Equal(t, true, sharing.Owner)
	assert.NotEmpty(t, sharing.SharingID)

	sRecipients := sharing.SRecipients
	for _, sRec := range sRecipients {
		assert.Equal(t, consts.PendingStatus, sRec.Status)
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

	res := m.Run()
	couchdb.DeleteDB(TestPrefix, consts.Sharings)
	os.Exit(res)
}
