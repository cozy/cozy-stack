package sharings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"encoding/base64"

	authClient "github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var ts2 *httptest.Server
var testInstance *instance.Instance
var recipientIn *instance.Instance
var clientOAuth *oauth.Client
var clientID string
var jar http.CookieJar
var client *http.Client
var recipientURL string
var token string
var iocozytests = "io.cozy.tests"

func createRecipient(t *testing.T) (*sharings.Recipient, error) {
	recipient := &sharings.Recipient{
		Email: "test.fr",
		URL:   "http://" + recipientURL,
	}
	err := sharings.CreateRecipient(testInstance, recipient)
	assert.NoError(t, err)
	return recipient, err
}

func createSharing(t *testing.T, recipient *sharings.Recipient) (*sharings.Sharing, error) {
	var recs []*sharings.RecipientStatus
	recStatus := new(sharings.RecipientStatus)
	if recipient != nil {
		ref := couchdb.DocReference{
			ID:   recipient.RID,
			Type: consts.Recipients,
		}
		recStatus.RefRecipient = ref
		recs = append(recs, recStatus)
	}

	sharing := &sharings.Sharing{
		SharingType:      consts.OneShotSharing,
		RecipientsStatus: recs,
	}
	err := sharings.CreateSharing(testInstance, sharing)
	assert.NoError(t, err)
	return sharing, err
}

func generateAccessCode(t *testing.T, clientID, scope string) (*oauth.AccessCode, error) {
	access, err := oauth.CreateAccessCode(recipientIn, clientID, scope)
	assert.NoError(t, err)
	return access, err
}

func createFile(t *testing.T, fs vfs.VFS, name, content string) *vfs.FileDoc {
	doc, err := vfs.NewFileDoc(name, "", -1, nil, "foo/bar", "foo", time.Now(), false, false, []string{"this", "is", "spartest"})
	assert.NoError(t, err)

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

func createDir(t *testing.T, fs vfs.VFS, name string) *vfs.DirDoc {
	dirDoc, err := vfs.NewDirDoc(fs, name, "", []string{"It's", "me", "again"})
	assert.NoError(t, err)
	dirDoc.CreatedAt = time.Now()
	dirDoc.UpdatedAt = time.Now()
	err = fs.CreateDir(dirDoc)
	assert.NoError(t, err)

	return dirDoc
}

func TestReceiveDocumentSuccessJSON(t *testing.T) {
	jsondataID := "1234bepoauie"
	jsondata := echo.Map{
		"test": "test",
		"id":   jsondataID,
	}
	jsonraw, err := json.Marshal(jsondata)
	assert.NoError(t, err)

	url, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	url.Path = fmt.Sprintf("/sharings/doc/%s/%s", iocozytests, jsondataID)

	req, err := http.NewRequest(http.MethodPost, url.String(),
		bytes.NewReader(jsonraw))
	assert.NoError(t, err)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Set(echo.HeaderContentType, "application/json")

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	// Ensure that document is present by fetching it.
	doc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, iocozytests, jsondataID, doc)
	assert.NoError(t, err)
}

func TestReceiveDocumentSuccessDir(t *testing.T) {
	id := "0987jldvnrst"

	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, id)
	urlDest.RawQuery = fmt.Sprintf("Name=TestDir&Type=%s", consts.DirType)

	req, err := http.NewRequest(http.MethodPost, urlDest.String(), nil)
	assert.NoError(t, err)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Ensure that the folder was created by fetching it.
	fs := testInstance.VFS()
	_, err = fs.DirByID(id)
	assert.NoError(t, err)
}

func TestReceiveDocumentSuccessFile(t *testing.T) {
	id := "testid"
	body := "testoutest"

	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, id)
	values := url.Values{
		"Name":       {"TestFile"},
		"Executable": {"false"},
		"Type":       {consts.FileType},
	}
	urlDest.RawQuery = values.Encode()
	buf := strings.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, urlDest.String(), buf)
	assert.NoError(t, err)
	req.Header.Add("Content-MD5", "VkzK5Gw9aNzQdazZe4y1cw==")
	req.Header.Add(echo.HeaderContentType, "text/plain")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	fs := testInstance.VFS()
	_, err = fs.FileByID(id)
	assert.NoError(t, err)
}

