package sharings

import (
	"testing"

	authClient "github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func createDoc(t *testing.T, docType string, params map[string]interface{}) couchdb.JSONDoc {
	// map are references, so beware to remove previous set values
	delete(params, "_id")
	delete(params, "_rev")
	doc := couchdb.JSONDoc{
		Type: docType,
		M:    params,
	}
	err := couchdb.CreateDoc(in, &doc)
	assert.NoError(t, err)

	return doc
}

func createNamedDoc(t *testing.T, docType string, params map[string]interface{}) couchdb.JSONDoc {
	// map are references, so beware to remove previous set values
	delete(params, "_rev")
	doc := couchdb.JSONDoc{
		Type: docType,
		M:    params,
	}
	err := couchdb.CreateNamedDoc(in, &doc)
	assert.NoError(t, err)

	return doc
}

func createEvent(t *testing.T, doc couchdb.JSONDoc, sharingID, eventType string) (jobs.Message, jobs.Event) {
	data := &sharings.SharingMessage{
		SharingID: sharingID,
		Rule: permissions.Rule{
			Description: "randomdesc",
			Selector:    "",
			Type:        doc.DocType(),
			Values:      []string{},
		},
	}
	msg, err := jobs.NewMessage(data)
	assert.NoError(t, err)

	evt, err := jobs.NewEvent(&realtime.Event{
		Verb: eventType,
		Doc:  doc,
	})
	assert.NoError(t, err)
	return msg, evt
}

func createRecipient(t *testing.T, email, url string) *contacts.Contact {
	recipient := &contacts.Contact{
		Email: []contacts.Email{
			contacts.Email{Address: email},
		},
		Cozy: []contacts.Cozy{
			contacts.Cozy{URL: url},
		},
	}
	err := sharings.CreateOrUpdateRecipient(in, recipient)
	assert.NoError(t, err)
	return recipient
}

func createSharing(t *testing.T, sharingType string, owner bool, recipients []*contacts.Contact, rule permissions.Rule) sharings.Sharing {
	sharing := sharings.Sharing{
		Owner:       owner,
		SharingType: sharingType,
		Recipients:  []sharings.Member{},
	}

	// TODO do something with rule

	for _, recipient := range recipients {
		u := recipient.Cozy[0].URL
		if recipient.ID() == "" {
			recipient = createRecipient(t, recipient.Email[0].Address, u)
		}

		rs := sharings.Member{
			Status: consts.SharingStatusAccepted,
			URL:    u,
			RefContact: couchdb.DocReference{
				ID:   recipient.ID(),
				Type: recipient.DocType(),
			},
			Client: authClient.Client{
				ClientID: utils.RandomString(32),
			},
			AccessToken: authClient.AccessToken{
				AccessToken:  utils.RandomString(32),
				RefreshToken: utils.RandomString(32),
			},
		}
		sharing.Recipients = append(sharing.Recipients, rs)
	}
	err := couchdb.CreateDoc(in, &sharing)
	assert.NoError(t, err)

	return sharing
}

func TestSharingUpdatesNoSharing(t *testing.T) {
	doc := createDoc(t, testDocType, map[string]interface{}{"test": "test"})
	defer func() {
		couchdb.DeleteDoc(in, doc)
	}()
	msg, event := createEvent(t, doc, "", "CREATED")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err := SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.Error(t, err)
	assert.Equal(t, ErrSharingDoesNotExist, err)
}

func TestSharingUpdatesBadSharingType(t *testing.T) {
	params := map[string]interface{}{
		"_id":          "mysharona.badtype",
		"sharing_type": consts.OneShotSharing,
	}
	sharingID := params["_id"].(string)
	sharingDoc := createNamedDoc(t, consts.Sharings, params)
	doc := createDoc(t, testDocType, params)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()
	msg, event := createEvent(t, doc, sharingID, "UPDATED")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err := SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.Error(t, err)
	assert.Equal(t, ErrDocumentNotLegitimate, err)
}

func TestSharingUpdatesNoRecipient(t *testing.T) {
	params := map[string]interface{}{
		"test": "testy",
	}
	doc := createDoc(t, testDocType, params)

	sharingParams := map[string]interface{}{
		"_id":   "mysharona.norecipient",
		"owner": true,
	}
	sharingID := sharingParams["_id"].(string)
	sharingDoc := createNamedDoc(t, consts.Sharings, sharingParams)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()
	msg, event := createEvent(t, doc, sharingID, "CREATED")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err := SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.NoError(t, err)
}

func TestSharingUpdatesBadRecipient(t *testing.T) {
	params := map[string]interface{}{
		"test": "testy",
	}
	doc := createDoc(t, testDocType, params)

	sharingParams := map[string]interface{}{
		"_id":   "mysharona.badrecipient",
		"owner": true,
	}
	sharingID := sharingParams["_id"].(string)
	sharingDoc := createNamedDoc(t, consts.Sharings, sharingParams)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()
	msg, event := createEvent(t, doc, sharingID, "CREATED")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err := SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.NoError(t, err)
}

func TestIsDocumentStillShared(t *testing.T) {
	sharedRef := []couchdb.DocReference{
		couchdb.DocReference{Type: "io.cozy.events", ID: "random"},
	}

	optsNotShared := SendOptions{
		Selector: consts.SelectorReferencedBy,
		Values:   []string{"io.cozy.events/static"},
	}
	assert.False(t, isDocumentStillShared(in.VFS(), &optsNotShared, sharedRef))

	optsShared := SendOptions{
		Selector: consts.SelectorReferencedBy,
		Values:   []string{"io.cozy.events/random"},
	}
	assert.True(t, isDocumentStillShared(in.VFS(), &optsShared, sharedRef))

	optsNotShared = SendOptions{
		Values: []string{"123"},
		DocID:  "456",
	}
	assert.False(t, isDocumentStillShared(in.VFS(), &optsNotShared, sharedRef))
}

func TestRevokedRecipient(t *testing.T) {
	rule := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     consts.Files,
		Values:   []string{"third/789"},
	}
	recipient := createRecipient(t, "email", "url")
	sharing := createSharing(t, consts.OneWaySharing, true,
		[]*contacts.Contact{recipient}, rule)

	sharingID := sharing.SID
	sharing.Recipients[0].Status = consts.SharingStatusRevoked
	err := couchdb.UpdateDoc(in, &sharing)
	assert.NoError(t, err)

	params := map[string]interface{}{
		"test": "testy",
	}
	doc := createDoc(t, testDocType, params)
	msg, event := createEvent(t, doc, sharingID, "UPDATED")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err = SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.NoError(t, err)
}
