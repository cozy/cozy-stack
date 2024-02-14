package contact

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindContacts(t *testing.T) {
	config.UseTestFile(t)
	instPrefix := prefixer.NewPrefixer(0, "cozy-test", "cozy-test")
	t.Cleanup(func() { _ = couchdb.DeleteDB(instPrefix, consts.Contacts) })

	g := NewGroup()
	g.SetID(uuid.Must(uuid.NewV7()).String())

	gaby := fmt.Sprintf(`{
  "address": [],
  "birthday": "",
  "birthplace": "",
  "company": "",
  "cozy": [],
  "cozyMetadata": {
    "createdAt": "2024-02-13T15:05:58.917Z",
    "createdByApp": "Contacts",
    "createdByAppVersion": "1.7.0",
    "doctypeVersion": 3,
    "metadataVersion": 1,
    "updatedAt": "2024-02-13T15:06:21.046Z",
    "updatedByApps": [
      {
        "date": "2024-02-13T15:06:21.046Z",
        "slug": "Contacts",
        "version": "1.7.0"
      }
    ]
  },
  "displayName": "Gaby",
  "email": [],
  "fullname": "Gaby",
  "gender": "female",
  "indexes": {
    "byFamilyNameGivenNameEmailCozyUrl": "gaby"
  },
  "jobTitle": "",
  "metadata": {
    "cozy": true,
    "version": 1
  },
  "name": {
    "givenName": "Gaby"
  },
  "note": "",
  "phone": [],
  "relationships": {
    "groups": {
      "data": [
        {
          "_id": "%s",
          "_type": "io.cozy.contacts.groups"
        }
      ]
    }
  }
}`, g.ID())

	doc := couchdb.JSONDoc{Type: consts.Contacts, M: make(map[string]interface{})}
	require.NoError(t, json.Unmarshal([]byte(gaby), &doc.M))
	require.NoError(t, couchdb.CreateDoc(instPrefix, &doc))

	contacts, err := g.FindContacts(instPrefix)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	assert.Equal(t, contacts[0].PrimaryName(), "Gaby")
}
