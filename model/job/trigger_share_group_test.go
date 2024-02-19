package job

import (
	"encoding/json"
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShareGroupTrigger(t *testing.T) {
	trigger := &ShareGroupTrigger{}

	noGroup := &couchdb.JSONDoc{
		Type: consts.Contacts,
		M: map[string]interface{}{
			"_id":      "85507320-b157-013c-12d8-18c04daba326",
			"_rev":     "1-abcdef",
			"fullname": "Bob",
		},
	}
	msg := trigger.match(&realtime.Event{
		Doc:    noGroup,
		OldDoc: nil,
		Verb:   realtime.EventCreate,
	})
	require.Nil(t, msg)

	updatedName := noGroup.Clone().(*couchdb.JSONDoc)
	updatedName.M["fullname"] = "Bobby"
	updatedName.M["_rev"] = "2-abcdef"
	msg = trigger.match(&realtime.Event{
		Doc:    updatedName,
		OldDoc: noGroup,
		Verb:   realtime.EventUpdate,
	})
	require.Nil(t, msg)

	var groups map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{
    "groups": {
      "data": [
        {
          "_id": "id-friends",
          "_type": "io.cozy.contacts.groups"
        },
        {
          "_id": "id-football",
          "_type": "io.cozy.contacts.groups"
        }
      ]
    }
}`), &groups))
	addedInGroups := updatedName.Clone().(*couchdb.JSONDoc)
	addedInGroups.M["relationships"] = groups
	addedInGroups.M["_rev"] = "3-abcdef"
	msg = trigger.match(&realtime.Event{
		Doc:    addedInGroups,
		OldDoc: updatedName,
		Verb:   realtime.EventUpdate,
	})
	require.NotNil(t, msg)
	assert.Equal(t, msg.ContactID, "85507320-b157-013c-12d8-18c04daba326")
	assert.EqualValues(t, msg.GroupsAdded, []string{"id-friends", "id-football"})
	assert.Len(t, msg.GroupsRemoved, 0)

	var groups2 map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{
    "groups": {
      "data": [
        {
          "_id": "id-friends",
          "_type": "io.cozy.contacts.groups"
        }
      ]
    }
}`), &groups2))
	removedFromFootball := addedInGroups.Clone().(*couchdb.JSONDoc)
	removedFromFootball.M["relationships"] = groups2
	removedFromFootball.M["_rev"] = "4-abcdef"
	msg = trigger.match(&realtime.Event{
		Doc:    removedFromFootball,
		OldDoc: addedInGroups,
		Verb:   realtime.EventUpdate,
	})
	require.NotNil(t, msg)
	assert.Equal(t, msg.ContactID, "85507320-b157-013c-12d8-18c04daba326")
	assert.Len(t, msg.GroupsAdded, 0)
	assert.EqualValues(t, msg.GroupsRemoved, []string{"id-football"})

	deleted := &couchdb.JSONDoc{
		Type: consts.Contacts,
		M: map[string]interface{}{
			"_id":      removedFromFootball.ID(),
			"_rev":     "5-abcdef",
			"_deleted": true,
		},
	}
	msg = trigger.match(&realtime.Event{
		Doc:    deleted,
		OldDoc: removedFromFootball,
		Verb:   realtime.EventDelete,
	})
	require.NotNil(t, msg)
	assert.Equal(t, msg.ContactID, "85507320-b157-013c-12d8-18c04daba326")
	assert.Len(t, msg.GroupsAdded, 0)
	assert.EqualValues(t, msg.GroupsRemoved, []string{"id-friends"})
}
