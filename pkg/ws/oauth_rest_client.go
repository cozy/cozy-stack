package ws

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

type tokenSource struct {
	AccessToken string
}

func (t *tokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

// OAuthRestClient is an OAuth client to access REST API
type OAuthRestClient struct {
	baseURL string
	client  *http.Client
}

// Init initializes a client from base URL and OAuth token
func (r *OAuthRestClient) Init(baseURL string, token string) {
	r.baseURL = baseURL
	tokenSource := &tokenSource{
		AccessToken: token,
	}
	r.client = oauth2.NewClient(context.TODO(), tokenSource)
}

// Do access REST resource with HTTP request
func (r *OAuthRestClient) Do(req *http.Request) ([]byte, error) {
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, nil
	}
	body := resp.Body
	defer body.Close()
	if resp.StatusCode >= 400 {
		return nil, errors.New(resp.Status)
	}
	return ioutil.ReadAll(body)
}

func (r *OAuthRestClient) newRequest(url string, method string, body []byte) (*http.Request, error) {
	url = r.baseURL + url
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = nil
	}
	return http.NewRequest(method, url, reader)
}

// Get access REST resource with GET request
func (r *OAuthRestClient) Get(url string) ([]byte, error) {
	url = r.baseURL + url
	req, err := http.NewRequest(url, "GET", nil)
	if err != nil {
		return nil, err
	}
	return r.Do(req)
}

// Post access REST resource with POST request
func (r *OAuthRestClient) Post(url string, contentType string, body []byte) ([]byte, error) {
	req, err := r.newRequest(url, "POST", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return r.Do(req)
}

// Put access REST resource with PUT request
func (r *OAuthRestClient) Put(url string, contentType string, body []byte) ([]byte, error) {
	req, err := r.newRequest(url, "PUT", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return r.Do(req)
}

// Delete access REST resource with DELETE request
func (r *OAuthRestClient) Delete(url string) ([]byte, error) {
	req, err := r.newRequest(url, "DELETE", nil)
	if err != nil {
		return nil, err
	}
	return r.Do(req)
}
