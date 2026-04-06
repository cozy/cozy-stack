package webdav

// End-to-end integration tests driving the full WebDAV write surface via a
// real github.com/studio-b12/gowebdav client. Each subtest exercises one write
// operation (PUT, DELETE, MKCOL, MOVE) and verifies both the HTTP result and
// the observable VFS state through subsequent gowebdav read calls.
//
// These tests complement TestE2E_GowebdavClient (Phase 1 read-only surface)
// and together they form the complete E2E gate for Phase 2.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/studio-b12/gowebdav"
)

// TestE2E_WriteOperations exercises all Phase 2 write methods through the
// gowebdav client library against a live test Cozy instance.
func TestE2E_WriteOperations(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// gowebdav: token passed as Basic-auth password (empty username is the
	// Cozy convention — see plan 01-05 auth middleware).
	client := gowebdav.NewClient(env.TS.URL+"/dav/files", "", env.Token)

	// --- PUT ---

	t.Run("PUT_CreateFile", func(t *testing.T) {
		err := client.Write("newfile.txt", []byte("gowebdav content"), 0644)
		require.NoError(t, err)

		data, err := client.Read("newfile.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("gowebdav content"), data)
	})

	t.Run("PUT_OverwriteFile", func(t *testing.T) {
		err := client.Write("overwrite.txt", []byte("v1"), 0644)
		require.NoError(t, err)

		err = client.Write("overwrite.txt", []byte("v2"), 0644)
		require.NoError(t, err)

		data, err := client.Read("overwrite.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("v2"), data)
	})

	// --- MKCOL ---

	t.Run("MKCOL_CreateDirectory", func(t *testing.T) {
		err := client.Mkdir("testdir", 0755)
		require.NoError(t, err)

		info, err := client.Stat("testdir")
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("PUT_FileInSubdir", func(t *testing.T) {
		// Depends on MKCOL having created testdir above.
		err := client.Write("testdir/subfile.txt", []byte("in subdir"), 0644)
		require.NoError(t, err)

		data, err := client.Read("testdir/subfile.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("in subdir"), data)
	})

	// --- MOVE ---

	t.Run("MOVE_RenameFile", func(t *testing.T) {
		err := client.Write("moveme.txt", []byte("move content"), 0644)
		require.NoError(t, err)

		err = client.Rename("moveme.txt", "moved.txt", true)
		require.NoError(t, err)

		// Old path gone.
		_, err = client.Read("moveme.txt")
		assert.Error(t, err)

		// New path has correct content.
		data, err := client.Read("moved.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("move content"), data)
	})

	t.Run("MOVE_ReparentFile", func(t *testing.T) {
		err := client.Mkdir("movedir", 0755)
		require.NoError(t, err)

		err = client.Write("reparent.txt", []byte("reparent me"), 0644)
		require.NoError(t, err)

		err = client.Rename("reparent.txt", "movedir/reparent.txt", true)
		require.NoError(t, err)

		data, err := client.Read("movedir/reparent.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("reparent me"), data)
	})

	// --- DELETE ---

	t.Run("DELETE_File", func(t *testing.T) {
		err := client.Write("deleteme.txt", []byte("bye"), 0644)
		require.NoError(t, err)

		err = client.Remove("deleteme.txt")
		require.NoError(t, err)

		_, err = client.Read("deleteme.txt")
		assert.Error(t, err) // 404 — file is in trash
	})

	t.Run("DELETE_Directory", func(t *testing.T) {
		err := client.Mkdir("deldir", 0755)
		require.NoError(t, err)

		err = client.Write("deldir/child.txt", []byte("child"), 0644)
		require.NoError(t, err)

		err = client.Remove("deldir")
		require.NoError(t, err)

		_, err = client.Stat("deldir")
		assert.Error(t, err) // 404 — directory is in trash
	})

	// --- Composite flow ---

	t.Run("OnlyOffice_OpenEditSave_Flow", func(t *testing.T) {
		// Simulate OnlyOffice mobile flow: create -> read -> overwrite -> read.
		err := client.Write("document.odt", []byte("original"), 0644)
		require.NoError(t, err)

		data, err := client.Read("document.odt")
		require.NoError(t, err)
		assert.Equal(t, []byte("original"), data)

		// Edit and save back.
		err = client.Write("document.odt", []byte("edited content"), 0644)
		require.NoError(t, err)

		data, err = client.Read("document.odt")
		require.NoError(t, err)
		assert.Equal(t, []byte("edited content"), data)
	})
}
