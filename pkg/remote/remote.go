package remote

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

var (
	// ErrNotFoundRemote is used when no request is defined for a doctype
	ErrNotFoundRemote = errors.New("the doctype has no request defined")
	// ErrInvalidRequest is used when we can't use the request defined by the
	// developer
	ErrInvalidRequest = errors.New("the request is not valid")
	// ErrRequestFailed when the connexion to the remote website can't be established
	ErrRequestFailed = errors.New("can't connect to the remote host")
)

// Remote is the struct used to call a remote website for a doctype
type Remote struct {
	Verb    string
	URL     *url.URL
	Headers map[string]string
	Body    string
}

var log = logger.WithNamespace("remote")

// ParseRawRequest takes a string and parse it as a remote struct.
// First line is verb and URL.
// Then, we have the headers.
// And for a POST, we have a blank line, and then the body.
func ParseRawRequest(doctype, raw string) (*Remote, error) {
	lines := strings.Split(raw, "\n")
	parts := strings.Split(lines[0], " ")
	if len(parts) != 2 {
		log.Infof("%s cannot be used as a remote doctype", doctype)
		return nil, ErrInvalidRequest
	}
	var remote Remote
	remote.Verb = parts[0]
	if remote.Verb != "GET" && remote.Verb != "POST" {
		log.Infof("Invalid verb for remote doctype %s: %s", doctype, remote.Verb)
		return nil, ErrInvalidRequest
	}
	u, err := url.Parse(parts[1])
	if err != nil {
		log.Infof("Invalid URL for remote doctype %s: %s", doctype, parts[1])
		return nil, ErrInvalidRequest
	}
	remote.URL = u
	remote.Headers = make(map[string]string)
	for i, line := range lines[1:] {
		if line == "" {
			remote.Body = strings.Join(lines[i+1:], "\n")
			break
		}
		parts = strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			log.Infof("Invalid header for remote doctype %s: %s", doctype, line)
			return nil, ErrInvalidRequest
		}
		remote.Headers[parts[0]] = strings.TrimSpace(parts[1])
	}
	return &remote, nil
}

// Find finds the request defined for the given doctype
func Find(doctype string) (*Remote, error) {
	var raw string

	if config.GetConfig().Doctypes == "" {
		// TODO fetch it from couch/github
		return nil, ErrNotFoundRemote
	}
	// } else {
	filename := path.Join(config.GetConfig().Doctypes, doctype, "request")
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, ErrNotFoundRemote
	}
	raw = string(bytes)

	remote, err := ParseRawRequest(doctype, raw)
	if err != nil {
		return nil, err
	}

	// Sanitize the remote URL
	if remote.URL.Scheme != "https" && remote.URL.Scheme != "http" {
		log.Infof("Invalid scheme for remote doctype %s: %s", doctype, remote.URL.Scheme)
		return nil, ErrInvalidRequest
	}
	if strings.Contains(remote.URL.Host, ":") {
		log.Infof("Invalid host for remote doctype %s: %s", doctype, remote.URL.Host)
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
	// TODO declare io.cozy.remote.requests in consts and data blacklist (-> read-only)
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
		log.Infof("Error on request %s: %s", remote.URL.String(), err)
		return ErrRequestFailed
	}

	rw.WriteHeader(res.StatusCode)
	copyHeader(rw.Header(), res.Header)
	_, err = io.Copy(rw, res.Body)
	if err != nil {
		log.Infof("Error on copying response from %s: %s", remote.URL.String(), err)
	}
	res.Body.Close()
	return nil
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
