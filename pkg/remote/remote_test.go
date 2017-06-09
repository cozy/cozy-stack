package remote

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

const doctype = "org.example.request"

func TestParseRawRequest(t *testing.T) {
	var raw = `GET`
	_, err := ParseRawRequest(doctype, raw)
	assert.Equal(t, ErrInvalidRequest, err)

	raw = `PUT https://example.org/`
	_, err = ParseRawRequest(doctype, raw)
	assert.Equal(t, ErrInvalidRequest, err)

	raw = `GET ftp://example.org/`
	_, err = ParseRawRequest(doctype, raw)
	assert.Equal(t, ErrInvalidRequest, err)

	raw = `GET /etc/hosts`
	_, err = ParseRawRequest(doctype, raw)
	assert.Equal(t, ErrInvalidRequest, err)

	raw = `GET https://example.org/
Foo`
	_, err = ParseRawRequest(doctype, raw)
	assert.Equal(t, ErrInvalidRequest, err)

	// Allow a trailing \n after headers on GET
	raw = `GET https://www.wikidata.org/wiki/Special:EntityData/{{entity}}.json
Content-Type: application/json
`
	_, err = ParseRawRequest(doctype, raw)
	assert.NoError(t, err)

	raw = `GET https://www.wikidata.org/wiki/Special:EntityData/{{entity}}.json`
	r1, err := ParseRawRequest(doctype, raw)
	assert.NoError(t, err)
	assert.Equal(t, "GET", r1.Verb)
	assert.Equal(t, "https", r1.URL.Scheme)
	assert.Equal(t, "www.wikidata.org", r1.URL.Host)
	assert.Equal(t, "/wiki/Special:EntityData/{{entity}}.json", r1.URL.Path)
	assert.Equal(t, "", r1.URL.RawQuery)

	raw = `POST https://www.wikidata.org/w/api.php?action=wbsearchentities&search={{q}}&language=en&format=json
Accept-Language: fr-FR,en
Content-Type: application/json

one={{one}}
two={{two}}`
	r2, err := ParseRawRequest(doctype, raw)
	assert.NoError(t, err)
	assert.Equal(t, "POST", r2.Verb)
	assert.Equal(t, "https", r2.URL.Scheme)
	assert.Equal(t, "www.wikidata.org", r2.URL.Host)
	assert.Equal(t, "/w/api.php", r2.URL.Path)
	assert.Equal(t, "action=wbsearchentities&search={{q}}&language=en&format=json", r2.URL.RawQuery)
	assert.Equal(t, "fr-FR,en", r2.Headers["Accept-Language"])
	assert.Equal(t, "application/json", r2.Headers["Content-Type"])
	assert.Equal(t, `one={{one}}
two={{two}}`, r2.Body)
}

func TestExtractVariablesGET(t *testing.T) {
	u, err := url.Parse("https://example.org/foo?one=un&two=deux")
	assert.NoError(t, err)
	in := &http.Request{URL: u}
	vars, err := ExtractVariables("GET", in)
	assert.NoError(t, err)
	assert.Equal(t, "un", vars["one"])
	assert.Equal(t, "deux", vars["two"])
}

func TestExtractVariablesPOST(t *testing.T) {
	body := bytes.NewReader([]byte(`{"one": "un", "two": "deux"}`))
	in := httptest.NewRequest("POST", "https://example.com/bar", body)
	vars, err := ExtractVariables("POST", in)
	assert.NoError(t, err)
	assert.Equal(t, "un", vars["one"])
	assert.Equal(t, "deux", vars["two"])

	body = bytes.NewReader([]byte(`one=un&two=deux`))
	in = httptest.NewRequest("POST", "https://example.com/bar", body)
	_, err = ExtractVariables("POST", in)
	assert.Error(t, err)
}

func TestInjectVariables(t *testing.T) {
	raw := `POST https://example.org/foo/{{bar}}?q={{q}}
Content-Type: application/json
Accept-Language: {{lang}},en

{ "one": "{{one}}", "two": "{{two}}" }`
	r, err := ParseRawRequest(doctype, raw)
	assert.NoError(t, err)

	vars := map[string]string{
		"bar":  "baz",
		"q":    "Q42",
		"lang": "fr-FR",
		"one":  "un",
		"two":  "deux",
	}

	InjectVariables(r, vars)
	assert.Equal(t, "POST", r.Verb)
	assert.Equal(t, "https", r.URL.Scheme)
	assert.Equal(t, "example.org", r.URL.Host)
	assert.Equal(t, "/foo/baz", r.URL.Path)
	assert.Equal(t, "q=Q42", r.URL.RawQuery)
	assert.Equal(t, "fr-FR,en", r.Headers["Accept-Language"])
	assert.Equal(t, "application/json", r.Headers["Content-Type"])
	assert.Equal(t, `{ "one": "un", "two": "deux" }`, r.Body)
}
