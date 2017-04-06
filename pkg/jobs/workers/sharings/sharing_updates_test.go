package workers

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/stretchr/testify/assert"
)

func createDoc(t *testing.T, db couchdb.Database, docType string, params map[string]interface{}) couchdb.JSONDoc {
	// map are references, so beware to remove previous set values
	delete(params, "_id")
	delete(params, "_rev")
	doc := couchdb.JSONDoc{
		Type: docType,
		M:    params,
	}
	err := couchdb.CreateDoc(db, &doc)
	assert.NoError(t, err)

	return doc
}

func createEvent(t *testing.T, doc couchdb.JSONDoc, sharingID string) *TriggerEvent {
	msg := &SharingMessage{
		SharingID: sharingID,
		DocType:   doc.Type,
	}
	event := &TriggerEvent{
		Event:   &EventDoc{Doc: &doc},
		Message: msg,
	}
	return event
}

func TestSharingUpdatesNoSharing(t *testing.T) {
	domain := "nosharing.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)
	doc := createDoc(t, db, testDocType, map[string]interface{}{"test": "test"})
	defer func() {
		couchdb.DeleteDoc(db, doc)
	}()
	event := createEvent(t, doc, "")

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): Database does not exist.", err.Error())
}

func TestSharingUpdatesBadSharing(t *testing.T) {
	domain := "badsharing.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)
	params := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	doc := createDoc(t, db, testDocType, params)
	sharingDoc := createDoc(t, db, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(db, doc)
		couchdb.DeleteDoc(db, sharingDoc)
	}()
	err := couchdb.DefineIndex(db, mango.IndexOnFields(
		consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	event := createEvent(t, doc, "badsharingid")

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, ErrSharingDoesNotExist, err)
}

func TestSharingUpdatesTooManySharing(t *testing.T) {
	domain := "toomanysharing.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)
	params := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	doc := createDoc(t, db, testDocType, params)
	sharingDoc := createDoc(t, db, consts.Sharings, params)
	sharingDoc2 := createDoc(t, db, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(db, doc)
		couchdb.DeleteDoc(db, sharingDoc)
		couchdb.DeleteDoc(db, sharingDoc2)
	}()
	err := couchdb.DefineIndex(db, mango.IndexOnFields(
		consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sharingID := doc.M["sharing_id"].(string)

	event := createEvent(t, doc, sharingID)

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, ErrSharingIDNotUnique, err)
}

func TestSharingUpdatesIllegitimateDoc(t *testing.T) {
	domain := "illegitimate.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)
	params := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	doc := createDoc(t, db, testDocType, params)
	sharingDoc := createDoc(t, db, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(db, doc)
		couchdb.DeleteDoc(db, sharingDoc)
	}()
	err := couchdb.DefineIndex(db, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sharingID := sharingDoc.M["sharing_id"].(string)
	event := createEvent(t, doc, sharingID)

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, ErrDocumentNotLegitimate, err)
}

func TestSharingUpdatesBadSharingType(t *testing.T) {
	domain := "badsharingtype.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)
	params := map[string]interface{}{
		"sharing_id":   "mysharona",
		"sharing_type": consts.OneShotSharing,
	}
	doc := createDoc(t, db, testDocType, params)
	sharingDoc := createDoc(t, db, consts.Sharings, params)
	defer func() {
		couchdb.DeleteDoc(db, doc)
		couchdb.DeleteDoc(db, sharingDoc)
	}()
	err := couchdb.DefineIndex(db, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sharingID := sharingDoc.M["sharing_id"].(string)
	event := createEvent(t, doc, sharingID)

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, ErrDocumentNotLegitimate, err)
}

func TestSharingUpdatesNoRecipient(t *testing.T) {
	domain := "success.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)

	params := map[string]interface{}{
		"test": "testy",
	}
	doc := createDoc(t, db, testDocType, params)

	sharingParams := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	r := permissions.Rule{
		Values: []string{doc.ID()},
	}
	perm := permissions.Set{r}
	sharingParams["permissions"] = perm

	sharingDoc := createDoc(t, db, consts.Sharings, sharingParams)
	defer func() {
		couchdb.DeleteDoc(db, doc)
		couchdb.DeleteDoc(db, sharingDoc)
	}()
	err := couchdb.DefineIndex(db, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sharingID := sharingDoc.M["sharing_id"].(string)
	event := createEvent(t, doc, sharingID)

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.NoError(t, err)
}

func TestSharingUpdatesBadRecipient(t *testing.T) {
	domain := "success.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)

	params := map[string]interface{}{
		"test": "testy",
	}
	doc := createDoc(t, db, testDocType, params)

	sharingParams := map[string]interface{}{
		"sharing_id": "mysharona",
	}
	r := permissions.Rule{
		Values: []string{doc.ID()},
	}
	perm := permissions.Set{r}
	sharingParams["permissions"] = perm

	sharingDoc := createDoc(t, db, consts.Sharings, sharingParams)
	defer func() {
		couchdb.DeleteDoc(db, doc)
		couchdb.DeleteDoc(db, sharingDoc)
	}()
	err := couchdb.DefineIndex(db, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sharingID := sharingDoc.M["sharing_id"].(string)
	event := createEvent(t, doc, sharingID)

	msg, err := jobs.NewMessage(jobs.JSONEncoding, event)
	assert.NoError(t, err)

	err = SharingUpdates(jobs.NewWorkerContext(domain), msg)
	assert.NoError(t, err)
}
