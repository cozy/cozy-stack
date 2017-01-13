package apps

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
)

func TestFindRoute(t *testing.T) {
	manifest := &Manifest{}
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
	var man Manifest
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

func TestBuildToken(t *testing.T) {
	manifest := &Manifest{
		Slug: "my-app",
	}
	i := &instance.Instance{
		Domain:        "test-ctx-token.example.com",
		SessionSecret: crypto.GenerateRandomBytes(64),
	}

	tokenString := manifest.BuildToken(i)
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return i.SessionSecret, nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "app", claims["aud"])
	assert.Equal(t, "test-ctx-token.example.com", claims["iss"])
	assert.Equal(t, "my-app", claims["sub"])
}
