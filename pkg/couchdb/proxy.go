package couchdb

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/labstack/echo/v4"
)

// Proxy generate a httputil.ReverseProxy which forwards the request to the
// correct route.
func Proxy(db Database, doctype, path string) *httputil.ReverseProxy {
	couchURL := config.CouchURL()
	couchAuth := config.GetConfig().CouchDB.Auth

	director := func(req *http.Request) {
		req.URL.Scheme = couchURL.Scheme
		req.URL.Host = couchURL.Host
		req.Header.Del(echo.HeaderAuthorization) // drop stack auth
		req.Header.Del(echo.HeaderCookie)
		req.URL.RawPath = "/" + makeDBName(db, doctype) + "/" + path
		req.URL.Path, _ = url.PathUnescape(req.URL.RawPath)
		if couchAuth != nil {
			if p, ok := couchAuth.Password(); ok {
				req.SetBasicAuth(couchAuth.Username(), p)
			}
		}
	}

	var transport http.RoundTripper
	if client := config.GetConfig().CouchDB.Client; client != nil {
		transport = client.Transport
	} else {
		transport = http.DefaultTransport
	}

	return &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
	}
}

// ProxyBulkDocs generates a httputil.ReverseProxy to forward the couchdb
// request on the _bulk_docs endpoint. This endpoint is specific since it will
// mutate many document in database, the stack has to read the response from
// couch to emit the correct realtime events.
func ProxyBulkDocs(db Database, doctype string, req *http.Request) (*httputil.ReverseProxy, *http.Request, error) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, nil, err
	}

	var reqValue struct {
		Docs     []JSONDoc `json:"docs"`
		NewEdits *bool     `json:"new_edits"`
	}

	if err = json.Unmarshal(body, &reqValue); err != nil {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest,
			"request body is not valid JSON")
	}

	docs := make(map[string]JSONDoc)
	for _, d := range reqValue.Docs {
		docs[d.ID()] = d
	}

	// reset body to proxy
	req.Body = ioutil.NopCloser(bytes.NewReader(body))

	var transport http.RoundTripper
	if client := config.GetConfig().CouchDB.Client; client != nil {
		transport = client.Transport
	} else {
		transport = http.DefaultTransport
	}

	p := Proxy(db, doctype, "/_bulk_docs")
	p.Transport = &bulkTransport{
		RoundTripper: transport,
		OnResponseRead: func(data []byte) {
			type respValue struct {
				ID    string `json:"id"`
				Rev   string `json:"rev"`
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}

			// When using the 'new_edits' flag (like pouchdb), the couchdb response
			// does not contain any value. We only rely on the request data and
			// expect no error.
			if reqValue.NewEdits != nil && !*reqValue.NewEdits {
				for _, doc := range reqValue.Docs {
					doc.Type = doctype
					rev := doc.Rev()
					var event string
					if strings.HasPrefix(rev, "1-") {
						event = realtime.EventCreate
					} else {
						event = realtime.EventUpdate
					}
					RTEvent(db, event, doc, nil)
				}
			} else {
				var respValues []*respValue
				if err = json.Unmarshal(data, &respValues); err != nil {
					return
				}

				docs := make(map[string]JSONDoc)
				for _, doc := range reqValue.Docs {
					docs[doc.ID()] = doc
				}

				for _, r := range respValues {
					if r.Error != "" || !r.OK {
						continue
					}
					doc, ok := docs[r.ID]
					if !ok {
						continue
					}
					var event string
					if doc.Rev() == "" {
						event = realtime.EventCreate
					} else if doc.Get("_deleted") == true {
						event = realtime.EventDelete
					} else {
						event = realtime.EventUpdate
					}
					doc.SetRev(r.Rev)
					RTEvent(db, event, doc, nil)
				}
			}
		},
	}

	return p, req, nil
}

type bulkTransport struct {
	http.RoundTripper
	OnResponseRead func([]byte)
}

func (t *bulkTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp, err = t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, newConnectionError(err)
	}
	defer func() {
		if errc := resp.Body.Close(); err == nil && errc != nil {
			err = errc
		}
	}()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusCreated {
		go t.OnResponseRead(b)
	}
	resp.Body = ioutil.NopCloser(bytes.NewReader(b))
	return resp, nil
}
