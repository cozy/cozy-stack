package sharings_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/labstack/echo"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

// Things for the replicator tests
var tsR *httptest.Server
var replInstance *instance.Instance
var replSharingID, replAccessToken string

const replDoctype = "io.cozy.replicator.tests"

// It's not really a test, more a setup for the replicator tests
func TestCreateSharingForReplicatorTest(t *testing.T) {
	rule := sharing.Rule{
		Title:    "tests",
		DocType:  replDoctype,
		Selector: "foo",
		Values:   []string{"bar", "baz"},
		Add:      "sync",
		Update:   "sync",
		Remove:   "sync",
	}
	s := sharing.Sharing{
		Description: "replicator tests",
		Rules:       []sharing.Rule{rule},
	}
	assert.NoError(t, s.BeOwner(replInstance, ""))
	s.Members = append(s.Members, sharing.Member{
		Status:   sharing.MemberStatusReady,
		Name:     "J. Doe",
		Email:    "j.doe@example.net",
		Instance: "https://j.example.net/",
	})
	s.Credentials = append(s.Credentials, sharing.Credentials{})
	_, err := s.Create(replInstance)
	assert.NoError(t, err)
	replSharingID = s.SID

	cli, err := sharing.CreateOAuthClient(replInstance, &s.Members[1])
	assert.NoError(t, err)
	s.Credentials[0].Client = sharing.ConvertOAuthClient(cli)
	token, err := sharing.CreateAccessToken(replInstance, cli, s.SID)
	assert.NoError(t, err)
	s.Credentials[0].AccessToken = token
	assert.NoError(t, couchdb.UpdateDoc(replInstance, &s))
	replAccessToken = token.AccessToken
}

func uuidv4() string {
	id, _ := uuid.NewV4()
	return id.String()
}

func createShared(t *testing.T, id string, revisions []string) *sharing.SharedRef {
	doc := sharing.SharedRef{
		SID:       id,
		Revisions: revisions,
	}
	err := couchdb.CreateNamedDocWithDB(replInstance, &doc)
	assert.NoError(t, err)
	return &doc
}

func TestPermissions(t *testing.T) {
	assert.NotNil(t, replSharingID)
	assert.NotNil(t, replAccessToken)

	id := replDoctype + "/" + uuidv4()
	doc := createShared(t, id, []string{"1-111111111"})

	body, _ := json.Marshal(sharing.Changes{
		"id": doc.Revisions,
	})
	u := tsR.URL + "/sharings/" + replSharingID + "/revs_diff"

	r := bytes.NewReader(body)
	req, err := http.NewRequest(http.MethodPost, u, r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAccept, "application/json")
	req.Header.Add(echo.HeaderContentType, "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	defer res.Body.Close()

	r = bytes.NewReader(body)
	req, err = http.NewRequest(http.MethodPost, u, r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAccept, "application/json")
	req.Header.Add(echo.HeaderContentType, "application/json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+replAccessToken)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()
}

func TestRevsDiff(t *testing.T) {
	assert.NotEmpty(t, replSharingID)
	assert.NotEmpty(t, replAccessToken)

	id1 := replDoctype + "/" + uuidv4()
	createShared(t, id1, []string{"1-1a", "2-1a", "3-1a"})
	id2 := replDoctype + "/" + uuidv4()
	createShared(t, id2, []string{"1-2a", "2-2a", "3-2a"})
	id3 := replDoctype + "/" + uuidv4()
	createShared(t, id3, []string{"1-3a", "2-3a", "3-3a"})
	id4 := replDoctype + "/" + uuidv4()
	createShared(t, id4, []string{"1-4a", "2-4a", "3-4a"})
	id5 := replDoctype + "/" + uuidv4()
	createShared(t, id5, []string{"1-5a", "2-5a", "3-5a"})
	id6 := replDoctype + "/" + uuidv4()

	body, _ := json.Marshal(sharing.Changes{
		id1: []string{"3-1a"},
		id2: []string{"2-2a"},
		id3: []string{"5-3b"},
		id4: []string{"2-4b", "2-4c", "4-4d"},
		id6: []string{"1-6b"},
	})
	r := bytes.NewReader(body)
	u := tsR.URL + "/sharings/" + replSharingID + "/revs_diff"
	req, err := http.NewRequest(http.MethodPost, u, r)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAccept, "application/json")
	req.Header.Add(echo.HeaderContentType, "application/json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+replAccessToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	defer res.Body.Close()

	missings := make(sharing.Missings)
	err = json.NewDecoder(res.Body).Decode(&missings)
	assert.NoError(t, err)

	// id1 is the same on both sides
	assert.NotContains(t, missings, id1)

	// id2 was updated on the target
	assert.NotContains(t, missings, id2)

	// id3 was updated on the source
	assert.Contains(t, missings, id3)
	assert.Equal(t, missings[id3].Missing, []string{"5-3b"})
	assert.Equal(t, missings[id3].PAs, []string{"3-3a"})

	// id4 is a conflict
	assert.Contains(t, missings, id4)
	assert.Equal(t, missings[id4].Missing, []string{"2-4b", "2-4c", "4-4d"})
	assert.Equal(t, missings[id4].PAs, []string{"1-4a", "3-4a"})

	// id5 has been created on the target
	assert.NotContains(t, missings, id5)

	// id6 has been created on the source
	assert.Contains(t, missings, id6)
	assert.Equal(t, missings[id6].Missing, []string{"1-6b"})
	assert.Empty(t, missings[id6].PAs)
}
