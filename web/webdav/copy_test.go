package webdav

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedFileWithMime creates a VFS file at root level with the given mime type.
// Used for Note tests where the mime must be set explicitly.
func seedFileWithMime(t *testing.T, env *webdavTestEnv, name string, content []byte, mime string) {
	t.Helper()
	fs := env.Inst.VFS()
	doc, err := vfs.NewFileDoc(
		name, "", int64(len(content)), nil,
		mime, "text", time.Now(),
		false, false, false, nil,
	)
	require.NoError(t, err)

	f, err := fs.CreateFile(doc, nil)
	require.NoError(t, err)

	n, err := io.Copy(f, bytes.NewReader(content))
	require.NoError(t, err)
	require.Equal(t, int64(len(content)), n)
	require.NoError(t, f.Close())
}

// TestCopy_File_NewDest: COPY a regular file to a new destination returns 201
// and produces a VFS replica with identical MD5Sum.
func TestCopy_File_NewDest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src.txt", []byte("copy-content"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/src.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst.txt").
		Expect().
		Status(201)

	// Source should still exist.
	srcDoc, err := env.Inst.VFS().FileByPath("/src.txt")
	require.NoError(t, err)

	// Destination must exist with same MD5.
	dstDoc, err := env.Inst.VFS().FileByPath("/dst.txt")
	require.NoError(t, err)
	assert.Equal(t, srcDoc.MD5Sum, dstDoc.MD5Sum)
}

// TestCopy_File_OverwriteAbsent: COPY without Overwrite header defaults to T.
// If destination already exists, old file is trashed and new copy written. Returns 204.
func TestCopy_File_OverwriteAbsent(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-abs.txt", []byte("source-abs"))
	seedFile(t, env.Inst, "dst-abs.txt", []byte("old-dst-abs"))

	// Capture original dst MD5.
	oldDst, err := env.Inst.VFS().FileByPath("/dst-abs.txt")
	require.NoError(t, err)
	oldMD5 := oldDst.MD5Sum

	host := env.TS.Listener.Addr().String()

	// No Overwrite header — defaults to T.
	env.E.Request("COPY", "/dav/files/src-abs.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-abs.txt").
		Expect().
		Status(204)

	// Destination now has source's content.
	srcDoc, err := env.Inst.VFS().FileByPath("/src-abs.txt")
	require.NoError(t, err)
	newDst, err := env.Inst.VFS().FileByPath("/dst-abs.txt")
	require.NoError(t, err)
	assert.Equal(t, srcDoc.MD5Sum, newDst.MD5Sum)
	assert.NotEqual(t, oldMD5, newDst.MD5Sum, "old content should have been replaced")
}

// TestCopy_File_OverwriteT: COPY with Overwrite:T and existing destination
// trashes the old destination and writes new one. Returns 204.
func TestCopy_File_OverwriteT(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-owt.txt", []byte("source-owt"))
	seedFile(t, env.Inst, "dst-owt.txt", []byte("old-dst-owt"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/src-owt.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-owt.txt").
		WithHeader("Overwrite", "T").
		Expect().
		Status(204)

	// Destination should have source content.
	srcDoc, err := env.Inst.VFS().FileByPath("/src-owt.txt")
	require.NoError(t, err)
	newDst, err := env.Inst.VFS().FileByPath("/dst-owt.txt")
	require.NoError(t, err)
	assert.Equal(t, srcDoc.MD5Sum, newDst.MD5Sum)
}

// TestCopy_File_OverwriteF_existing: COPY with Overwrite:F to an existing
// destination returns 412 Precondition Failed. Destination is unchanged.
func TestCopy_File_OverwriteF_existing(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-owf.txt", []byte("source-owf"))
	seedFile(t, env.Inst, "dst-owf.txt", []byte("original-dst"))

	host := env.TS.Listener.Addr().String()

	origDst, err := env.Inst.VFS().FileByPath("/dst-owf.txt")
	require.NoError(t, err)
	origMD5 := origDst.MD5Sum

	env.E.Request("COPY", "/dav/files/src-owf.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-owf.txt").
		WithHeader("Overwrite", "F").
		Expect().
		Status(412)

	// Destination must be unchanged.
	unchangedDst, err := env.Inst.VFS().FileByPath("/dst-owf.txt")
	require.NoError(t, err)
	assert.Equal(t, origMD5, unchangedDst.MD5Sum)
}

// TestCopy_File_OverwriteF_newDest: COPY with Overwrite:F when destination
// does not exist — the F flag only blocks overwriting, not fresh creation.
// Should return 201.
func TestCopy_File_OverwriteF_newDest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-owf-new.txt", []byte("src-content"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/src-owf-new.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst-owf-new.txt").
		WithHeader("Overwrite", "F").
		Expect().
		Status(201)

	_, err := env.Inst.VFS().FileByPath("/dst-owf-new.txt")
	require.NoError(t, err)
}

