package webdav

import (
	"testing"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/stretchr/testify/require"
)

func TestMkcol_CreateDir(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// MKCOL to create a new directory — expect 201 Created.
	env.E.Request("MKCOL", "/dav/files/newdir").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(201)

	// Verify the directory exists via PROPFIND Depth:0.
	env.E.Request("PROPFIND", "/dav/files/newdir").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(207)
}

func TestMkcol_AlreadyExists(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// Pre-create the directory via VFS.
	_, err := vfs.Mkdir(env.Inst.VFS(), "/existingdir", nil)
	require.NoError(t, err)

	// MKCOL on an already-existing path — expect 405 Method Not Allowed.
	env.E.Request("MKCOL", "/dav/files/existingdir").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(405)
}

func TestMkcol_MissingParent(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// MKCOL where parent does not exist — expect 409 Conflict.
	env.E.Request("MKCOL", "/dav/files/nonexistent/child").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(409)
}

func TestMkcol_WithBody(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// MKCOL with a request body — expect 415 Unsupported Media Type.
	env.E.Request("MKCOL", "/dav/files/bodydir").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithBytes([]byte("<xml>stuff</xml>")).
		Expect().
		Status(415)
}

func TestMkcol_InTrash(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// MKCOL inside .cozy_trash — expect 403 Forbidden.
	env.E.Request("MKCOL", "/dav/files/.cozy_trash/newdir").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(403)
}
