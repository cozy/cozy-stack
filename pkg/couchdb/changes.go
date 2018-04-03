package couchdb

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-querystring/query"
)

// ChangesFeedMode is a value for the feed parameter of a ChangesRequest
type ChangesFeedMode string

// ChangesFeedStyle is a value for the style parameter of a ChangesRequest
type ChangesFeedStyle string

const (
	// ChangesModeNormal is the only mode supported by cozy-stack
	ChangesModeNormal ChangesFeedMode = "normal"
	// ChangesStyleAllDocs pass all revisions including conflicts
	ChangesStyleAllDocs ChangesFeedStyle = "all_docs"
	// ChangesStyleMainOnly only pass the winning revision
	ChangesStyleMainOnly ChangesFeedStyle = "main_only"
)

// ValidChangesMode convert any string into a ChangesFeedMode or gives an error
// if the string is invalid.
func ValidChangesMode(feed string) (ChangesFeedMode, error) {
	if feed == "" || feed == string(ChangesModeNormal) {
		return ChangesModeNormal, nil
	}

	err := fmt.Errorf("Unsuported feed value '%s'", feed)
	return ChangesModeNormal, err
}

// ValidChangesStyle convert any string into a ChangesFeedStyle or gives an
// error if the string is invalid.
func ValidChangesStyle(style string) (ChangesFeedStyle, error) {
	if style == "" || style == string(ChangesStyleMainOnly) {
		return ChangesStyleMainOnly, nil
	}
	if style == string(ChangesStyleAllDocs) {
		return ChangesStyleAllDocs, nil
	}
	err := fmt.Errorf("Unsuported style value '%s'", style)
	return ChangesStyleMainOnly, err
}

// A ChangesRequest are all parameters than can be passed to a changes feed
type ChangesRequest struct {
	DocType string `url:"-"`
	// see Changes Feeds. Default is normal.
	Feed ChangesFeedMode `url:"feed,omitempty"`
	// Maximum period in milliseconds to wait for a change before the response
	// is sent, even if there are no results. Only applicable for longpoll or
	// continuous feeds. Default value is specified by httpd/changes_timeout
	// configuration option. Note that 60000 value is also the default maximum
	// timeout to prevent undetected dead connections.
	Timeout int `url:"timeout,omitempty"`
	// Period in milliseconds after which an empty line is sent in the results.
	// Only applicable for longpoll, continuous, and eventsource feeds. Overrides
	// any timeout to keep the feed alive indefinitely. Default is 60000. May be
	// true to use default value.
	Heartbeat int `url:"heartbeat,omitempty"`
	// Includes conflicts information in response. Ignored if include_docs isn’t
	// true. Default is false.
	Conflicts bool `url:"conflicts,omitempty"`
	// Return the change results in descending sequence order (most recent change
	// first). Default is false.
	Descending bool `url:"descending,omitempty"`
	// Include the associated document with each result. If there are conflicts,
	// only the winning revision is returned. Default is false.
	IncludeDocs bool `url:"include_docs,omitempty"`
	// Include the Base64-encoded content of attachments in the documents that
	// are included if include_docs is true. Ignored if include_docs isn’t true.
	// Default is false.
	Attachments bool `url:"attachments,omitempty"`
	// Include encoding information in attachment stubs if include_docs is true
	// and the particular attachment is compressed. Ignored if include_docs isn’t
	// true. Default is false.
	AttEncodingInfo bool `url:"att_encoding_info,omitempty"`
	// Alias of Last-Event-ID header.
	LastEventID int `url:"last,omitempty"`
	// Limit number of result rows to the specified value (note that using 0 here
	//  has the same effect as 1).
	Limit int `url:"limit,omitempty"`
	// Start the results from the change immediately after the given update
	// sequence. Can be valid update sequence or now value. Default is 0.
	Since string `url:"since,omitempty"`
	// Specifies how many revisions are returned in the changes array. The
	// default, main_only, will only return the current “winning” revision;
	// all_docs will return all leaf revisions (including conflicts and deleted
	// former conflicts).
	Style ChangesFeedStyle `url:"style,omitempty"`
	// Reference to a filter function from a design document that will filter
	// whole stream emitting only filtered events. See the section Change
	// Notifications in the book CouchDB The Definitive Guide for more
	// information.
	Filter string `url:"filter,omitempty"`
	// Allows to use view functions as filters. Documents counted as “passed” for
	// view filter in case if map function emits at least one record for them.
	// See _view for more info.
	View string `url:"view,omitempty"`
	// SeqInterval tells CouchDB to only calculate the update seq with every
	// Nth result returned. It is used by PouchDB replication, and helps to
	// lower the load on a CouchDB cluster.
	SeqInterval int `url:"seq_interval,omitempty"`
}

// A ChangesResponse is the response provided by a GetChanges call
type ChangesResponse struct {
	LastSeq string   `json:"last_seq"` // Last change update sequence
	Pending int      `json:"pending"`  // Count of remaining items in the feed
	Results []Change `json:"results"`  // Changes made to a database
}

// A Change is an atomic change in couchdb
type Change struct {
	DocID   string  `json:"id"`
	Seq     string  `json:"seq"`
	Doc     JSONDoc `json:"doc"`
	Changes []struct {
		Rev string `json:"rev"`
	} `json:"changes"`
}

// GetChanges returns a list of change in couchdb
func GetChanges(db Database, req *ChangesRequest) (*ChangesResponse, error) {
	if req.DocType == "" {
		return nil, errors.New("Empty doctype in GetChanges")
	}

	v, err := query.Values(req)
	if err != nil {
		return nil, err
	}

	var response ChangesResponse
	url := "_changes?" + v.Encode()
	err = makeRequest(db, req.DocType, http.MethodGet, url, nil, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}
