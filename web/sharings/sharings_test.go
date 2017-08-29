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

	"reflect"

	authClient "github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var setup *testutils.TestSetup
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

func createRecipient(t *testing.T, email, url string) *sharings.Recipient {
	recipient := &sharings.Recipient{
		Email: []sharings.RecipientEmail{
			sharings.RecipientEmail{Address: email},
		},
		Cozy: []sharings.RecipientCozy{
			sharings.RecipientCozy{URL: url},
		},
	}
	err := sharings.CreateRecipient(testInstance, recipient)
	assert.NoError(t, err)
	return recipient
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

func createSharing(t *testing.T, sharingID, sharingType string, owner bool, slug string, recipients []*sharings.Recipient, rule permissions.Rule) *sharings.Sharing {
	sharing := &sharings.Sharing{
		SharingType:      sharingType,
		Owner:            owner,
		Permissions:      permissions.Set{rule},
		RecipientsStatus: []*sharings.RecipientStatus{},
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
			recipient.Cozy[0].URL = strings.TrimPrefix(recipient.Cozy[0].URL, "http://")
			err = sharings.CreateRecipient(testInstance, recipient)
			assert.NoError(t, err)
		}

		client := createOAuthClient(t)
		client.CouchID = client.ClientID
		accessToken, errc := client.CreateJWT(testInstance,
			permissions.AccessTokenAudience, scope)
		assert.NoError(t, errc)

		rs := &sharings.RecipientStatus{
			Status: consts.SharingStatusAccepted,
			RefRecipient: couchdb.DocReference{
				ID:   recipient.ID(),
				Type: recipient.DocType(),
			},
			Client: authClient.Client{
				ClientID: client.ClientID,
			},
			AccessToken: authClient.AccessToken{
				AccessToken: accessToken,
				Scope:       scope,
			},
		}

		if sharingType == consts.MasterMasterSharing {
			rs.HostClientID = client.ClientID
		}

		if owner {
			sharing.RecipientsStatus = append(sharing.RecipientsStatus, rs)
		} else {
			sharing.Sharer = sharings.Sharer{
				SharerStatus: rs,
				URL:          recipient.Cozy[0].URL,
			}
			break
		}
	}

	err = couchdb.CreateDoc(testInstance, sharing)
	assert.NoError(t, err)

	return sharing
}

func generateAppToken(t *testing.T, ruleReq permissions.Rule) (string, string) {
	slug := utils.RandomString(15)
	permReq := permissions.Permission{
		Permissions: permissions.Set{ruleReq},
		Type:        permissions.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}

	err := couchdb.CreateDoc(testInstance, &permReq)
	assert.NoError(t, err)
	manifest := &apps.WebappManifest{
		DocSlug:        slug,
		DocPermissions: permissions.Set{ruleReq},
	}
	err = couchdb.CreateNamedDocWithDB(testInstance, manifest)
	assert.NoError(t, err)
	appToken := testInstance.BuildAppToken(manifest)
	return appToken, slug
}

