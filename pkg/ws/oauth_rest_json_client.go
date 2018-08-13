package ws

import (
	"encoding/json"
)

// OAuthRestJSONClient is an OAuth client to access JSON REST API
type OAuthRestJSONClient struct {
	*OAuthRestClient
}

// Init initializes a client from base URL and OAuth token
func (r *OAuthRestJSONClient) Init(baseURL string, token string) {
	r.OAuthRestClient = &OAuthRestClient{}
	r.OAuthRestClient.Init(baseURL, token)
}

// Get access REST resource with GET request
func (r *OAuthRestJSONClient) Get(url string, result interface{}) error {
	body, err := r.OAuthRestClient.Get(url)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

// Post access REST resource with POST request
func (r *OAuthRestJSONClient) Post(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.OAuthRestClient.Post(url, "application/json", body)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

// Put access REST resource with PUT request
func (r *OAuthRestJSONClient) Put(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.OAuthRestClient.Put(url, "application/json", body)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

// Delete access REST resource with DELETE request
func (r *OAuthRestJSONClient) Delete(url string, result interface{}) error {
	body, err := r.OAuthRestClient.Delete(url)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}