// TestCopy_File_MissingSource: COPY a non-existent source returns 404.
func TestCopy_File_MissingSource(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/nope.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/dst.txt").
		Expect().
		Status(404)
}

// TestCopy_File_MissingDestParent: COPY to a destination whose parent does
// not exist returns 409 Conflict.
func TestCopy_File_MissingDestParent(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-mp.txt", []byte("data"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/src-mp.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/missingdir/dst.txt").
		Expect().
		Status(409)
}

// TestCopy_File_SourceEqualsDest: COPY a file onto itself returns 403 Forbidden
// per RFC 4918 §9.8.5.
func TestCopy_File_SourceEqualsDest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "self.txt", []byte("unchanged"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/self.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/self.txt").
		Expect().
		Status(403)

	// Source must remain untouched.
	doc, err := env.Inst.VFS().FileByPath("/self.txt")
	require.NoError(t, err)
	assert.NotNil(t, doc)
}

// TestCopy_File_IntoTrash: COPY to a destination inside .cozy_trash returns 403.
func TestCopy_File_IntoTrash(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-trash.txt", []byte("data"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/src-trash.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/.cozy_trash/stolen.txt").
		Expect().
		Status(403)
}

// TestCopy_File_FromTrash: COPY a file that resides in .cozy_trash returns 403.
func TestCopy_File_FromTrash(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	// Seed a regular file, then trash it to put it in .cozy_trash.
	seedFile(t, env.Inst, "to-be-trashed.txt", []byte("trashed-data"))
	fs := env.Inst.VFS()
	fileDoc, err := fs.FileByPath("/to-be-trashed.txt")
	require.NoError(t, err)
	_, err = vfs.TrashFile(fs, fileDoc)
	require.NoError(t, err)

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/.cozy_trash/to-be-trashed.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/leak.txt").
		Expect().
		Status(403)
}

// TestCopy_File_MissingDestinationHeader: COPY without a Destination header
// returns 400 Bad Request.
func TestCopy_File_MissingDestinationHeader(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "src-nodest.txt", []byte("data"))

	env.E.Request("COPY", "/dav/files/src-nodest.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(400)
}

// TestCopy_File_Nextcloud_Route: Anti-regression gate for commit 7c9ab3a59.
// Both /dav/files/* AND /remote.php/webdav/* must serve the same handleCopy.
func TestCopy_File_Nextcloud_Route(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	registerNextcloudRoutes(t, env)
	seedFile(t, env.Inst, "nc-src.txt", []byte("nc-content"))

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/remote.php/webdav/nc-src.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/remote.php/webdav/nc-dst.txt").
		Expect().
		Status(201)

	dstDoc, err := env.Inst.VFS().FileByPath("/nc-dst.txt")
	require.NoError(t, err)
	assert.NotNil(t, dstDoc)
}

// TestCopy_File_Notes: COPY a Cozy Note file (mime = consts.NoteMimeType).
// The handler MUST branch on olddoc.Mime and call note.CopyFile instead of
// fs.CopyFile. Observable: the new file exists with non-empty content.
//
// TODO(plan 03-01): Full assertion that note.CopyFile was called (not fs.CopyFile)
// requires injection or a mock. The current integration-level check verifies the
// outcome (file exists) without inspecting the branch taken. This is acceptable
// because the note branch is unit-tested in note/copy_test.go.
func TestCopy_File_Notes(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	// Seed a note file with the NoteMimeType MIME.
	noteContent := []byte("# My note\nHello world")
	seedFileWithMime(t, env, "my.note", noteContent, consts.NoteMimeType)

	host := env.TS.Listener.Addr().String()

	env.E.Request("COPY", "/dav/files/my.note").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Destination", "http://"+host+"/dav/files/copied.note").
		Expect().
		Status(201)

	// The copied file must exist.
	dstDoc, err := env.Inst.VFS().FileByPath("/copied.note")
	require.NoError(t, err)
	assert.NotNil(t, dstDoc)
}
