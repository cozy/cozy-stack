package webdav

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelete_File(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "todelete.txt", []byte("bye"))

	// DELETE should soft-trash the file and return 204.
	env.E.DELETE("/dav/files/todelete.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(204)

	// Subsequent GET should return 404 (file moved to trash).
	env.E.GET("/dav/files/todelete.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)
}

func TestDelete_Directory(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	fs := env.Inst.VFS()

	// Create a directory with a child file.
	_, err := vfs.Mkdir(fs, "/testdir", nil)
	require.NoError(t, err)

	parent, err := fs.DirByPath("/testdir")
	require.NoError(t, err)

	doc, err := vfs.NewFileDoc(
		"child.txt", parent.ID(), 5, nil,
		"text/plain", "text", time.Now(),
		false, false, false, nil,
	)
	require.NoError(t, err)
	f, err := fs.CreateFile(doc, nil)
	require.NoError(t, err)
	_, err = f.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// DELETE the directory — should trash the entire tree.
	env.E.DELETE("/dav/files/testdir").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(204)

	// Child file should no longer be accessible.
	env.E.GET("/dav/files/testdir/child.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)
}

func TestDelete_NotFound(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.DELETE("/dav/files/ghost.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)
}

func TestDelete_InTrash(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	resp := env.E.DELETE("/dav/files/.cozy_trash/something").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(405)

	allow := resp.Header("Allow").Raw()
	assert.Equal(t, "PROPFIND, GET, HEAD, OPTIONS", allow)
}

func TestDelete_InTrashRoot(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	resp := env.E.DELETE("/dav/files/.cozy_trash").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(405)

	allow := resp.Header("Allow").Raw()
	assert.Equal(t, "PROPFIND, GET, HEAD, OPTIONS", allow)
}