func TestUpdateDocumentSuccessJSON(t *testing.T) {
	resp, err := postJSON(t, "/data/"+iocozytests+"/", echo.Map{
		"testcontent": "old",
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	doc := couchdb.JSONDoc{}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&doc)
	assert.NoError(t, err)
	doc.SetID(doc.M["id"].(string))
	doc.SetRev(doc.M["rev"].(string))
	doc.Type = doc.M["type"].(string)
	doc.M["testcontent"] = "new"
	values, err := doc.MarshalJSON()
	assert.NoError(t, err)

	path := fmt.Sprintf("/sharings/doc/%s/%s", doc.DocType(), doc.ID())
	req, err := http.NewRequest(http.MethodPut, ts.URL+path,
		bytes.NewReader(values))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, "application/json")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	updatedDoc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, doc.DocType(), doc.ID(), updatedDoc)
	assert.NoError(t, err)
	assert.Equal(t, doc.M["testcontent"], updatedDoc.M["testcontent"])
}

func TestUpdateDocumentSuccessFile(t *testing.T) {
	t.SkipNow()
	fs := testInstance.VFS()

	fileDoc := createFile(t, fs, "testupdate", "randomcontent")
	updateDoc := createFile(t, fs, "updatetestfile", "updaterandomcontent")

	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
		fileDoc.ID())
	values := url.Values{
		"Name":       {fileDoc.DocName},
		"Executable": {"false"},
		"Type":       {consts.FileType},
		"rev":        {fileDoc.Rev()},
	}
	urlDest.RawQuery = values.Encode()

	body, err := fs.OpenFile(updateDoc)
	assert.NoError(t, err)
	defer body.Close()

	req, err := http.NewRequest(http.MethodPut, urlDest.String(), body)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, updateDoc.Mime)
	req.Header.Add("Content-MD5",
		base64.StdEncoding.EncodeToString(updateDoc.MD5Sum))
	req.Header.Add(echo.HeaderAcceptEncoding, "application/vnd.api+json")

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// TODO
	// - get updated file using fileDoc.ID()
	// - check that md5 matches that of updateDoc.MD5
	updatedFileDoc, err := fs.FileByID(fileDoc.ID())
	assert.NoError(t, err)
	assert.Equal(t, base64.StdEncoding.EncodeToString(updateDoc.MD5Sum),
		base64.StdEncoding.EncodeToString(updatedFileDoc.MD5Sum))
}

func TestDeleteDocumentSuccessJSON(t *testing.T) {
	// To delete a JSON we need to create one and get its revision.
	resp, err := postJSON(t, "/data/"+iocozytests+"/", echo.Map{
		"test": "content",
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	doc := couchdb.JSONDoc{}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&doc)
	assert.NoError(t, err)

	delURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	delURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", doc.M["type"], doc.M["id"])
	delURL.RawQuery = url.Values{"rev": {doc.M["rev"].(string)}}.Encode()

	req, err := http.NewRequest("DELETE", delURL.String(), nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	delDoc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(testInstance, doc.DocType(), doc.ID(), delDoc)
	assert.Error(t, err)
}

func TestDeleteDocumentSuccessFile(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "filetotrash", "randomgarbagecontent")

	delURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	delURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
		fileDoc.ID())
	delURL.RawQuery = url.Values{
		"rev":  {fileDoc.Rev()},
		"Type": {consts.FileType},
	}.Encode()

	req, err := http.NewRequest("DELETE", delURL.String(), nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	trashedFileDoc, err := fs.FileByID(fileDoc.ID())
	assert.NoError(t, err)
	assert.True(t, trashedFileDoc.Trashed)
}

func TestDeleteDocumentSuccessDir(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "dirtotrash")

	delURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	delURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
		dirDoc.ID())
	delURL.RawQuery = url.Values{
		"rev":  {dirDoc.Rev()},
		"Type": {consts.DirType},
	}.Encode()

	req, err := http.NewRequest("DELETE", delURL.String(), nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	trashedDirDoc, err := fs.DirByID(dirDoc.ID())
	assert.NoError(t, err)
	assert.Equal(t, consts.TrashDirID, trashedDirDoc.DirID)
}

