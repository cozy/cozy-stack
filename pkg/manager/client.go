// Package manager is used for interacting with the cloudery.
package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

// tokenSource implements the oauth2.TokenSource interface
type tokenSource struct {
	token string
}

func (t *tokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.token,
	}
	return token, nil
}

// APIClient is an http client that can be used to query the API of the
// manager.
type APIClient struct {
	baseURL string
	client  *http.Client
}

// NewAPIClient builds a new client for the manager API
func NewAPIClient(baseURL, token string) *APIClient {
	tokenSource := &tokenSource{token: token}
	client := oauth2.NewClient(context.Background(), tokenSource)
	client.Timeout = 15 * time.Second
	return &APIClient{
		baseURL: baseURL,
		client:  client,
	}
}

// Do makes a request to the manager API
func (c *APIClient) Do(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	return c.client.Do(req)
}

// Get makes a GET request to the manager API
func (c *APIClient) Get(url string) (map[string]interface{}, error) {
	res, err := c.Do(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, errors.New(res.Status)
	}
	var data map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// Post makes a POST request to the manager API
func (c *APIClient) Post(url string, body io.Reader) error {
	res, err := c.Do(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	if err := res.Body.Close(); err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return errors.New(res.Status)
	}
	return nil
}

// Put makes a PUT request to the manager API
func (c *APIClient) Put(url string, params map[string]interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(body)
	res, err := c.Do(http.MethodPut, url, reader)
	if err != nil {
		return err
	}
	if err := res.Body.Close(); err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return errors.New(res.Status)
	}
	return nil
}

// Delete makes a DELETE request to the manager API
func (c *APIClient) Delete(url string) error {
	res, err := c.Do(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	if err := res.Body.Close(); err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return errors.New(res.Status)
	}
	return nil
}
