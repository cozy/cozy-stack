package webdav

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProppatch_SingleProperty_Returns207With200 asserts that a PROPPATCH
// request with a single dead property SET returns 207 Multi-Status where
// the requested property is accepted with 200 OK.
// This is Strategy C: in-memory dead-property storage.
func TestProppatch_SingleProperty_Returns207With200(t *testing.T) {
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
	// Strategy C: property is accepted with 200 OK
	assert.Contains(t, body, "200", "dead property set must be accepted with 200 OK")
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

// TestProppatch_MultipleProperties_AllAcceptedWith200 asserts that a PROPPATCH
// with multiple properties in a single <D:set> all get accepted with 200 OK.
// Strategy C stores all dead properties in memory.
func TestProppatch_MultipleProperties_AllAcceptedWith200(t *testing.T) {
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
	// Strategy C: properties are accepted with 200 OK
	assert.Contains(t, body, "200")
	assert.NotContains(t, body, "403", "Strategy C must not reject properties with 403")
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
