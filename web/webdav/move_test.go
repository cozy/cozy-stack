package webdav

import (
	"testing"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMove_RenameFile(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "rename-src.txt", []byte("hello"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/rename-src.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/rename-dst.txt").
		Expect().
		Status(201)

	// Old path should be gone.
	env.E.GET("/dav/files/rename-src.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)

	// New path should have the content.
	resp := env.E.GET("/dav/files/rename-dst.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "hello", resp.Body().Raw())
}

func TestMove_ReparentFile(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "reparent.txt", []byte("data"))

	// Create a target subdirectory via VFS.
	_, err := vfs.Mkdir(env.Inst.VFS(), "/subdir", nil)
	require.NoError(t, err)

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/reparent.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/subdir/reparent.txt").
		Expect().
		Status(201)

	// Old path gone.
	env.E.GET("/dav/files/reparent.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)

	// New path returns content.
	resp := env.E.GET("/dav/files/subdir/reparent.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "data", resp.Body().Raw())
}

func TestMove_RenameDir(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	_, err := vfs.Mkdir(env.Inst.VFS(), "/olddir", nil)
	require.NoError(t, err)

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/olddir").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/newdir").
		Expect().
		Status(201)

	// Old path should be 404 on PROPFIND.
	env.E.Request("PROPFIND", "/dav/files/olddir").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(404)

	// New path should respond to PROPFIND.
	env.E.Request("PROPFIND", "/dav/files/newdir").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Depth", "0").
		Expect().
		Status(207)
}

func TestMove_OverwriteT_ExistingDest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-ow.txt", []byte("source content"))
	seedFile(t, env.Inst, "dst-ow.txt", []byte("old dest content"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/src-ow.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-ow.txt").
		WithHeader("Overwrite", "T").
		Expect().
		Status(204)

	// Source should be gone.
	env.E.GET("/dav/files/src-ow.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)

	// Destination should have source's content.
	resp := env.E.GET("/dav/files/dst-ow.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "source content", resp.Body().Raw())
}

func TestMove_OverwriteAbsent_DefaultsToT(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-abs.txt", []byte("src"))
	seedFile(t, env.Inst, "dst-abs.txt", []byte("dst"))

	host := env.TS.Listener.Addr().String()

	// No Overwrite header — should default to T per RFC 4918.
	env.E.Request("MOVE", "/dav/files/src-abs.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-abs.txt").
		Expect().
		Status(204)

	// Source gone.
	env.E.GET("/dav/files/src-abs.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)

	// Destination has source content.
	resp := env.E.GET("/dav/files/dst-abs.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "src", resp.Body().Raw())
}

func TestMove_OverwriteF_ExistingDest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-f.txt", []byte("src"))
	seedFile(t, env.Inst, "dst-f.txt", []byte("dst"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/src-f.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-f.txt").
		WithHeader("Overwrite", "F").
		Expect().
		Status(412)
}

func TestMove_OverwriteF_NewDest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-fnew.txt", []byte("content"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/src-fnew.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-fnew.txt").
		WithHeader("Overwrite", "F").
		Expect().
		Status(201)

	resp := env.E.GET("/dav/files/dst-fnew.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "content", resp.Body().Raw())
}

func TestMove_DestInTrash(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "trash-move.txt", []byte("data"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("MOVE", "/dav/files/trash-move.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/.cozy_trash/trash-move.txt").
		Expect().
		Status(403)
}

func TestMove_MissingDestHeader(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "nodest.txt", []byte("data"))

	// MOVE without Destination header.
	env.E.Request("MOVE", "/dav/files/nodest.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(400)
}

func TestMove_DestParentMissing(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "orphan.txt", []byte("data"))

	host := env.TS.Listener.Addr().String()

	// Destination parent dir does not exist.
	env.E.Request("MOVE", "/dav/files/orphan.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/nonexistent/orphan.txt").
		Expect().
		Status(409)
}