func generateAccessCode(t *testing.T, clientID, scope string) (*oauth.AccessCode, error) {
	access, err := oauth.CreateAccessCode(recipientIn, clientID, scope)
	assert.NoError(t, err)
	return access, err
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

func TestReceiveDocumentSuccessJSON(t *testing.T) {
	jsondataID := "1234bepoauie"
	jsondata := echo.Map{
		"test": "test",
		"id":   jsondataID,
	}
	jsonraw, err := json.Marshal(jsondata)
	assert.NoError(t, err)

	sharing := createSharing(t, "", consts.MasterSlaveSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})

	urlReceive, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlReceive.Path = fmt.Sprintf("/sharings/doc/%s/%s", iocozytests,
		jsondataID)
	urlReceive.RawQuery = url.Values{
		consts.QueryParamSharingID: {sharing.SharingID},
	}.Encode()

	req, err := http.NewRequest(http.MethodPost, urlReceive.String(),
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

	sharing := createSharing(t, "", consts.MasterSlaveSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})

	// Test: creation of a directory that did not existed before.
	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, id)
	strNow := time.Now().Format(time.RFC1123)
	query := url.Values{
		consts.QueryParamSharingID: {sharing.SharingID},
		"Name":       {"TestDir"},
		"Type":       {consts.DirType},
		"Created_at": {strNow},
		"Updated_at": {strNow},
	}
	urlDest.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodPost, urlDest.String(), nil)
	assert.NoError(t, err)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Ensure that the folder was created by fetching it.
	fs := testInstance.VFS()
	dirDoc, err := fs.DirByID(id)
	assert.NoError(t, err)
	assert.Empty(t, dirDoc.ReferencedBy)

	// Test: update of a directory that did exist before.
	refs := []couchdb.DocReference{couchdb.DocReference{Type: "1", ID: "123"}}
	b, err := json.Marshal(refs)
	assert.NoError(t, err)
	references := string(b[:])
	query.Add(consts.QueryParamReferencedBy, references)
	urlDest.RawQuery = query.Encode()

	req, err = http.NewRequest(http.MethodPost, urlDest.String(), nil)
	assert.NoError(t, err)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	dirDoc, err = fs.DirByID(id)
	assert.NoError(t, err)
	assert.NotEmpty(t, dirDoc.ReferencedBy)
}

func TestReceiveDocumentSuccessFile(t *testing.T) {
	id := "testid"
	body := "testoutest"

	sharing := createSharing(t, "", consts.MasterSlaveSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})

	// Test: creation of a file that did not exist.
	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, id)

	reference := []couchdb.DocReference{{ID: "randomid", Type: "randomtype"}}
	refBy, err := json.Marshal(reference)
	assert.NoError(t, err)
	refs := string(refBy)

	strNow := time.Now().Format(time.RFC1123)

	values := url.Values{
		consts.QueryParamSharingID:    {sharing.SharingID},
		consts.QueryParamName:         {"TestFile"},
		consts.QueryParamType:         {consts.FileType},
		consts.QueryParamReferencedBy: {refs},
		consts.QueryParamCreatedAt:    {strNow},
		consts.QueryParamUpdatedAt:    {strNow},
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

	// Test: update of a file that existed, we add another reference.
	refsArr := []couchdb.DocReference{
		couchdb.DocReference{Type: "1", ID: "123"},
	}
	b, err := json.Marshal(refsArr)
	assert.NoError(t, err)
	references := string(b)
	values.Del(consts.QueryParamReferencedBy)
	values.Add(consts.QueryParamReferencedBy, references)
	urlDest.RawQuery = values.Encode()

	req, err = http.NewRequest(http.MethodPost, urlDest.String(), buf)
	assert.NoError(t, err)
	req.Header.Add("Content-MD5", "VkzK5Gw9aNzQdazZe4y1cw==")
	req.Header.Add(echo.HeaderContentType, "text/plain")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	fileDoc, err := fs.FileByID(id)
	assert.NoError(t, err)
	assert.Len(t, fileDoc.ReferencedBy, 2)
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
	doc.Type = iocozytests
	doc.M["testcontent"] = "new"
	values, err := doc.MarshalJSON()
	assert.NoError(t, err)

	// If after an update a document is no longer shared, it is removed.
	rule := permissions.Rule{
		Selector: "_id",
		Type:     iocozytests,
		Verbs:    permissions.ALL,
		Values:   []string{doc.ID()},
	}
	_ = createSharing(t, "", consts.MasterSlaveSharing, false, "",
		[]*sharings.Recipient{}, rule)

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

func TestUpdateDocumentConflictError(t *testing.T) {
	fs := testInstance.VFS()

	fileDoc := createFile(t, fs, "testupdate", "randomcontent",
		[]couchdb.DocReference{})
	updateDoc := createFile(t, fs, "updatetestfile", "updaterandomcontent",
		[]couchdb.DocReference{})

	urlDest, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urlDest.Path = fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
		fileDoc.ID())
	strNow := time.Now().Format(time.RFC1123)
	values := url.Values{
		"Name":       {fileDoc.DocName},
		"Executable": {"false"},
		"Type":       {consts.FileType},
		"rev":        {fileDoc.Rev()},
		"Updated_at": {strNow},
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
	fileDoc := createFile(t, fs, "filetotrash", "randomgarbagecontent",
		[]couchdb.DocReference{})

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
	dirDoc := createDir(t, fs, "dirtotrash", []couchdb.DocReference{})

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
	fileDoc := createFile(t, fs, "filetopatch", "randompatchcontent",
		[]couchdb.DocReference{})
	_, err := fs.FileByID(fileDoc.ID())
	assert.NoError(t, err)

	sharing := createSharing(t, "", consts.MasterSlaveSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})

	patchURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	patchURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", fileDoc.DocType(),
		fileDoc.ID())
	patchURL.RawQuery = url.Values{
		"rev":  {fileDoc.Rev()},
		"Type": {consts.FileType},
		consts.QueryParamSharingID: {sharing.SharingID},
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
	assert.WithinDuration(t, now, patchedFile.UpdatedAt, time.Millisecond)
}

