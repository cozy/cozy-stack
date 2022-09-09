package bi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/pkg/safehttp"
)

type apiClient struct {
	host string
}

func newApiClient(rawURL string) (*apiClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if strings.Contains(u.Host, ":") {
		return nil, errors.New("port not allowed in BI url")
	}
	return &apiClient{host: u.Host}, nil
}

func (c *apiClient) makeRequest(verb, path, token string, body io.Reader) (*http.Response, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   c.host,
		Path:   path,
	}
	req, err := http.NewRequest(verb, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := safehttp.ClientWithKeepAlive.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *apiClient) get(path, token string) (*http.Response, error) {
	return c.makeRequest(http.MethodGet, path, token, nil)
}

func (c *apiClient) delete(path, token string) (*http.Response, error) {
	return c.makeRequest(http.MethodDelete, path, token, nil)
}

type connectionsResponse struct {
	Total int `json:"total"`
}

func (c *apiClient) getNumberOfConnections(token string) (int, error) {
	res, err := c.get("/users/me/connections", token)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return 0, fmt.Errorf("/users/me/connections received response code %d", res.StatusCode)
	}

	var data connectionsResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return 0, err
	}
	return data.Total, nil
}

func (c *apiClient) deleteUser(token string) error {
	res, err := c.delete("/users/me", token)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if 200 <= res.StatusCode && res.StatusCode < 300 {
		return nil
	}
	return errors.New("invalid response from BI API")
}

func (c *apiClient) getConnectorUUID(connectionID int, token string) (string, error) {
	path := fmt.Sprintf("/connections/%d", connectionID)
	res, err := c.get(path, token)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return "", err
	}
	if uuid, ok := data["connector_uuid"].(string); ok {
		return uuid, nil
	}
	return "", errors.New("invalid response from BI API")
}
