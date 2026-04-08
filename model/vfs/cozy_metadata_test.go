package vfs

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/stretchr/testify/assert"
)

func TestUpdatedByApp(t *testing.T) {
	fcm := NewCozyMetadata("alice.cozy.localhost")
	entry := &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "alice.cozy.localhost",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 1)
	assert.Equal(t, "drive", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[0].Instance)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "photo",
		Date:     time.Now(),
		Instance: "alice.cozy.localhost",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 2)
	assert.Equal(t, "photo", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[1].Instance)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "bob.cozy.localhost",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "drive", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "bob.cozy.localhost", fcm.UpdatedByApps[2].Instance)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "alice.cozy.localhost",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "photo", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[0].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "bob.cozy.localhost", fcm.UpdatedByApps[1].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[2].Instance)
	assert.Equal(t, entry.Date, fcm.UpdatedByApps[2].Date)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "alice.cozy.localhost",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "photo", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[0].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "bob.cozy.localhost", fcm.UpdatedByApps[1].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[2].Instance)
	assert.Equal(t, entry.Date, fcm.UpdatedByApps[2].Date)

	one := *entry
	two := *entry
	three := *entry
	fcm.UpdatedByApps = append(fcm.UpdatedByApps, &one, &two, &three)
	entry = &metadata.UpdatedByAppEntry{
		Slug:     "photo",
		Date:     time.Now(),
		Instance: "alice.cozy.localhost",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "drive", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "bob.cozy.localhost", fcm.UpdatedByApps[0].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[1].Instance)
	assert.Equal(t, "photo", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "alice.cozy.localhost", fcm.UpdatedByApps[2].Instance)
	assert.Equal(t, entry.Date, fcm.UpdatedByApps[2].Date)
}

func TestFilesCozyMetadataCloneAndToJSONDoc(t *testing.T) {
	trashedAt := time.Now().UTC().Round(0)
	fcm := NewCozyMetadata("alice.cozy.localhost")
	fcm.TrashedAt = &trashedAt
	fcm.TrashedBy = &TrashedByEntry{
		Kind:        TrashedByKindMember,
		DisplayName: "Alice",
		Domain:      "alice.cozy.localhost",
	}

	cloned := fcm.Clone()
	if assert.NotNil(t, cloned.TrashedAt) {
		assert.Equal(t, trashedAt, *cloned.TrashedAt)
	}
	if assert.NotNil(t, cloned.TrashedBy) {
		assert.Equal(t, fcm.TrashedBy, cloned.TrashedBy)
		cloned.TrashedBy.DisplayName = "Bob"
		assert.Equal(t, "Alice", fcm.TrashedBy.DisplayName)
	}

	doc := fcm.ToJSONDoc()
	assert.Equal(t, trashedAt, doc["trashedAt"])
	trashedBy, ok := doc["trashedBy"].(map[string]interface{})
	if assert.True(t, ok) {
		assert.Equal(t, TrashedByKindMember, trashedBy["kind"])
		assert.Equal(t, "Alice", trashedBy["displayName"])
		assert.Equal(t, "alice.cozy.localhost", trashedBy["domain"])
	}
}
