package files

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangesFilterStreamWithVFSLocked(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()

	dir, err := vfs.Mkdir(inst.VFS(), "/Shared", nil)
	require.NoError(t, err)
	results := &couchdb.ChangesResponse{
		LastSeq: "2-abc",
		Results: []couchdb.Change{{
			DocID: "file-id",
			Seq:   "2-abc",
			Doc: couchdb.JSONDoc{M: map[string]interface{}{
				"_id":    "file-id",
				"_rev":   "1-abc",
				"type":   "file",
				"dir_id": dir.ID(),
				"name":   "foo.txt",
			}},
		}},
	}

	// Another request holds the VFS lock while the feed is streamed
	mu := config.Lock().ReadWrite(inst, "vfs")
	require.NoError(t, mu.Lock())
	defer mu.Unlock()

	filter := &changesFilter{IncludePath: true}
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- filter.Stream(&buf, inst, results) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the changes feed is waiting for the VFS lock")
	}

	var feed struct {
		Results []couchdb.Change `json:"results"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &feed))
	require.Len(t, feed.Results, 1)
	assert.Equal(t, "/Shared/foo.txt", feed.Results[0].Doc.M["path"])
}

func TestChangesFilterSharedDriveChangeWithoutPath(t *testing.T) {
	filter := &changesFilter{
		IncludePath: true,
		SharedDir: &vfs.DirDoc{
			DocID:        "shared-dir-id",
			Fullpath:     "/Drives/Shared",
			ReferencedBy: []couchdb.DocReference{{Type: consts.Sharings, ID: "sharing-id"}},
		},
	}
	change := &couchdb.Change{
		DocID: "file-id",
		Seq:   "5-xyz",
		Doc: couchdb.JSONDoc{M: map[string]interface{}{
			"_id":    "file-id",
			"_rev":   "2-abc",
			"type":   "file",
			"dir_id": "missing",
		}},
	}

	got := filter.SharedDriveChange(change)

	require.NotNil(t, got)
	assert.True(t, got.Deleted)
	assert.Equal(t, "file-id", got.DocID)
	assert.NotContains(t, got.Doc.M, "driveId")
}
