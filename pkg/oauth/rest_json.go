package oauth

import (
	"encoding/json"
)

type RestJSON struct {
	*Rest
}

func (r *RestJSON) Init(baseURL string, token string) {
	r.Rest = &Rest{}
	r.Rest.Init(baseURL, token)
}

func (r *RestJSON) Get(url string, result interface{}) error {
	body, err := r.Rest.Get(url)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (r *RestJSON) Post(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.Rest.Post(url, "application/json", body)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (r *RestJSON) Put(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.Rest.Put(url, "application/json", body)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (r *RestJSON) Delete(url string, params interface{}, result interface{}) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body, err = r.Rest.Delete(url)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}
