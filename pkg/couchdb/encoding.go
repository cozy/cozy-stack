package couchdb

import (
	"encoding/json"
	"net/url"

	"github.com/google/go-querystring/query"
)

func maybeSet(u url.Values, k string, v interface{}) error {
	if v == nil || v == "" {
		return nil
	}

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	u.Set(k, string(b))
	return nil
}

// Values transforms a ViewRequest into a query string suitable for couchdb
// ie, where non-scalar fields have been JSON+URL encoded.
func (vr *ViewRequest) Values() (url.Values, error) {

	var v url.Values

	v, err := query.Values(vr)
	if err != nil {
		return nil, err
	}

	if err := maybeSet(v, "key", vr.Key); err != nil {
		return nil, err
	}
	if len(vr.Keys) > 0 {
		if err := maybeSet(v, "keys", vr.Keys); err != nil {
			return nil, err
		}
	}
	if err := maybeSet(v, "start_key", vr.StartKey); err != nil {
		return nil, err
	}
	if err := maybeSet(v, "end_key", vr.EndKey); err != nil {
		return nil, err
	}

	return v, nil
}

// Values transforms a AllDocsRequest into a query string suitable for couchdb
// ie, where non-scalar fields have been JSON+URL encoded.
func (adr *AllDocsRequest) Values() (url.Values, error) {

	var v url.Values

	v, err := query.Values(adr)
	if err != nil {
		return nil, err
	}

	if len(adr.Keys) > 0 {
		if err := maybeSet(v, "keys", adr.Keys); err != nil {
			return nil, err
		}
	}
	if err := maybeSet(v, "startkey", adr.StartKey); err != nil {
		return nil, err
	}
	if err := maybeSet(v, "endkey", adr.EndKey); err != nil {
		return nil, err
	}

	return v, nil
}
