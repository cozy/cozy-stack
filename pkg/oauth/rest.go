package oauth

import (
	"bytes"
	"context"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"io"
	"io/ioutil"
	"net/http"
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

type Rest struct {
	baseURL string
	client  *http.Client
}

func (r *Rest) Init(baseURL string, token string) {
	r.baseURL = baseURL
	tokenSource := &tokenSource{
		AccessToken: token,
	}
	r.client = oauth2.NewClient(context.TODO(), tokenSource)
}

func (r *Rest) Do(req *http.Request) ([]byte, error) {
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

func (r *Rest) newRequest(url string, method string, body []byte) (*http.Request, error) {
	url = r.baseURL + url
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = nil
	}
	return http.NewRequest(method, url, reader)
}

func (r *Rest) Get(url string) ([]byte, error) {
	url = r.baseURL + url
	req, err := http.NewRequest(url, "GET", nil)
	if err != nil {
		return nil, err
	}
	return r.Do(req)
}

func (r *Rest) Post(url string, contentType string, body []byte) ([]byte, error) {
	req, err := r.newRequest(url, "POST", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return r.Do(req)
}

func (r *Rest) Put(url string, contentType string, body []byte) ([]byte, error) {
	req, err := r.newRequest(url, "PUT", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return r.Do(req)
}

func (r *Rest) Delete(url string) ([]byte, error) {
	req, err := r.newRequest(url, "DELETE", nil)
	if err != nil {
		return nil, err
	}
	return r.Do(req)
}
