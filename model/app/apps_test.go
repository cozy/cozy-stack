package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindRoute(t *testing.T) {
	manifest := &WebappManifest{}
	manifest.Routes = make(Routes)
	manifest.Routes["/foo"] = Route{Folder: "/foo", Index: "index.html"}
	manifest.Routes["/foo/bar"] = Route{Folder: "/bar", Index: "index.html"}
	manifest.Routes["/foo/qux"] = Route{Folder: "/qux", Index: "index.html"}
	manifest.Routes["/public"] = Route{Folder: "/public", Index: "public.html", Public: true}
	manifest.Routes["/admin"] = Route{Folder: "/admin", Index: "admin.html"}
	manifest.Routes["/admin/special"] = Route{Folder: "/special", Index: "admin.html"}

	ctx, rest := manifest.FindRoute("/admin")
	assert.Equal(t, "/admin", ctx.Folder)
	assert.Equal(t, "admin.html", ctx.Index)
	assert.Equal(t, false, ctx.Public)
	assert.Equal(t, "", rest)

	ctx, rest = manifest.FindRoute("/public/")
	assert.Equal(t, "/public", ctx.Folder)
	assert.Equal(t, "public.html", ctx.Index)
	assert.Equal(t, true, ctx.Public)
	assert.Equal(t, "", rest)

	ctx, rest = manifest.FindRoute("/public")
	assert.Equal(t, "/public", ctx.Folder)
	assert.Equal(t, "", rest)

	ctx, rest = manifest.FindRoute("/public/app.js")
	assert.Equal(t, "/public", ctx.Folder)
	assert.Equal(t, "app.js", rest)

	ctx, rest = manifest.FindRoute("/foo/admin/special")
	assert.Equal(t, "/foo", ctx.Folder)
	assert.Equal(t, "admin/special", rest)

	ctx, rest = manifest.FindRoute("/admin/special/foo")
	assert.Equal(t, "/special", ctx.Folder)
	assert.Equal(t, "foo", rest)

	ctx, rest = manifest.FindRoute("/foo/bar.html")
	assert.Equal(t, "/foo", ctx.Folder)
	assert.Equal(t, "bar.html", rest)

	ctx, rest = manifest.FindRoute("/foo/baz")
	assert.Equal(t, "/foo", ctx.Folder)
	assert.Equal(t, "baz", rest)

	ctx, rest = manifest.FindRoute("/foo/bar")
	assert.Equal(t, "/bar", ctx.Folder)
	assert.Equal(t, "", rest)

	ctx, _ = manifest.FindRoute("/")
	assert.Equal(t, "", ctx.Folder)
}

func TestNoRegression217(t *testing.T) {
	var man WebappManifest
	man.Routes = make(Routes)
	man.Routes["/"] = Route{
		Folder: "/",
		Index:  "index.html",
		Public: false,
	}

	ctx, rest := man.FindRoute("/any/path")
	assert.Equal(t, "/", ctx.Folder)
	assert.Equal(t, "any/path", rest)
}

func TestFindIntent(t *testing.T) {
	var man WebappManifest
	found := man.FindIntent("PICK", "io.cozy.files")
	assert.Nil(t, found)

	man.Intents = []Intent{
		{
			Action: "PICK",
			Types:  []string{"io.cozy.contacts", "io.cozy.calendars"},
			Href:   "/pick",
		},
		{
			Action: "OPEN",
			Types:  []string{"io.cozy.files", "image/gif"},
			Href:   "/open",
		},
		{
			Action: "EDIT",
			Types:  []string{"image/*"},
			Href:   "/open",
		},
	}
	found = man.FindIntent("PICK", "io.cozy.files")
	assert.Nil(t, found)
	found = man.FindIntent("OPEN", "io.cozy.contacts")
	assert.Nil(t, found)
	found = man.FindIntent("PICK", "io.cozy.contacts")
	assert.NotNil(t, found)
	assert.Equal(t, "PICK", found.Action)
	found = man.FindIntent("OPEN", "io.cozy.files")
	assert.NotNil(t, found)
	assert.Equal(t, "OPEN", found.Action)
	found = man.FindIntent("open", "io.cozy.files")
	assert.NotNil(t, found)
	assert.Equal(t, "OPEN", found.Action)

	found = man.FindIntent("OPEN", "image/gif")
	assert.NotNil(t, found)
	assert.Equal(t, "OPEN", found.Action)
	found = man.FindIntent("EDIT", "image/gif")
	assert.NotNil(t, found)
	assert.Equal(t, "EDIT", found.Action)

	man.Intents = []Intent{
		{
			Action: "PICK",
			Href:   "/pick",
		},
	}
	found = man.FindIntent("PICK", "io.cozy.files")
	assert.Nil(t, found)
}
