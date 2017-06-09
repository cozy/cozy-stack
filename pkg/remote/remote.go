package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
)

var (
	// ErrNotFoundRemote is used when no request is defined for a doctype
	ErrNotFoundRemote = errors.New("the doctype has no request defined")
	// ErrInvalidRequest is used when we can't use the request defined by the
	// developer
	ErrInvalidRequest = errors.New("the request is not valid")
	// ErrRequestFailed is used when the connexion to the remote website can't
	// be established
	ErrRequestFailed = errors.New("can't connect to the remote host")
	// ErrInvalidVariables is used when the variables can't be extracted from
	// the request
	ErrInvalidVariables = errors.New("the variables are not valid")
	// ErrInvalidContentType is used when the response has a content-type that
	// we deny for security reasons
	ErrInvalidContentType = errors.New("the content-type for the response is not authorized")
)

const rawURL = "https://raw.githubusercontent.com/cozy/cozy-doctypes/master/%s/request"

// Doctype is used to describe a doctype, its request for a remote doctype for example
type Doctype struct {
	DocID   string `json:"_id,omitempty"`
	DocRev  string `json:"_rev,omitempty"`
	Request string `json:"request"`
}

// ID is used to implement the couchdb.Doc interface
func (d *Doctype) ID() string { return d.DocID }

// Rev is used to implement the couchdb.Doc interface
func (d *Doctype) Rev() string { return d.DocRev }

// SetID is used to implement the couchdb.Doc interface
func (d *Doctype) SetID(id string) { d.DocID = id }

// SetRev is used to implement the couchdb.Doc interface
func (d *Doctype) SetRev(rev string) { d.DocRev = rev }

// DocType implements couchdb.Doc
func (d *Doctype) DocType() string { return consts.Doctypes }

// Clone implements couchdb.Doc
func (d *Doctype) Clone() couchdb.Doc { cloned := *d; return &cloned }

// Request is used to log in couchdb a call to a remote website
type Request struct {
	DocID         string `json:"_id,omitempty"`
	DocRev        string `json:"_rev,omitempty"`
	RemoteDoctype string `json:"doctype"`
	Verb          string `json:"verb"`
	URL           string `json:"url"`
	ResponseCode  int    `json:"response_code"`
	ContentType   string `json:"content_type"`
}

// ID is used to implement the couchdb.Doc interface
func (r *Request) ID() string { return r.DocID }

// Rev is used to implement the couchdb.Doc interface
func (r *Request) Rev() string { return r.DocRev }

// SetID is used to implement the couchdb.Doc interface
func (r *Request) SetID(id string) { r.DocID = id }

// SetRev is used to implement the couchdb.Doc interface
func (r *Request) SetRev(rev string) { r.DocRev = rev }

// DocType implements couchdb.Doc
func (r *Request) DocType() string { return consts.RemoteRequests }

// Clone implements couchdb.Doc
func (r *Request) Clone() couchdb.Doc { cloned := *r; return &cloned }

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
	if u.Scheme != "https" && u.Scheme != "http" {
		log.Infof("Invalid scheme for remote doctype %s: %s", doctype, u.Scheme)
		return nil, ErrInvalidRequest
	}
	remote.URL = u
	remote.Headers = make(map[string]string)
	for i, line := range lines[1:] {
		if line == "" {
			if remote.Verb == "GET" {
				continue
			}
			remote.Body = strings.Join(lines[i+2:], "\n")
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
func Find(ins *instance.Instance, doctype string) (*Remote, error) {
	var raw string

	if config.GetConfig().Doctypes == "" {
		dt := Doctype{
			DocID: consts.Doctypes + "/" + doctype,
		}
		err := couchdb.GetDoc(ins, consts.Doctypes, dt.DocID, &dt)
		if err != nil {
			u := fmt.Sprintf(rawURL, doctype)
			res, err := http.Get(u)
			log.Debugf("Fetch remote doctype from %s\n", doctype)
			if err != nil {
				log.Infof("Request not found for remote doctype %s: %s", doctype, err)
				return nil, ErrNotFoundRemote
			}
			b, err := ioutil.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				log.Infof("Request not found for remote doctype %s: %s", doctype, err)
				return nil, ErrNotFoundRemote
			}
			dt.Request = string(b)
			err = couchdb.CreateNamedDocWithDB(ins, &dt)
			if err != nil {
				log.Infof("Cannot save remote doctype %s: %s", doctype, err)
			}
		}
		raw = dt.Request
	} else {
		filename := path.Join(config.GetConfig().Doctypes, doctype, "request")
		bytes, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, ErrNotFoundRemote
		}
		raw = string(bytes)
	}

	return ParseRawRequest(doctype, raw)
}

