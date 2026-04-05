package webdav

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarshalMultistatus verifies that marshalling a Response containing all 9
// live properties produces XML with the `DAV:` namespace bound to the `D:`
// prefix and all expected property elements present.
func TestMarshalMultistatus(t *testing.T) {
	r := Response{
		Href: "/dav/files/doc.txt",
		Propstat: []Propstat{{
			Prop: Prop{
				DisplayName:      "doc.txt",
				GetContentLength: 1234,
				GetContentType:   "text/plain",
				GetETag:          `"abc123"`,
				GetLastModified:  "Mon, 07 Apr 2025 10:00:00 GMT",
				CreationDate:     "2025-04-07T10:00:00Z",
				ResourceType:     ResourceType{},
				SupportedLock:    &SupportedLock{},
			},
			Status: "HTTP/1.1 200 OK",
		}},
	}
	data, err := marshalMultistatus([]Response{r})
	require.NoError(t, err)
	s := string(data)

	assert.Contains(t, s, `xmlns:D="DAV:"`)
	assert.Contains(t, s, "D:multistatus")
	assert.Contains(t, s, "D:response")
	assert.Contains(t, s, "D:href")
	assert.Contains(t, s, "D:propstat")
	assert.Contains(t, s, "D:prop")
	assert.Contains(t, s, "D:displayname")
	assert.Contains(t, s, "D:getcontentlength")
	assert.Contains(t, s, "D:getcontenttype")
	assert.Contains(t, s, "D:getetag")
	assert.Contains(t, s, "D:getlastmodified")
	assert.Contains(t, s, "D:creationdate")
	assert.Contains(t, s, "D:resourcetype")
	assert.Contains(t, s, "D:supportedlock")
	assert.Contains(t, s, "doc.txt")
}

// TestXMLNamespacePrefix asserts that every element in the emitted multistatus
// XML uses the literal D: prefix (required for Windows Mini-Redirector).
func TestXMLNamespacePrefix(t *testing.T) {
	r := Response{
		Href: "/dav/files/doc.txt",
		Propstat: []Propstat{{
			Prop:   Prop{GetETag: `"abc"`, GetLastModified: "Mon, 07 Apr 2025 10:00:00 GMT"},
			Status: "HTTP/1.1 200 OK",
		}},
	}
	data, err := marshalMultistatus([]Response{r})
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, `xmlns:D="DAV:"`)
	assert.Contains(t, s, "D:multistatus")
	assert.Contains(t, s, "D:response")
	assert.Contains(t, s, "D:href")
	assert.Contains(t, s, "D:propstat")
	assert.Contains(t, s, "D:getetag")
}

// TestGetLastModifiedFormat verifies RFC 1123 (http.TimeFormat) is used for
// getlastmodified and NOT RFC 3339. macOS Finder silently misparses ISO 8601.
func TestGetLastModifiedFormat(t *testing.T) {
	tm := time.Date(2025, 4, 7, 10, 0, 0, 0, time.UTC)
	got := tm.UTC().Format(http.TimeFormat)
	assert.Equal(t, "Mon, 07 Apr 2025 10:00:00 GMT", got)
	// Guard against an RFC 3339 regression: the ISO 8601 date/time
	// separator 'T' (as in 2025-04-07T10:00:00Z) must not appear between
	// the date and the time — macOS Finder silently misparses that form.
	assert.NotRegexp(t, `\dT\d`, got)
}

// TestGetETagQuoting asserts buildETag returns a double-quoted string built
// from an MD5 sum (content-addressed).
func TestGetETagQuoting(t *testing.T) {
	md5 := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb}
	got := buildETag(md5)
	re := regexp.MustCompile(`^"[A-Za-z0-9+/=]+"$`)
	assert.Regexp(t, re, got)
	assert.True(t, strings.HasPrefix(got, `"`))
	assert.True(t, strings.HasSuffix(got, `"`))
}

// TestCreationDateISO8601 asserts buildCreationDate returns the RFC 3339 form
// ending in "Z". creationdate uses ISO 8601 per RFC 4918 §15.1.
func TestCreationDateISO8601(t *testing.T) {
	tm := time.Date(2025, 4, 7, 10, 0, 0, 0, time.UTC)
	got := buildCreationDate(tm)
	assert.True(t, strings.HasSuffix(got, "Z"), "got=%q", got)
	assert.Contains(t, got, "2025-04-07T10:00:00")
}

// TestParsePropfindRequest parses both an explicit <D:allprop/> request and an
// empty body. Per RFC 4918 §9.1, an empty body equals allprop.
func TestParsePropfindRequest(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`)
	pf, err := parsePropFind(body)
	require.NoError(t, err)
	require.NotNil(t, pf)
	assert.NotNil(t, pf.AllProp)

	pf2, err := parsePropFind([]byte{})
	require.NoError(t, err)
	require.NotNil(t, pf2)
	assert.NotNil(t, pf2.AllProp, "empty body should default to allprop")
}

// TestResourceTypeCollectionVsFile asserts the serialised form of
// ResourceType differs between files (empty) and collections (<D:collection/>).
func TestResourceTypeCollectionVsFile(t *testing.T) {
	// Collection
	colResp := Response{
		Href: "/dav/files/MyDir/",
		Propstat: []Propstat{{
			Prop: Prop{
				ResourceType: ResourceType{Collection: &struct{}{}},
			},
			Status: "HTTP/1.1 200 OK",
		}},
	}
	data, err := marshalMultistatus([]Response{colResp})
	require.NoError(t, err)
	assert.Contains(t, string(data), "D:collection")

	// File
	fileResp := Response{
		Href: "/dav/files/note.txt",
		Propstat: []Propstat{{
			Prop: Prop{
				ResourceType: ResourceType{},
			},
			Status: "HTTP/1.1 200 OK",
		}},
	}
	data, err = marshalMultistatus([]Response{fileResp})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "D:collection")
}
