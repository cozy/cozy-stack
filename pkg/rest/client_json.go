package rest

import (
	"encoding/json"
)

type JSONClient struct {
	*Client
}

func (r *JSONClient) Init(baseURL string, token string) {
	r.Client = &Client{}
	r.Client.Init(baseURL, token)
}

func (r *JSONClient) Get(url string, result interface{}) error {
	body, err := r.Client.Get(url)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (r *JSONClient) Post(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.Client.Post(url, "application/json", body)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (r *JSONClient) Put(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.Client.Put(url, "application/json", body)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (r *JSONClient) Delete(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.Client.Delete(url)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}
