// Package webdav — XML helpers for RFC 4918 PROPFIND responses.
//
// The types and helpers in this file implement the minimum surface required
// by the WebDAV PROPFIND/PROPPATCH machinery: a Multistatus/Response/Propstat/
// Prop tree, a ResourceType discriminator for files vs collections, a
// PropFind request parser, and marshalling helpers that guarantee the
// `D:` XML namespace prefix (required for Windows Mini-Redirector
// compatibility).
//
// Struct tags intentionally use a literal `D:` element-name prefix rather
// than Go's `"DAV: name"` namespace form. The multistatus root element is
// written by hand with `xmlns:D="DAV:"`, and every child reuses that
// prefix by name. Using the namespace form would cause encoding/xml to
// emit redundant `xmlns="DAV:"` declarations on every child element,
// which Windows Mini-Redirector rejects (see 01-RESEARCH.md Risk 1).
package webdav

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"time"
)

// Multistatus is the root element of a 207 Multi-Status response per
// RFC 4918 §14.16.
type Multistatus struct {
	XMLName   xml.Name   `xml:"D:multistatus"`
	Responses []Response `xml:"D:response"`
}

// Response is a single <D:response> entry inside a multistatus document
// (RFC 4918 §14.24).
type Response struct {
	XMLName  xml.Name   `xml:"D:response"`
	Href     string     `xml:"D:href"`
	Propstat []Propstat `xml:"D:propstat"`
}

// Propstat groups a set of properties with a single HTTP status
// (RFC 4918 §14.22).
type Propstat struct {
	XMLName xml.Name `xml:"D:propstat"`
	Prop    Prop     `xml:"D:prop"`
	Status  string   `xml:"D:status"`
}

// Prop carries the nine WebDAV live properties this server supports.
// Zero-valued string / int fields are omitted by the marshaller so the
// same struct can describe both files and collections.
type Prop struct {
	XMLName          xml.Name       `xml:"D:prop"`
	ResourceType     ResourceType   `xml:"D:resourcetype"`
	DisplayName      string         `xml:"D:displayname,omitempty"`
	GetLastModified  string         `xml:"D:getlastmodified,omitempty"`
	GetETag          string         `xml:"D:getetag,omitempty"`
	GetContentLength int64          `xml:"D:getcontentlength,omitempty"`
	GetContentType   string         `xml:"D:getcontenttype,omitempty"`
	CreationDate     string         `xml:"D:creationdate,omitempty"`
	SupportedLock    *SupportedLock `xml:"D:supportedlock,omitempty"`
	LockDiscovery    *LockDiscovery `xml:"D:lockdiscovery,omitempty"`
}

// ResourceType encodes whether a resource is a regular file (empty) or a
// collection (contains <D:collection/>). Collection is modelled as a
// pointer to an empty struct so omitempty produces the exact XML shape
// Windows Mini-Redirector expects.
type ResourceType struct {
	XMLName    xml.Name  `xml:"D:resourcetype"`
	Collection *struct{} `xml:"D:collection,omitempty"`
}

// SupportedLock is an RFC 4918 §15.10 stub — PROPFIND responses advertise
// an empty <D:supportedlock/> element because this server is currently
// Class 1 only (no real lock support). The element is still emitted so
// that clients which probe for lock capability find the property defined.
type SupportedLock struct {
	XMLName xml.Name `xml:"D:supportedlock"`
}

// LockDiscovery is an RFC 4918 §15.8 stub, empty for Class 1 servers.
type LockDiscovery struct {
	XMLName xml.Name `xml:"D:lockdiscovery"`
}

// PropFind models the body of a PROPFIND request (RFC 4918 §14.20).
// Exactly one of AllProp, PropName, or Prop should be non-nil after
// parsing; an empty request body is treated as AllProp.
//
// The request parser uses the `DAV:` namespace form (not the `D:` prefix
// form) because inbound requests may bind the namespace to an arbitrary
// prefix — clients are not required to use `D:`.
type PropFind struct {
	XMLName  xml.Name  `xml:"DAV: propfind"`
	AllProp  *struct{} `xml:"DAV: allprop"`
	PropName *struct{} `xml:"DAV: propname"`
	Prop     *PropList `xml:"DAV: prop"`
}

// PropList is the per-property inclusion set of a <D:prop>-style
// PROPFIND request — each non-nil field signals that the client wants
// that particular live property returned.
type PropList struct {
	ResourceType     *struct{} `xml:"DAV: resourcetype"`
	DisplayName      *struct{} `xml:"DAV: displayname"`
	GetLastModified  *struct{} `xml:"DAV: getlastmodified"`
	GetETag          *struct{} `xml:"DAV: getetag"`
	GetContentLength *struct{} `xml:"DAV: getcontentlength"`
	GetContentType   *struct{} `xml:"DAV: getcontenttype"`
	CreationDate     *struct{} `xml:"DAV: creationdate"`
	SupportedLock    *struct{} `xml:"DAV: supportedlock"`
	LockDiscovery    *struct{} `xml:"DAV: lockdiscovery"`
}

// buildETag returns an RFC 7232 strong ETag derived from a file's md5sum.
// The returned string includes the surrounding double quotes required by
// the spec (RFC 7232 §2.3). Never use CouchDB _rev here — _rev changes on
// metadata edits, which would break client caches.
func buildETag(md5sum []byte) string {
	return `"` + base64.StdEncoding.EncodeToString(md5sum) + `"`
}

// buildCreationDate returns an ISO 8601 / RFC 3339 date string as required
// by RFC 4918 §15.1 for the DAV:creationdate property.
func buildCreationDate(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// buildLastModified returns the RFC 1123 date string required by RFC 7231
// for the DAV:getlastmodified property and the HTTP Last-Modified header.
// macOS Finder silently misparses any other format — do not use RFC 3339.
func buildLastModified(t time.Time) string {
	return t.UTC().Format(http.TimeFormat)
}

// parsePropFind parses the body of a PROPFIND request. Per RFC 4918 §9.1,
// an empty body is equivalent to <D:allprop/>.
func parsePropFind(body []byte) (*PropFind, error) {
	pf := &PropFind{}
	if len(bytes.TrimSpace(body)) == 0 {
		pf.AllProp = &struct{}{}
		return pf, nil
	}
	if err := xml.Unmarshal(body, pf); err != nil {
		return nil, err
	}
	return pf, nil
}

// marshalMultistatus writes a complete 207 Multi-Status response to a
// byte slice. The root element is written manually to guarantee the
// `D:` prefix on the xmlns declaration — Go's encoding/xml would otherwise
// generate an auto-prefix such as `ns1:`, which Windows Mini-Redirector
// rejects outright (see 01-RESEARCH.md Risk 1).
func marshalMultistatus(responses []Response) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	enc := xml.NewEncoder(&buf)
	for i := range responses {
		if err := enc.Encode(responses[i]); err != nil {
			return nil, err
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	buf.WriteString(`</D:multistatus>`)
	return buf.Bytes(), nil
}