func TestPatchDirOrFileSuccessFile(t *testing.T) {
	fs := testInstance.VFS()
	fileDoc := createFile(t, fs, "filetopatch", "randompatchcontent")
	_, err := fs.FileByID(fileDoc.ID())
	assert.NoError(t, err)

	patchURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	patchURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
		fileDoc.ID())
	patchURL.RawQuery = url.Values{
		"rev":  {fileDoc.Rev()},
		"Type": {consts.FileType},
	}.Encode()

	patchedName := "patchedfilename"
	now := time.Now()
	patch := &vfs.DocPatch{
		Name:      &patchedName,
		DirID:     &fileDoc.DirID,
		Tags:      &fileDoc.Tags,
		UpdatedAt: &now,
	}
	attrs, err := json.Marshal(patch)
	assert.NoError(t, err)
	obj := &jsonapi.ObjectMarshalling{
		Type:       consts.Files,
		ID:         fileDoc.ID(),
		Attributes: (*json.RawMessage)(&attrs),
		Meta:       jsonapi.Meta{Rev: fileDoc.Rev()},
	}
	data, err := json.Marshal(obj)
	docPatch := &jsonapi.Document{Data: (*json.RawMessage)(&data)}
	assert.NoError(t, err)
	body, err := request.WriteJSON(docPatch)
	assert.NoError(t, err)

	req, err := http.NewRequest("PATCH", patchURL.String(), body)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, jsonapi.ContentType)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	patchedFile, err := fs.FileByID(fileDoc.ID())
	assert.NoError(t, err)
	assert.Equal(t, patchedName, patchedFile.DocName)
	assert.Equal(t, now, patchedFile.UpdatedAt)
}

func TestPatchDirOrFileSuccessDir(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "dirtopatch")
	_, err := fs.DirByID(dirDoc.ID())
	assert.NoError(t, err)

	patchURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	patchURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
		dirDoc.ID())
	patchURL.RawQuery = url.Values{
		"rev":  {dirDoc.Rev()},
		"Type": {consts.DirType},
	}.Encode()

	patchedName := "patcheddirname"
	now := time.Now()
	patch := &vfs.DocPatch{
		Name:      &patchedName,
		DirID:     &dirDoc.DirID,
		Tags:      &dirDoc.Tags,
		UpdatedAt: &now,
	}
	attrs, err := json.Marshal(patch)
	assert.NoError(t, err)
	obj := &jsonapi.ObjectMarshalling{
		Type:       consts.Files,
		ID:         dirDoc.ID(),
		Attributes: (*json.RawMessage)(&attrs),
		Meta:       jsonapi.Meta{Rev: dirDoc.Rev()},
	}
	data, err := json.Marshal(obj)
	docPatch := &jsonapi.Document{Data: (*json.RawMessage)(&data)}
	assert.NoError(t, err)
	body, err := request.WriteJSON(docPatch)
	assert.NoError(t, err)

	req, err := http.NewRequest("PATCH", patchURL.String(), body)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, jsonapi.ContentType)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	patchedDir, err := fs.DirByID(dirDoc.ID())
	assert.NoError(t, err)
	assert.Equal(t, patchedName, patchedDir.DocName)
	assert.Equal(t, now, patchedDir.UpdatedAt)
}