// ExtractVariables extracts the variables:
// - from the query string for a GET
// - from the body formatted as JSON for a POST
func ExtractVariables(verb string, in *http.Request) (map[string]string, error) {
	vars := make(map[string]string)
	if verb == "GET" {
		for k, v := range in.URL.Query() {
			vars[k] = v[0]
		}
	} else {
		err := json.NewDecoder(in.Body).Decode(&vars)
		if err != nil {
			return nil, err
		}
	}
	return vars, nil
}

var injectionRegexp = regexp.MustCompile("{{\\w+}}")

func injectVar(src string, vars map[string]string) string {
	return injectionRegexp.ReplaceAllStringFunc(src, func(m string) string {
		m = strings.TrimLeft(m, "{{")
		m = strings.TrimRight(m, "}}")
		m = strings.TrimSpace(m)
		return vars[m]
	})
}

// InjectVariables replaces {{variable}} by its value in some fields of the
// remote struct
func InjectVariables(remote *Remote, vars map[string]string) {
	if strings.Contains(remote.URL.Path, "{{") {
		remote.URL.Path = injectVar(remote.URL.Path, vars)
	}
	if strings.Contains(remote.URL.RawQuery, "{{") {
		remote.URL.RawQuery = injectVar(remote.URL.RawQuery, vars)
	}
	for k, v := range remote.Headers {
		if strings.Contains(v, "{{") {
			remote.Headers[k] = injectVar(v, vars)
		}
	}
	if strings.Contains(remote.Body, "{{") {
		remote.Body = injectVar(remote.Body, vars)
	}
}

// ProxyTo calls the external website and proxy the reponse
func (remote *Remote) ProxyTo(doctype string, ins *instance.Instance, rw http.ResponseWriter, in *http.Request) error {
	vars, err := ExtractVariables(remote.Verb, in)
	if err != nil {
		log.Infof("Error on extracting variables: %s", err)
		return ErrInvalidVariables
	}
	InjectVariables(remote, vars)

	// Sanitize the remote URL
	if strings.Contains(remote.URL.Host, ":") {
		log.Infof("Invalid host for remote doctype %s: %s", doctype, remote.URL.Host)
		return ErrInvalidRequest
	}
	remote.URL.User = nil
	remote.URL.Fragment = ""

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
	defer res.Body.Close()

	ctype := strings.SplitN(res.Header.Get("Content-Type"), ";", 2)[0] // Drop the charset
	if ctype != "application/json" && ctype != "text/xml" && ctype != "application/xml" {
		class := strings.SplitN(ctype, "/", 2)[0]
		if class != "image" && class != "audio" && class != "video" {
			log.Infof("request %s has a content-type that is not allowed: %s",
				remote.URL.String(), ctype)
			return ErrInvalidContentType
		}
	}

	logged := &Request{
		RemoteDoctype: doctype,
		Verb:          remote.Verb,
		URL:           remote.URL.String(),
		ResponseCode:  res.StatusCode,
		ContentType:   ctype,
	}
	err = couchdb.CreateDoc(ins, logged)
	if err != nil {
		log.Errorf("Can't save remote request: %s", err)
	}
	log.Debugf("Remote request: %#v\n", logged)

	rw.WriteHeader(res.StatusCode)
	copyHeader(rw.Header(), res.Header)
	_, err = io.Copy(rw, res.Body)
	if err != nil {
		log.Infof("Error on copying response from %s: %s", remote.URL.String(), err)
	}
	return nil
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

var (
	_ couchdb.Doc = (*Doctype)(nil)
	_ couchdb.Doc = (*Request)(nil)
)
