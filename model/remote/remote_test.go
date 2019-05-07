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
	vars, err := extractVariables("GET", in)
	assert.NoError(t, err)
	assert.Equal(t, "un", vars["one"])
	assert.Equal(t, "deux", vars["two"])
}

func TestExtractVariablesPOST(t *testing.T) {
	body := bytes.NewReader([]byte(`{"one": "un", "two": "deux"}`))
	in := httptest.NewRequest("POST", "https://example.com/bar", body)
	vars, err := extractVariables("POST", in)
	assert.NoError(t, err)
	assert.Equal(t, "un", vars["one"])
	assert.Equal(t, "deux", vars["two"])

	body = bytes.NewReader([]byte(`one=un&two=deux`))
	in = httptest.NewRequest("POST", "https://example.com/bar", body)
	_, err = extractVariables("POST", in)
	assert.Error(t, err)
}

func TestInjectVariables(t *testing.T) {
	raw := `POST https://example.org/foo/{{bar}}?q={{q}}
Content-Type: {{contentType}}
Accept-Language: {{lang}},en

{ "one": "{{ json one }}", "two": "{{ json two }}" }
<p>{{html content}}</p>`
	r, err := ParseRawRequest(doctype, raw)
	if !assert.NoError(t, err) {
		return
	}

	vars := map[string]string{
		"contentType": "application/json",
		"bar":         "baz&/",
		"q":           "Q42&?",
		"lang":        "fr-FR\n",
		"one":         "un\"\n",
		"two":         "deux",
		"content":     "hey ! <<>>",
	}

	err = injectVariables(r, vars)
	assert.NoError(t, err)
	assert.Equal(t, "POST", r.Verb)
	assert.Equal(t, "https", r.URL.Scheme)
	assert.Equal(t, "example.org", r.URL.Host)
	assert.Equal(t, "/foo/baz&%2F", r.URL.Path)
	assert.Equal(t, "q=Q42%26%3F", r.URL.RawQuery)
	assert.Equal(t, "fr-FR\\n,en", r.Headers["Accept-Language"])
	assert.Equal(t, "application/json", r.Headers["Content-Type"])
	assert.Equal(t, `{ "one": "un\"\n", "two": "deux" }
<p>hey ! &lt;&lt;&gt;&gt;</p>`, r.Body)

	r, err = ParseRawRequest(doctype, `POST https://example.org/{{missing}}`)
	assert.NoError(t, err)
	err = injectVariables(r, vars)
	assert.Equal(t, ErrMissingVar, err)
}
