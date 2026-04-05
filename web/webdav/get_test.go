package webdav

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedFile creates a file at the given VFS relative path (e.g. "hello.txt")
// under the instance root with the given byte content. Helper is intentionally
// minimal — Phase 1 tests only seed root-level files.
func seedFile(t *testing.T, inst *instance.Instance, name string, content []byte) {
	t.Helper()
	fs := inst.VFS()
	doc, err := vfs.NewFileDoc(
		name, "", int64(len(content)), nil,
		"text/plain", "text", time.Now(),
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

func TestGet_File_ReturnsContent(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

	resp := env.E.GET("/dav/files/hello.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)

	resp.Header("Content-Length").Equal("14")
	etag := resp.Header("Etag").Raw()
	assert.NotEmpty(t, etag, "Etag header must be set")

	body := resp.Body().Raw()
	assert.Equal(t, "Hello, WebDAV!", body)
}

func TestHead_File_NoBody(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

	resp := env.E.HEAD("/dav/files/hello.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)

	resp.Header("Content-Length").Equal("14")
	assert.NotEmpty(t, resp.Header("Etag").Raw(), "Etag header must be set on HEAD")
	resp.Body().Empty()
}

func TestGet_File_RangeRequest(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

	resp := env.E.GET("/dav/files/hello.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Range", "bytes=0-4").
		Expect().
		Status(206)

	resp.Header("Content-Range").Equal("bytes 0-4/14")
	assert.Equal(t, "Hello", resp.Body().Raw())
}

func TestGet_Collection_Returns405(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	resp := env.E.GET("/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(405)

	allow := resp.Header("Allow").Raw()
	assert.Contains(t, allow, "OPTIONS")
	assert.Contains(t, allow, "PROPFIND")
	assert.Contains(t, allow, "HEAD")
}

func TestGet_Nonexistent_Returns404(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.GET("/dav/files/does-not-exist.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(404)
}

func TestGet_Unauthenticated_Returns401(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "hello.txt", []byte("Hello, WebDAV!"))

	resp := env.E.GET("/dav/files/hello.txt").
		Expect().
		Status(401)

	resp.Header("WWW-Authenticate").Equal(`Basic realm="Cozy"`)
}