func TestPatchDirOrFileSuccessDir(t *testing.T) {
	fs := testInstance.VFS()
	dirDoc := createDir(t, fs, "dirtopatch", []couchdb.DocReference{})
	_, err := fs.DirByID(dirDoc.ID())
	assert.NoError(t, err)

	sharing := createSharing(t, "", consts.MasterSlaveSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})

	patchURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	patchURL.Path = fmt.Sprintf("/sharings/doc/%s/%s", dirDoc.DocType(),
		dirDoc.ID())
	patchURL.RawQuery = url.Values{
		"rev":  {dirDoc.Rev()},
		"Type": {consts.DirType},
		consts.QueryParamSharingID: {sharing.SharingID},
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
	assert.WithinDuration(t, now, patchedDir.UpdatedAt, time.Millisecond)
}

func TestRemoveReferences(t *testing.T) {
	rule := permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     consts.Files,
		Values:   []string{"io.cozy.photos.albums/123"},
		Verbs:    permissions.ALL,
	}
	_ = createSharing(t, "", consts.MasterSlaveSharing, false, "",
		[]*sharings.Recipient{}, rule)

	refAlbum123 := couchdb.DocReference{
		Type: "io.cozy.photos.albums",
		ID:   "123",
	}
	refAlbum456 := couchdb.DocReference{
		Type: "io.cozy.photos.albums",
		ID:   "456",
	}

	// Test: the file has two references, we remove one and we check that:
	// * that reference was removed;
	// * the file is not trashed (since there is still one shared reference).
	fileToKeep := createFile(t, testInstance.VFS(), "testRemoveReference",
		"testRemoveReferenceContent", []couchdb.DocReference{
			refAlbum123, refAlbum456,
		})

	removeRefURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	removeRefURL.Path = fmt.Sprintf("/sharings/files/%s/referenced_by",
		fileToKeep.ID())
	removeRefURL.RawQuery = url.Values{
		consts.QueryParamSharer: {"false"},
	}.Encode()
	data, err := json.Marshal(refAlbum456)
	assert.NoError(t, err)
	doc := jsonapi.Document{Data: (*json.RawMessage)(&data)}
	body, err := request.WriteJSON(doc)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, removeRefURL.String(), body)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, jsonapi.ContentType)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, res.StatusCode)

	fileDoc, err := testInstance.VFS().FileByID(fileToKeep.ID())
	assert.NoError(t, err)
	assert.False(t, fileDoc.Trashed)
	assert.Len(t, fileDoc.ReferencedBy, 1)

	// Test: the file only has one reference, we remove it and we check that:
	// * the reference was removed;
	// * the file is NOT trashed since it is the sharer.
	removeRefURL.RawQuery = url.Values{
		consts.QueryParamSharer: {"true"},
	}.Encode()
	data, err = json.Marshal(refAlbum123)
	assert.NoError(t, err)
	doc = jsonapi.Document{Data: (*json.RawMessage)(&data)}
	body, err = request.WriteJSON(doc)
	assert.NoError(t, err)

	req, err = http.NewRequest(http.MethodDelete, removeRefURL.String(), body)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, jsonapi.ContentType)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, res.StatusCode)

	fileDoc, err = testInstance.VFS().FileByID(fileToKeep.ID())
	assert.NoError(t, err)
	assert.False(t, fileDoc.Trashed)
	assert.Len(t, fileDoc.ReferencedBy, 0)

	// Test: the directory has one reference, we remove it and check that:
	// * the directory is trashed since it is a recipient;
	// * the reference is gone.
	dirToTrash := createDir(t, testInstance.VFS(), "testRemoveReferenceDir",
		[]couchdb.DocReference{refAlbum123})

	removeRefURL.Path = fmt.Sprintf("/sharings/files/%s/referenced_by",
		dirToTrash.ID())
	removeRefURL.RawQuery = url.Values{
		consts.QueryParamSharer: {"false"},
	}.Encode()
	data, err = json.Marshal(refAlbum123)
	assert.NoError(t, err)
	doc = jsonapi.Document{Data: (*json.RawMessage)(&data)}
	body, err = request.WriteJSON(doc)
	assert.NoError(t, err)

	req, err = http.NewRequest(http.MethodDelete, removeRefURL.String(), body)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add(echo.HeaderContentType, jsonapi.ContentType)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, res.StatusCode)

	dirDoc, err := testInstance.VFS().DirByID(dirToTrash.ID())
	assert.NoError(t, err)
	assert.True(t, dirDoc.DirID == consts.TrashDirID)
	assert.Len(t, dirDoc.ReferencedBy, 0)
}

