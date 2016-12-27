package apps

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindContext(t *testing.T) {
	manifest := &Manifest{}
	manifest.Contexts = make(Contexts)
	manifest.Contexts["/foo"] = Context{Folder: "/foo", Index: "index.html"}
	manifest.Contexts["/foo/bar"] = Context{Folder: "/bar", Index: "index.html"}
	manifest.Contexts["/foo/qux"] = Context{Folder: "/qux", Index: "index.html"}
	manifest.Contexts["/public"] = Context{Folder: "/public", Index: "public.html", Public: true}
	manifest.Contexts["/admin"] = Context{Folder: "/admin", Index: "admin.html"}
	manifest.Contexts["/admin/special"] = Context{Folder: "/special", Index: "admin.html"}

	ctx, rest := manifest.FindContext("/admin")
	assert.Equal(t, "/admin", ctx.Folder)
	assert.Equal(t, "admin.html", ctx.Index)
	assert.Equal(t, false, ctx.Public)
	assert.Equal(t, "", rest)

	ctx, rest = manifest.FindContext("/public/")
	assert.Equal(t, "/public", ctx.Folder)
	assert.Equal(t, "public.html", ctx.Index)
	assert.Equal(t, true, ctx.Public)
	assert.Equal(t, "", rest)

	ctx, rest = manifest.FindContext("/public")
	assert.Equal(t, "/public", ctx.Folder)
	assert.Equal(t, "", rest)

	ctx, rest = manifest.FindContext("/public/app.js")
	assert.Equal(t, "/public", ctx.Folder)
	assert.Equal(t, "app.js", rest)

	ctx, rest = manifest.FindContext("/foo/admin/special")
	assert.Equal(t, "/foo", ctx.Folder)
	assert.Equal(t, "admin/special", rest)

	ctx, rest = manifest.FindContext("/admin/special/foo")
	assert.Equal(t, "/special", ctx.Folder)
	assert.Equal(t, "foo", rest)

	ctx, rest = manifest.FindContext("/foo/bar.html")
	assert.Equal(t, "/foo", ctx.Folder)
	assert.Equal(t, "bar.html", rest)

	ctx, rest = manifest.FindContext("/foo/baz")
	assert.Equal(t, "/foo", ctx.Folder)
	assert.Equal(t, "baz", rest)

	ctx, rest = manifest.FindContext("/foo/bar")
	assert.Equal(t, "/bar", ctx.Folder)
	assert.Equal(t, "", rest)

	ctx, _ = manifest.FindContext("/")
	assert.Equal(t, "", ctx.Folder)
}
