package webdav

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildErrorXML_PropfindFiniteDepth asserts the XML body shape for the
// propfind-finite-depth precondition defined by RFC 4918 §8.7 + §9.1.
func TestBuildErrorXML_PropfindFiniteDepth(t *testing.T) {
	got := string(buildErrorXML("propfind-finite-depth"))

	assert.Contains(t, got, `xmlns:D="DAV:"`)
	assert.Contains(t, got, `<D:error`)
	assert.Contains(t, got, `<D:propfind-finite-depth`)
	assert.Contains(t, got, `</D:error>`)
}

// TestBuildErrorXML_Forbidden asserts that an arbitrary condition name is
// emitted as a single self-closing element inside the D:error root.
func TestBuildErrorXML_Forbidden(t *testing.T) {
	got := string(buildErrorXML("forbidden"))

	assert.Contains(t, got, `xmlns:D="DAV:"`)
	assert.Contains(t, got, `<D:error`)
	assert.Contains(t, got, `<D:forbidden`)
	assert.Contains(t, got, `</D:error>`)
	// The body must be well-formed enough that D:error opens before the
	// condition and closes after it.
	errOpen := strings.Index(got, "<D:error")
	condIdx := strings.Index(got, "<D:forbidden")
	errClose := strings.Index(got, "</D:error>")
	require.True(t, errOpen >= 0 && condIdx > errOpen && errClose > condIdx,
		"elements out of order: %q", got)
}

// TestSendWebDAVError_HeadersAndStatus verifies the full HTTP contract:
// status code, Content-Type with charset, Content-Length matching the body
// byte length, and body containing the namespaced condition element.
func TestSendWebDAVError_HeadersAndStatus(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/dav/files/foo", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := sendWebDAVError(c, http.StatusForbidden, "propfind-finite-depth")
	require.NoError(t, err)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, `application/xml; charset="utf-8"`, rec.Header().Get("Content-Type"))

	clen, cerr := strconv.Atoi(rec.Header().Get("Content-Length"))
	require.NoError(t, cerr)
	assert.Equal(t, len(rec.Body.Bytes()), clen)

	body := rec.Body.String()
	assert.Contains(t, body, `xmlns:D="DAV:"`)
	assert.Contains(t, body, "D:propfind-finite-depth")
	assert.Contains(t, body, "D:error")
}