func TestAddSharingRecipientNoSharing(t *testing.T) {
	res, err := putJSON(t, "/sharings/fakeid/recipient", echo.Map{})
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddSharingRecipientBadRecipient(t *testing.T) {
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})
	args := echo.Map{
		"ID":   "fakeid",
		"Type": "io.cozy.recipients",
	}
	url := "/sharings/" + sharing.ID() + "/recipient"
	res, err := putJSON(t, url, args)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
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

func TestCreateRecipientNoURLNoEmail(t *testing.T) {
	res, err := postJSON(t, "/sharings/recipient", echo.Map{})
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
	recipient := createRecipient(t, "email", "url")
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{recipient}, permissions.Rule{})

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
	recipient := createRecipient(t, "email", "url")
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{recipient}, permissions.Rule{})

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

	ruleSharing := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	setSharing := permissions.Set{ruleSharing}

	ruleReq := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	appToken, _ := generateAppToken(t, ruleReq)

	u := fmt.Sprintf("%s/sharings/", ts.URL)
	v := echo.Map{
		"sharing_type": "badsharingtype",
		"permissions":  setSharing,
	}
	body, _ := json.Marshal(v)

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
}

func TestCreateSharingBadPermission(t *testing.T) {
	ruleSharing := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	setSharing := permissions.Set{ruleSharing}

	ruleReq := permissions.Rule{
		Type:   "io.cozy.badone",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	appToken, _ := generateAppToken(t, ruleReq)

	u := fmt.Sprintf("%s/sharings/", ts.URL)
	v := echo.Map{
		"sharing_type": consts.MasterMasterSharing,
		"permissions":  setSharing,
	}
	body, _ := json.Marshal(v)

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
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

	ruleSharing := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	setSharing := permissions.Set{ruleSharing}

	ruleReq := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	appToken, _ := generateAppToken(t, ruleReq)

	u := fmt.Sprintf("%s/sharings/", ts.URL)
	v := echo.Map{
		"sharing_type": consts.MasterMasterSharing,
		"permissions":  setSharing,
		"recipients":   recipients,
	}
	body, _ := json.Marshal(v)

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestCreateSharingSuccess(t *testing.T) {
	ruleSharing := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	setSharing := permissions.Set{ruleSharing}

	ruleReq := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	appToken, _ := generateAppToken(t, ruleReq)

	u := fmt.Sprintf("%s/sharings/", ts.URL)
	v := echo.Map{
		"sharing_type": consts.MasterMasterSharing,
		"permissions":  setSharing,
	}
	body, _ := json.Marshal(v)

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderContentType, "application/json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
}

func TestGetSharingDocNotFound(t *testing.T) {
	appRule := permissions.Rule{
		Type: "io.cozy.foos",
	}
	appToken, _ := generateAppToken(t, appRule)

	u := fmt.Sprintf("%s/sharings/%s", ts.URL, "fakeid")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestGetSharingDocBadPermissions(t *testing.T) {
	rule := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar"},
		Verbs:  permissions.ALL,
	}
	appRule := permissions.Rule{
		Type: "io.cozy.baddoctype",
	}
	sharing := createSharing(t, "", consts.MasterMasterSharing, true, "",
		[]*sharings.Recipient{}, rule)
	appToken, _ := generateAppToken(t, appRule)

	u := fmt.Sprintf("%s/sharings/%s", ts.URL, sharing.SharingID)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestGetSharingDocSuccess(t *testing.T) {
	rule := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar"},
		Verbs:  permissions.ALL,
	}
	appRule := permissions.Rule{
		Type: "io.cozy.foos",
	}
	sharing := createSharing(t, "", consts.MasterMasterSharing, true, "",
		[]*sharings.Recipient{}, rule)
	appToken, _ := generateAppToken(t, appRule)

	u := fmt.Sprintf("%s/sharings/%s", ts.URL, sharing.SharingID)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestReceiveClientIDBadSharing(t *testing.T) {
	recipient := createRecipient(t, "email", "url")
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{recipient}, permissions.Rule{})
	authCli := authClient.Client{
		ClientID: "myclientid",
	}
	sharing.RecipientsStatus[0].Client = authCli
	err := couchdb.UpdateDoc(testInstance, sharing)
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
	recipient := createRecipient(t, "email", "url")
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{recipient}, permissions.Rule{})
	authCli := authClient.Client{
		ClientID: "myclientid",
	}
	sharing.RecipientsStatus[0].Client = authCli
	err := couchdb.UpdateDoc(testInstance, sharing)
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
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})
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

