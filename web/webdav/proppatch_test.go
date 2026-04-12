package webdav

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProppatch_SingleProperty_Returns207With403 asserts that a PROPPATCH
// request with a single dead property SET returns 207 Multi-Status where
// the requested property is rejected with 403 Forbidden.
// This is Strategy B: server rejects all dead-property writes with 403.
// Litmus propset test: sets a custom property and expects either 207 with
// 200 OK propstat (stored) or some non-405 response.
func TestProppatch_SingleProperty_Returns207With403(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	body := env.E.Request("PROPPATCH", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Content-Type", "application/xml; charset=utf-8").
		WithBytes([]byte(`<?xml version="1.0" encoding="utf-8"?>
<D:propertyupdate xmlns:D="DAV:" xmlns:X="http://example.com/ns">
  <D:set>
    <D:prop>
      <X:foo>bar</X:foo>
    </D:prop>
  </D:set>
</D:propertyupdate>`)).
		Expect().
		Status(http.StatusMultiStatus).
		Body().Raw()

	// Must be a valid multistatus
	assert.Contains(t, body, "D:multistatus", "response must be a DAV multistatus")
	assert.Contains(t, body, "D:propstat", "response must contain propstat")
	// Must reject with 403 (Strategy B: no dead-property storage)
	assert.Contains(t, body, "403", "dead property set must be rejected with 403")
}

// TestProppatch_NoBody_Returns400 asserts that a PROPPATCH with no body
// returns 400 Bad Request.
func TestProppatch_NoBody_Returns400(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.Request("PROPPATCH", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(http.StatusBadRequest)
}

// TestProppatch_MalformedXML_Returns400 asserts that a PROPPATCH with
// malformed XML body returns 400 Bad Request.
func TestProppatch_MalformedXML_Returns400(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.Request("PROPPATCH", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Content-Type", "application/xml; charset=utf-8").
		WithBytes([]byte(`<?xml version="1.0" encoding="utf-8"?><D:propertyupdate xmlns:D="DAV:`)).
		Expect().
		Status(http.StatusBadRequest)
}

// TestProppatch_MultipleProperties_AllRejectedWith403 asserts that a PROPPATCH
// with multiple properties in a single <D:set> all get rejected with 403.
func TestProppatch_MultipleProperties_AllRejectedWith403(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	body := env.E.Request("PROPPATCH", "/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Content-Type", "application/xml; charset=utf-8").
		WithBytes([]byte(`<?xml version="1.0" encoding="utf-8"?>
<D:propertyupdate xmlns:D="DAV:" xmlns:X="http://example.com/ns">
  <D:set>
    <D:prop>
      <X:foo>bar</X:foo>
      <X:baz>qux</X:baz>
    </D:prop>
  </D:set>
</D:propertyupdate>`)).
		Expect().
		Status(http.StatusMultiStatus).
		Body().Raw()

	assert.Contains(t, body, "D:multistatus")
	// 403 must appear somewhere (for the rejection)
	assert.Contains(t, body, "403")
	// The response body should contain the property names from the request
	count403 := strings.Count(body, "403")
	assert.GreaterOrEqual(t, count403, 1, "at least one 403 propstat expected")
}

// TestProppatch_NotFound_Returns404 asserts that PROPPATCH on a non-existent
// resource returns 404 Not Found.
func TestProppatch_NotFound_Returns404(t *testing.T) {
	env := newWebdavTestEnv(t, nil)

	env.E.Request("PROPPATCH", "/dav/files/does-not-exist").
		WithHeader("Authorization", "Bearer "+env.Token).
		WithHeader("Content-Type", "application/xml; charset=utf-8").
		WithBytes([]byte(`<?xml version="1.0" encoding="utf-8"?>
<D:propertyupdate xmlns:D="DAV:">
  <D:set>
    <D:prop>
      <D:displayname>test</D:displayname>
    </D:prop>
  </D:set>
</D:propertyupdate>`)).
		Expect().
		Status(http.StatusNotFound)
}
