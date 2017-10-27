package sharings

import (
	"testing"

	authClient "github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
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

func createRecipient(t *testing.T, email, url string) *sharings.Recipient {
	recipient := &sharings.Recipient{
		Email: []sharings.RecipientEmail{
			sharings.RecipientEmail{Address: email},
		},
		Cozy: []sharings.RecipientCozy{
			sharings.RecipientCozy{URL: url},
		},
	}
	err := sharings.CreateOrUpdateRecipient(in, recipient)
	assert.NoError(t, err)
	return recipient
}

func createSharing(t *testing.T, sharingType string, owner bool, recipients []*sharings.Recipient, rule permissions.Rule) sharings.Sharing {
	sharing := sharings.Sharing{
		Owner:            owner,
		SharingType:      sharingType,
		SharingID:        utils.RandomString(32),
		Permissions:      permissions.Set{rule},
		RecipientsStatus: []*sharings.RecipientStatus{},
	}

	for _, recipient := range recipients {
		if recipient.ID() == "" {
			recipient = createRecipient(t, recipient.Email[0].Address, recipient.Cozy[0].URL)
		}

		rs := &sharings.RecipientStatus{
			Status: consts.SharingStatusAccepted,
			RefRecipient: couchdb.DocReference{
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
		sharing.RecipientsStatus = append(sharing.RecipientsStatus, rs)
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
	assert.Equal(t, "Sharing does not exist", err.Error())

}

func TestSharingUpdatesBadSharing(t *testing.T) {
	params := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	doc := createDoc(t, testDocType, params)
	sharingDoc := createDoc(t, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()

	msg, event := createEvent(t, doc, "badsharingid", "")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err := SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.Error(t, err)
	assert.Equal(t, ErrSharingDoesNotExist, err)

}

func TestSharingUpdatesTooManySharing(t *testing.T) {
	params := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	doc := createDoc(t, testDocType, params)
	sharingDoc := createDoc(t, consts.Sharings, params)
	sharingDoc2 := createDoc(t, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
		couchdb.DeleteDoc(in, sharingDoc2)

	}()
	sharingID := doc.M["sharing_id"].(string)

	msg, event := createEvent(t, doc, sharingID, "UPDATED")

	j := jobs.NewJob(&jobs.JobRequest{
		Domain:     domainSharer,
		Message:    msg,
		WorkerType: "sharingupdates",
	})

	err := SharingUpdates(jobs.NewWorkerContextWithEvent("123", j, event))
	assert.Error(t, err)
	assert.Equal(t, ErrSharingIDNotUnique, err)
}

func TestSharingUpdatesBadSharingType(t *testing.T) {
	params := map[string]interface{}{
		"sharing_id":   "mysharona.badtype",
		"sharing_type": consts.OneShotSharing,
	}
	doc := createDoc(t, testDocType, params)
	sharingDoc := createDoc(t, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()
	sharingID := sharingDoc.M["sharing_id"].(string)
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
		"sharing_id": "mysharona.norecipient",
	}
	r := permissions.Rule{
		Values: []string{doc.ID()},
	}
	perm := permissions.Set{r}
	sharingParams["permissions"] = perm

	sharingDoc := createDoc(t, consts.Sharings, sharingParams)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()
	sharingID := sharingDoc.M["sharing_id"].(string)
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
		"sharing_id": "mysharona.badrecipient",
	}
	r := permissions.Rule{
		Values: []string{doc.ID()},
	}
	perm := permissions.Set{r}
	sharingParams["permissions"] = perm

	sharingDoc := createDoc(t, consts.Sharings, sharingParams)
	defer func() {
		couchdb.DeleteDoc(in, doc)
		couchdb.DeleteDoc(in, sharingDoc)
	}()
	sharingID := sharingDoc.M["sharing_id"].(string)
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
	sharing := createSharing(t, consts.MasterSlaveSharing, true,
		[]*sharings.Recipient{recipient}, rule)

	sharingID := sharing.SharingID
	sharing.RecipientsStatus[0].Status = consts.SharingStatusRevoked
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