func TestRevokeSharing(t *testing.T) {
	// Test: revoke a wrong sharing id
	delURL := fmt.Sprintf("%s/sharings/%s", ts.URL, "fakeid")
	req, err := http.NewRequest(http.MethodDelete, delURL, nil)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	// Test: correct sharing id but incorrect permissions.

	rule := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	badRule := permissions.Rule{
		Type: "io.cozy.badone",
	}
	sharing := createSharing(t, "", consts.MasterMasterSharing, true, "",
		[]*sharings.Recipient{}, rule)
	appToken, _ := generateAppToken(t, badRule)

	delURL = fmt.Sprintf("%s/sharings/%s", ts.URL, sharing.SharingID)
	req, err = http.NewRequest(http.MethodDelete, delURL, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)

	// Test: correct sharing id and permission but incorrect recursive param.
	appToken, slug := generateAppToken(t, rule)

	delURL = fmt.Sprintf("%s/sharings/%s", ts.URL, sharing.SharingID)
	queries := url.Values{
		consts.QueryParamRecursive: {"neithertruenorfalse"},
	}.Encode()
	req, err = http.NewRequest(http.MethodDelete, delURL+"?"+queries, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	// Test: correct sharing id, app slug and recursive query param.
	// It should pass.
	delURL = fmt.Sprintf("%s/sharings/%s", ts.URL, sharing.SharingID)
	queries = url.Values{consts.QueryParamRecursive: {"false"}}.Encode()
	req, err = http.NewRequest(http.MethodDelete, delURL+"?"+queries, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// Test: request comes from someone that doesn't have the correct token.
	sharer := createRecipient(t, "email1", "url1")
	sharing = createSharing(t, "", consts.MasterMasterSharing, false, slug,
		[]*sharings.Recipient{sharer}, rule)
	scope, err := rule.MarshalScopeString()
	assert.NoError(t, err)

	badRule = permissions.Rule{
		Selector: consts.SelectorReferencedBy,
		Type:     "io.cozy.foos",
		Values:   []string{"bar", "baz"},
		Verbs:    permissions.ALL,
	}
	badScope, err := badRule.MarshalScopeString()
	assert.NoError(t, err)
	client := createOAuthClient(t)
	client.CouchID = client.ClientID
	tokenBadScope, err := client.CreateJWT(testInstance,
		permissions.AccessTokenAudience, badScope)
	assert.NoError(t, err)

	delURL = fmt.Sprintf("%s/sharings/%s", ts.URL, sharing.SharingID)
	queries = url.Values{consts.QueryParamRecursive: {"false"}}.Encode()
	req, err = http.NewRequest(http.MethodDelete, delURL+"?"+queries, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+tokenBadScope)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)

	// Test: request comes from someone that is not the sharer.
	client = createOAuthClient(t)
	client.CouchID = client.ClientID // CreateJWT expects CouchID
	wrongToken, err := client.CreateJWT(testInstance,
		permissions.AccessTokenAudience, scope)
	assert.NoError(t, err)

	req.Header.Del(echo.HeaderAuthorization)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+wrongToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)

	// Test: request comes from the sharer. It should pass.
	sharerClient, err := oauth.FindClient(testInstance,
		sharing.Sharer.SharerStatus.HostClientID)
	assert.NoError(t, err)
	token, err = sharerClient.CreateJWT(testInstance,
		permissions.AccessTokenAudience, scope)
	assert.NoError(t, err)

	req.Header.Del(echo.HeaderAuthorization)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestRevokeRecipient(t *testing.T) {
	// Test: revoke a wrong sharing id
	delURL := fmt.Sprintf("%s/sharings/%s/recipient/%s", ts.URL,
		"fakesharingid", "fakerecipientid")
	req, err := http.NewRequest(http.MethodDelete, delURL, nil)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	// Test: correct sharing id and app slug while being the sharer: it should
	// pass.
	slug := utils.RandomString(15)
	rule := permissions.Rule{
		Type:   "io.cozy.foos",
		Values: []string{"bar", "baz"},
		Verbs:  permissions.ALL,
	}
	permission := permissions.Permission{
		Permissions: permissions.Set{rule},
		Type:        permissions.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}
	err = couchdb.CreateDoc(testInstance, &permission)
	assert.NoError(t, err)
	manifest := &apps.WebappManifest{
		DocSlug:        slug,
		DocPermissions: permissions.Set{rule},
	}
	err = couchdb.CreateNamedDocWithDB(testInstance, manifest)
	assert.NoError(t, err)
	appToken := testInstance.BuildAppToken(manifest)

	recipient0 := createRecipient(t, "email0", "url0")
	sharing := createSharing(t, "", consts.MasterMasterSharing, true, slug,
		[]*sharings.Recipient{recipient0}, rule)

	delURL = fmt.Sprintf("%s/sharings/%s/recipient/%s", ts.URL,
		sharing.SharingID, sharing.RecipientsStatus[0].Client.ClientID)
	queries := url.Values{consts.QueryParamRecursive: {"false"}}.Encode()
	req, err = http.NewRequest(http.MethodDelete, delURL+"?"+queries, nil)
	assert.NoError(t, err)
	req.Header.Del(echo.HeaderAuthorization)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+appToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// Test: correct sharing id while being the sharer but the request is not
	// coming from the recipient that is to be revoked.
	recipient1 := createRecipient(t, "email1", "url1")
	sharing = createSharing(t, "", consts.MasterMasterSharing, true, slug,
		[]*sharings.Recipient{recipient0, recipient1}, rule)

	delURL = fmt.Sprintf("%s/sharings/%s/recipient/%s", ts.URL,
		sharing.SharingID, sharing.RecipientsStatus[0].HostClientID)
	req, err = http.NewRequest(http.MethodDelete, delURL+"?"+queries, nil)
	assert.NoError(t, err)
	req.Header.Del(echo.HeaderAuthorization)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+
		sharing.RecipientsStatus[1].AccessToken.AccessToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)

	// Test: correct sharing id while being the sharer and the request comes
	// from the recipient that is to be revoked: it should pass.
	req.Header.Del(echo.HeaderAuthorization)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+
		sharing.RecipientsStatus[0].AccessToken.AccessToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestSetDestinationDirectory(t *testing.T) {
	setDestinationURL := fmt.Sprintf("%s/sharings/app/destinationDirectory",
		ts.URL)
	req, err := http.NewRequest(http.MethodPost, setDestinationURL, nil)
	assert.NoError(t, err)

	// Test: no query params given.
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	// Test: only slug given.
	queries := url.Values{
		consts.QueryParamAppSlug: {"randomapp"},
	}.Encode()

	req, err = http.NewRequest(http.MethodPost, setDestinationURL+"?"+queries,
		nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	// Test: doctype + slug given but doctype doesn't exist.
	queries = url.Values{
		consts.QueryParamAppSlug: {"randomapp"},
		consts.QueryParamDocType: {"randomdoctype"},
	}.Encode()

	req, err = http.NewRequest(http.MethodPost, setDestinationURL+"?"+queries,
		nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	// Test: doctype + slug and doctype exists.
	queries = url.Values{
		consts.QueryParamAppSlug: {"randomapp"},
		consts.QueryParamDocType: {"io.cozy.files"},
	}.Encode()

	req, err = http.NewRequest(http.MethodPost, setDestinationURL+"?"+queries,
		nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	// Test: slug + doctype + dirID but dirID doesn't exist.
	queries = url.Values{
		consts.QueryParamAppSlug: {"randomapp"},
		consts.QueryParamDocType: {"io.cozy.files"},
		consts.QueryParamDirID:   {"randomid"},
	}.Encode()

	req, err = http.NewRequest(http.MethodPost, setDestinationURL+"?"+queries,
		nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	// Test: everything was provided.
	queries = url.Values{
		consts.QueryParamAppSlug: {"randomapp"},
		consts.QueryParamDocType: {"io.cozy.files"},
		consts.QueryParamDirID:   {consts.RootDirID},
	}.Encode()

	req, err = http.NewRequest(http.MethodPost, setDestinationURL+"?"+queries,
		nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestDiscoveryFormNoSharingID(t *testing.T) {
	urlVal := url.Values{
		"sharing_id": {""},
	}
	res, err := requestGET("/sharings/discovery", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestDiscoveryFormNoRecipientID(t *testing.T) {
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})
	urlVal := url.Values{
		"sharing_id": {sharing.SharingID},
	}
	res, err := requestGET("/sharings/discovery", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestDiscoveryFormRecipientWithURL(t *testing.T) {
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})
	recipient := createRecipient(t, "email", "url")
	urlVal := url.Values{
		"sharing_id":   {sharing.SharingID},
		"recipient_id": {recipient.ID()},
	}
	res, err := requestGET("/sharings/discovery", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestDiscoveryFormNoEmail(t *testing.T) {
	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{}, permissions.Rule{})
	recipient := &sharings.Recipient{
		Email: []sharings.RecipientEmail{
			sharings.RecipientEmail{Address: "test@mail.fr"},
		},
	}
	err := sharings.CreateRecipient(testInstance, recipient)
	assert.NoError(t, err)
	urlVal := url.Values{
		"sharing_id":   {sharing.SharingID},
		"recipient_id": {recipient.ID()},
	}
	res, err := requestGET("/sharings/discovery", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestDiscoverySuccess(t *testing.T) {
	recipient := createRecipient(t, "email", recipientURL)

	sharing := createSharing(t, "", consts.OneShotSharing, true, "",
		[]*sharings.Recipient{recipient}, permissions.Rule{})
	urlVal := url.Values{
		"sharing_id":   {sharing.SharingID},
		"recipient_id": {sharing.RecipientsStatus[0].RefRecipient.ID},
		"url":          {recipientURL},
	}
	res, err := formPOST("/sharings/discovery", urlVal)
	assert.NoError(t, err)
	assert.Equal(t, 302, res.StatusCode)
}

func TestMergeMetadata(t *testing.T) {
	newMeta := vfs.Metadata{"un": "1", "deux": "2"}
	oldMeta := vfs.Metadata{"trois": "3"}
	expected := vfs.Metadata{"un": "1", "deux": "2", "trois": "3"}

	res := mergeMetadata(newMeta, nil)
	assert.True(t, reflect.DeepEqual(newMeta, res))

	res = mergeMetadata(nil, oldMeta)
	assert.True(t, reflect.DeepEqual(oldMeta, res))

	res = mergeMetadata(newMeta, oldMeta)
	assert.True(t, reflect.DeepEqual(expected, res))
}

func TestMergeReferencedBy(t *testing.T) {
	ref1 := couchdb.DocReference{Type: "1", ID: "123"}
	ref2 := couchdb.DocReference{Type: "2", ID: "456"}
	ref3 := couchdb.DocReference{Type: "3", ID: "789"}
	newRefs := []couchdb.DocReference{ref1, ref2}
	oldRefs := []couchdb.DocReference{ref1, ref3}
	expected := []couchdb.DocReference{ref1, ref2, ref3}

	res := mergeReferencedBy(newRefs, nil)
	assert.True(t, reflect.DeepEqual(newRefs, res))

	res = mergeReferencedBy(nil, oldRefs)
	assert.True(t, reflect.DeepEqual(oldRefs, res))

	res = mergeReferencedBy([]couchdb.DocReference{}, oldRefs)
	assert.True(t, reflect.DeepEqual(oldRefs, res))

	res = mergeReferencedBy(newRefs, oldRefs)
	assert.True(t, reflect.DeepEqual(expected, res))
}

func TestMergeTags(t *testing.T) {
	newTags := []string{"1", "2"}
	oldTags := []string{"2", "3"}
	expected := []string{"1", "2", "3"}

	res := mergeTags(newTags, nil)
	assert.True(t, reflect.DeepEqual(newTags, res))

	res = mergeTags(nil, oldTags)
	assert.True(t, reflect.DeepEqual(oldTags, res))

	res = mergeTags(newTags, oldTags)
	assert.True(t, reflect.DeepEqual(expected, res))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()

	setup = testutils.NewSetup(m, "sharing_test_alice")
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

	jar = setup.GetCookieJar()
	client = &http.Client{
		CheckRedirect: noRedirect,
		Jar:           jar,
	}

	scope := consts.Files + " " + iocozytests + " " + consts.Sharings
	clientOAuth, token = setup.GetTestClient(scope)
	clientID = clientOAuth.ClientID

	routes := map[string]func(*echo.Group){
		"/sharings": Routes,
		"/data":     data.Routes,
	}
	ts = setup.GetTestServerMultipleRoutes(routes)
	ts2 = setup2.GetTestServer("/auth", auth.Routes)
	recipientURL = ts2.URL

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
	fmt.Printf("DEBUG: ts.URL+path: %#v\n", ts.URL+path)
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

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
