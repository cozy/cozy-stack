package vfs

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/stretchr/testify/assert"
)

func TestUpdatedByApp(t *testing.T) {
	fcm := NewCozyMetadata("alice.cozy.tools")
	entry := &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "alice.cozy.tools",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 1)
	assert.Equal(t, "drive", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "alice.cozy.tools", fcm.UpdatedByApps[0].Instance)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "photo",
		Date:     time.Now(),
		Instance: "alice.cozy.tools",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 2)
	assert.Equal(t, "photo", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "alice.cozy.tools", fcm.UpdatedByApps[1].Instance)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "bob.cozy.tools",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "drive", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "bob.cozy.tools", fcm.UpdatedByApps[2].Instance)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "alice.cozy.tools",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "photo", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "alice.cozy.tools", fcm.UpdatedByApps[0].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "bob.cozy.tools", fcm.UpdatedByApps[1].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "alice.cozy.tools", fcm.UpdatedByApps[2].Instance)
	assert.Equal(t, entry.Date, fcm.UpdatedByApps[2].Date)

	entry = &metadata.UpdatedByAppEntry{
		Slug:     "drive",
		Date:     time.Now(),
		Instance: "alice.cozy.tools",
	}
	fcm.UpdatedByApp(entry)
	assert.Len(t, fcm.UpdatedByApps, 3)
	assert.Equal(t, "photo", fcm.UpdatedByApps[0].Slug)
	assert.Equal(t, "alice.cozy.tools", fcm.UpdatedByApps[0].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[1].Slug)
	assert.Equal(t, "bob.cozy.tools", fcm.UpdatedByApps[1].Instance)
	assert.Equal(t, "drive", fcm.UpdatedByApps[2].Slug)
	assert.Equal(t, "alice.cozy.tools", fcm.UpdatedByApps[2].Instance)
	assert.Equal(t, entry.Date, fcm.UpdatedByApps[2].Date)
}