func TestAddSharingRecipientNoSharing(t *testing.T) {
	res, err := putJSON(t, "/sharings/fakeid/recipient", echo.Map{})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddSharingRecipientBadRecipient(t *testing.T) {
	sharing, err := createSharing(t, nil)
	assert.NoError(t, err)
	args := echo.Map{
		"ID":   "fakeid",
		"Type": "io.cozy.recipients",
	}
	url := "/sharings/" + sharing.ID() + "/recipient"
	res, err := putJSON(t, url, args)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddSharingRecipientSuccess(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	args := echo.Map{
		"ID":   recipient.ID(),
		"Type": "io.cozy.recipients",
	}
	url := "/sharings/" + sharing.ID() + "/recipient"
	res, err := putJSON(t, url, args)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}

func TestRecipientRefusedSharingWhenThereIsNoState(t *testing.T) {
	urlVal := url.Values{
		"state":     {""},
		"client_id": {"randomclientid"},
	}

	resp, err := formPOST("/sharings/formRefuse", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 400)
}

func TestRecipientRefusedSharingWhenThereIsNoClientID(t *testing.T) {
	urlVal := url.Values{
		"state":     {"randomsharingid"},
		"client_id": {""},
	}

	resp, err := formPOST("/sharings/formRefuse", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 400)
}

func TestRecipientRefusedSharingSuccess(t *testing.T) {
	// To be able to refuse a sharing we first need to receive a sharing
	// request… This is a copy/paste of the code found in the test:
	// TestSharingRequestSuccess.
	rule := permissions.Rule{
		Type:        "io.cozy.events",
		Title:       "event",
		Description: "My event",
		Verbs:       permissions.VerbSet{permissions.POST: {}},
		Values:      []string{"1234"},
	}
	set := permissions.Set{rule}
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	state := "sharing_id"
	desc := "share cher"

	urlVal := url.Values{
		"desc":          {desc},
		"state":         {state},
		"scope":         {scope},
		"sharing_type":  {consts.OneShotSharing},
		"client_id":     {clientID},
		"redirect_uri":  {clientOAuth.RedirectURIs[0]},
		"response_type": {"code"},
	}

	req, _ := http.NewRequest("GET", ts.URL+"/sharings/request?"+urlVal.Encode(), nil)
	noRedirectClient := http.Client{CheckRedirect: noRedirect}
	res, err := noRedirectClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()

	resp, err := formPOST("/sharings/formRefuse", url.Values{
		"state":     {state},
		"client_id": {clientID},
	})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestSharingAnswerBadState(t *testing.T) {
	urlVal := url.Values{
		"state": {""},
	}
	res, err := requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateRecipientNoURL(t *testing.T) {
	email := "mailme@maybe"
	res, err := postJSON(t, "/sharings/recipient", echo.Map{
		"email": email,
	})
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestCreateRecipientSuccess(t *testing.T) {
	email := "mailme@maybe"
	url := strings.Split(ts2.URL, "http://")[1]
	res, err := postJSON(t, "/sharings/recipient", echo.Map{
		"url":   url,
		"email": email,
	})

	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
}

func TestSharingAnswerBadClientID(t *testing.T) {
	urlVal := url.Values{
		"state":     {"stateoftheart"},
		"client_id": {"myclient"},
	}
	res, err := requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestSharingAnswerBadCode(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	assert.NotNil(t, recipient)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)

	urlVal := url.Values{
		"state":       {sharing.SharingID},
		"client_id":   {sharing.RecipientsStatus[0].Client.ClientID},
		"access_code": {"fakeaccess"},
	}
	res, err := requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestSharingAnswerSuccess(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	assert.NotNil(t, recipient)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)

	cID := sharing.RecipientsStatus[0].Client.ClientID

	access, err := generateAccessCode(t, cID, "")
	assert.NoError(t, err)
	assert.NotNil(t, access)

	urlVal := url.Values{
		"state":       {sharing.SharingID},
		"client_id":   {cID},
		"access_code": {access.Code},
	}
	_, err = requestGET("/sharings/answer", urlVal)
	assert.NoError(t, err)
}

func TestSharingRequestNoScope(t *testing.T) {
	urlVal := url.Values{
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoState(t *testing.T) {
	urlVal := url.Values{
		"scope":        {"dummyscope"},
		"sharing_type": {consts.OneShotSharing},
		"client_id":    {"dummyclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoSharingType(t *testing.T) {
	urlVal := url.Values{
		"scope":     {"dummyscope"},
		"state":     {"dummystate"},
		"client_id": {"dummyclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestSharingRequestBadScope(t *testing.T) {
	urlVal := url.Values{
		"scope":        []string{":"},
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
		"client_id":    {"dummyclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestNoClientID(t *testing.T) {
	urlVal := url.Values{
		"scope":        {"dummyscope"},
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestBadClientID(t *testing.T) {
	urlVal := url.Values{
		"scope":        {"dummyscope"},
		"state":        {"dummystate"},
		"sharing_type": {consts.OneShotSharing},
		"client_id":    {"badclientid"},
	}
	res, err := requestGET("/sharings/request", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestSharingRequestSuccess(t *testing.T) {

	rule := permissions.Rule{
		Type:        "io.cozy.events",
		Title:       "event",
		Description: "My event",
		Verbs:       permissions.VerbSet{permissions.POST: {}},
		Values:      []string{"1234"},
	}
	set := permissions.Set{rule}
	scope, err := set.MarshalScopeString()
	assert.NoError(t, err)

	state := "sharing_id"
	desc := "share cher"

	urlVal := url.Values{
		"desc":          {desc},
		"state":         {state},
		"scope":         {scope},
		"sharing_type":  {consts.OneShotSharing},
		"client_id":     {clientID},
		"redirect_uri":  {clientOAuth.RedirectURIs[0]},
		"response_type": {"code"},
	}

	req, _ := http.NewRequest("GET", ts.URL+"/sharings/request?"+urlVal.Encode(), nil)
	noRedirectClient := http.Client{CheckRedirect: noRedirect}
	res, err := noRedirectClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusSeeOther, res.StatusCode)
}

func TestCreateSharingWithBadType(t *testing.T) {
	res, err := postJSON(t, "/sharings/", echo.Map{
		"sharing_type": "shary pie",
	})
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestSendMailsWithWrongSharingID(t *testing.T) {
	req, _ := http.NewRequest("PUT", ts.URL+"/sharings/wrongid/sendMails",
		nil)

	res, err := http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateSharingWithNonExistingRecipient(t *testing.T) {
	type recipient map[string]map[string]string

	rec := recipient{
		"recipient": {
			"id": "hodor",
		},
	}
	recipients := []recipient{rec}

	res, err := postJSON(t, "/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
		"recipients":   recipients,
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateSharingSuccess(t *testing.T) {
	res, err := postJSON(t, "/sharings/", echo.Map{
		"sharing_type": consts.OneShotSharing,
	})
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
}

func TestReceiveClientIDBadSharing(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)
	authCli := &authClient.Client{
		ClientID: "myclientid",
	}
	sharing.RecipientsStatus[0].Client = authCli
	err = couchdb.UpdateDoc(testInstance, sharing)
	assert.NoError(t, err)
	res, err := postJSON(t, "/sharings/access/client", echo.Map{
		"state":          "fakestate",
		"client_id":      "fakeclientid",
		"host_client_id": "newclientid",
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestReceiveClientIDSuccess(t *testing.T) {
	recipient, err := createRecipient(t)
	assert.NoError(t, err)
	sharing, err := createSharing(t, recipient)
	assert.NoError(t, err)
	assert.NotNil(t, sharing)
	authCli := &authClient.Client{
		ClientID: "myclientid",
	}
	sharing.RecipientsStatus[0].Client = authCli
	err = couchdb.UpdateDoc(testInstance, sharing)
	assert.NoError(t, err)
	res, err := postJSON(t, "/sharings/access/client", echo.Map{
		"state":          sharing.SharingID,
		"client_id":      sharing.RecipientsStatus[0].Client.ClientID,
		"host_client_id": "newclientid",
	})
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}

func TestGetAccessTokenMissingState(t *testing.T) {
	res, err := postJSON(t, "/sharings/access/code", echo.Map{
		"state": "",
	})
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
}

func TestGetAccessTokenMissingCode(t *testing.T) {
	sharing, err := createSharing(t, nil)
	assert.NoError(t, err)
	res, err := postJSON(t, "/sharings/access/code", echo.Map{
		"state": sharing.SharingID,
	})
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestGetAccessTokenBadState(t *testing.T) {
	res, err := postJSON(t, "/sharings/access/code", echo.Map{
		"state": "fakeid",
		"code":  "fakecode",
	})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()

	setup := testutils.NewSetup(m, "sharing_test_alice")
	setup2 := testutils.NewSetup(m, "sharing_test_bob")
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	settings.M["public_name"] = "Alice"
	testInstance = setup.GetTestInstance(&instance.Options{
		Settings: settings,
	})
	var settings2 couchdb.JSONDoc
	settings2.M = make(map[string]interface{})
	settings2.M["public_name"] = "Bob"
	recipientIn = setup2.GetTestInstance(&instance.Options{
		Settings: settings2,
	})

	err := couchdb.ResetDB(testInstance, iocozytests)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(testInstance, consts.Files)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jar = setup.GetCookieJar()
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	scope := consts.Files + " " + iocozytests + " " + consts.Sharings
	clientOAuth, token = setup.GetTestClient(scope)
	clientID = clientOAuth.ClientID

	// As shared files are put in the shared with me dir, we need it
	err = createSharedWithMeDir(testInstance.VFS())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	routes := map[string]func(*echo.Group){
		"/sharings": Routes,
		"/data":     data.Routes,
	}
	ts = setup.GetTestServerMultipleRoutes(routes)
	ts2 = setup2.GetTestServer("/auth", auth.Routes)
	recipientURL = strings.Split(ts2.URL, "http://")[1]

	setup.AddCleanup(func() error { setup2.Cleanup(); return nil })

	os.Exit(setup.Run())
}

func postJSON(t *testing.T, path string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, err := http.NewRequest(http.MethodPost, ts.URL+path,
		bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, "application/json")

	return http.DefaultClient.Do(req)
}

func putJSON(t *testing.T, path string, v echo.Map) (*http.Response, error) {
	body, _ := json.Marshal(v)
	req, err := http.NewRequest(http.MethodPut, ts.URL+path,
		bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, "application/json")

	return http.DefaultClient.Do(req)
}

func requestGET(u string, v url.Values) (*http.Response, error) {
	if v != nil {
		reqURL := v.Encode()
		return http.Get(ts.URL + u + "?" + reqURL)
	}
	return http.Get(ts.URL + u)
}

func formPOST(u string, v url.Values) (*http.Response, error) {
	req, _ := http.NewRequest("POST", ts.URL+u, strings.NewReader(v.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Host = testInstance.Domain
	noRedirectClient := http.Client{CheckRedirect: noRedirect}
	return noRedirectClient.Do(req)
}

func extractJSONRes(res *http.Response, mp *map[string]interface{}) error {
	if res.StatusCode >= 300 {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(mp)
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
