package remote

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
)

var (
	// ErrNotFoundRemote is used when no request is defined for a doctype
	ErrNotFoundRemote = errors.New("the doctype has no request defined")
	// ErrInvalidRequest is used when we can't use the request defined by the
	// developer
	ErrInvalidRequest = errors.New("the request is not valid")
)

// Remote is the struct used to call a remote website for a doctype
type Remote struct {
	Verb    string
	URL     *url.URL
	Headers map[string]string
}

// Find finds the request defined for the given doctype
func Find(doctype string) (*Remote, error) {
	var remote *Remote
	// TODO don't use an hardcoded remote
	if doctype == "org.wikidata.entity" {
		url, _ := url.Parse("https://www.wikidata.org/wiki/Special:EntityData/Q42.json")
		headers := make(map[string]string)
		headers["Accept"] = "application/json"
		remote = &Remote{
			Verb:    "GET",
			URL:     url,
			Headers: headers,
		}
	}
	if remote == nil {
		return nil, ErrNotFoundRemote
	}

	// Sanitize the remote URL
	if remote.URL.Scheme != "https" && remote.URL.Scheme != "http" {
		return nil, ErrInvalidRequest
	}
	if strings.Contains(remote.URL.Host, ":") {
		return nil, ErrInvalidRequest
	}
	remote.URL.User = nil
	remote.URL.Fragment = ""
	return remote, nil
}

// ProxyTo calls the external website and proxy the reponse
func (remote *Remote) ProxyTo(rw http.ResponseWriter, in *http.Request) error {
	// TODO replace variables
	// TODO logging (to syslog & couchdb)
	// TODO check resp content-type

	req, err := http.NewRequest(remote.Verb, remote.URL.String(), nil)
	if err != nil {
		return ErrInvalidRequest
	}

	req.Header.Set("User-Agent", "cozy-stack "+config.Version+" ("+runtime.Version()+")")
	for k, v := range remote.Headers {
		req.Header.Set(k, v)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	rw.WriteHeader(res.StatusCode)
	copyHeader(rw.Header(), res.Header)
	_, err = io.Copy(rw, res.Body)
	res.Body.Close()
	return err
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
