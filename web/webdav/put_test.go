package webdav

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPut_CreateNewFile(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.PUT("/dav/files/newfile.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithBytes([]byte("hello")).
		Expect().
		Status(201)

	// Verify via GET that the file exists with correct content.
	resp := env.E.GET("/dav/files/newfile.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "hello", resp.Body().Raw())
}

func TestPut_OverwriteExistingFile(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "existing.txt", []byte("old content"))

	env.E.PUT("/dav/files/existing.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithBytes([]byte("new content")).
		Expect().
		Status(204)

	// Verify the content was overwritten.
	resp := env.E.GET("/dav/files/existing.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	assert.Equal(t, "new content", resp.Body().Raw())
}

func TestPut_ZeroByte(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.PUT("/dav/files/empty.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithBytes([]byte{}).
		Expect().
		Status(201)

	// Verify empty file exists.
	resp := env.E.GET("/dav/files/empty.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	resp.Header("Content-Length").Equal("0")
	assert.Equal(t, "", resp.Body().Raw())
}

func TestPut_MissingParent(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.PUT("/dav/files/nonexistent-dir/file.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithBytes([]byte("data")).
		Expect().
		Status(409)
}

func TestPut_IfMatch_Matches(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "match.txt", []byte("original"))

	// Get the current ETag.
	getResp := env.E.GET("/dav/files/match.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(200)
	etag := getResp.Header("Etag").Raw()
	require.NotEmpty(t, etag)

	// PUT with matching If-Match should succeed (overwrite).
	env.E.PUT("/dav/files/match.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("If-Match", etag).
		WithBytes([]byte("updated")).
		Expect().
		Status(204)
}

func TestPut_IfMatch_Mismatch(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "mismatch.txt", []byte("original"))

	env.E.PUT("/dav/files/mismatch.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("If-Match", `"wrong-etag"`).
		WithBytes([]byte("updated")).
		Expect().
		Status(412)
}

func TestPut_IfNoneMatch_Star_Existing(t *testing.T) {
	env := newWebdavTestEnv(t, nil)
	seedFile(t, env.Inst, "existing2.txt", []byte("data"))

	env.E.PUT("/dav/files/existing2.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("If-None-Match", "*").
		WithBytes([]byte("new data")).
		Expect().
		Status(412)
}

func TestPut_IfNoneMatch_Star_New(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.PUT("/dav/files/brand-new.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("If-None-Match", "*").
		WithBytes([]byte("fresh")).
		Expect().
		Status(201)
}

func TestPut_InTrash(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.PUT("/dav/files/.cozy_trash/foo.txt").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithBytes([]byte("data")).
		Expect().
		Status(403)
}
