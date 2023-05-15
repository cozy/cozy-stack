package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ServeContent(t *testing.T) {
	t.Run("with other than a HEAD method", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://localhost/foo/bar", nil)
		w := httptest.NewRecorder()

		ServeContent(w, r, "application/json", 10, strings.NewReader("foobar"))

		assert.Equal(t, http.StatusOK, w.Result().StatusCode)
		assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))
		assert.Equal(t, "foobar", w.Body.String())

		// Not that the length 10 as the `size` parameter and not `len("foobar")`
		assert.Equal(t, "10", w.Result().Header.Get("Content-Length"))
	})

	t.Run("with the HEAD method", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodHead, "http://localhost/foo/bar", nil)
		w := httptest.NewRecorder()

		ServeContent(w, r, "application/json", 10, strings.NewReader("foobar"))

		assert.Equal(t, http.StatusOK, w.Result().StatusCode)
		assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))

		// With HEAD we don't setup a body
		assert.Empty(t, w.Body.String())

		// Not that the length 10 as the `size` parameter and not `len("foobar")`
		assert.Equal(t, "10", w.Result().Header.Get("Content-Length"))
	})
}

func Test_CheckPreconditions_with_matching_etag(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://localhost/foo/bar", nil)
	w := httptest.NewRecorder()

	r.Header.Set("If-None-Match", `"some-etag"`)

	done := CheckPreconditions(w, r, `"some-etag"`)

	assert.True(t, done)
	assert.Equal(t, http.StatusNotModified, w.Result().StatusCode)
}

func Test_CheckPreconditions_with_no_etag(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://localhost/foo/bar", nil)
	w := httptest.NewRecorder()

	done := CheckPreconditions(w, r, `"some-etag"`)

	assert.False(t, done)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func Test_CheckPreconditions_with_non_matching_etag(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://localhost/foo/bar", nil)
	w := httptest.NewRecorder()

	r.Header.Set("If-None-Match", `"some-etag"`)

	done := CheckPreconditions(w, r, `"some-other-etag"`)

	assert.False(t, done)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func Test_checkIfNoneMatch(t *testing.T) {
	var tests = []struct {
		Name        string
		IfNoneMatch string
		Etag        string
		Match       bool
	}{
		{
			Name:        "strong inm with string etag",
			IfNoneMatch: `"some-etag"`,
			Etag:        `"some-etag"`,
			Match:       true,
		},
		{
			Name:        "weak inm with strong etag",
			IfNoneMatch: `W/"some-etag"`,
			Etag:        `"some-etag"`,
			Match:       true,
		},
		{
			Name:        "strong inm with weak etag",
			IfNoneMatch: `"some-etag"`,
			Etag:        `W/"some-etag"`,
			Match:       true,
		},
		{
			Name:        "multiple inm values match etag",
			IfNoneMatch: `"first-etag","second-etag"`,
			Etag:        `"second-etag"`,
			Match:       true,
		},
		// TODO: This doesn't pass with the current implem, propose a new implem
		// with working with this case.
		// {
		// 	Name:        "multiple inm values are trimmed",
		// 	IfNoneMatch: `"first-etag" , "second-etag"`,
		// 	Etag:        `"second-etag"`,
		// 	Match:       true,
		// },
		{
			Name:        "inm is trimmed",
			IfNoneMatch: `  "second-etag"\t`,
			Etag:        `"second-etag"`,
			Match:       true,
		},
		{
			Name:        "multiple inm values not matching etag",
			IfNoneMatch: `"first-etag","second-etag`,
			Etag:        `"third-etag"`,
			Match:       false,
		},
		{
			Name:        "inm not matching with etag",
			IfNoneMatch: `"some-etag"`,
			Etag:        `"some-other-etag"`,
			Match:       false,
		},
		{
			Name:        "* match every etag",
			IfNoneMatch: `*`,
			Etag:        `"some--etag"`,
			Match:       true,
		},
		{
			Name:        "Invalid etag quote",
			IfNoneMatch: `"some-etag`,
			Etag:        `W/"some-etag"`,
			Match:       false,
		},
		{
			Name:        "Invalid etag quote",
			IfNoneMatch: `some-etag`,
			Etag:        `W/"some-etag"`,
			Match:       false,
		},
		{
			Name:        "Invalid etag quote",
			IfNoneMatch: `"some-etag"`,
			Etag:        `some-etag`,
			Match:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			assert.Equal(t, test.Match, checkIfNoneMatch(test.IfNoneMatch, test.Etag))
		})
	}
}
