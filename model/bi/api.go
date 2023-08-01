package bi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
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

func (c *apiClient) getNumberOfConnections(inst *instance.Instance, token string) (int, error) {
	res, err := c.get("/users/me/connections", token)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return 0, fmt.Errorf("/users/me/connections received response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}
	var data connectionsResponse
	if err := json.Unmarshal(body, &data); err != nil {
		// Truncate the body for the log message if too long
		msg := string(body)
		if len(msg) > 200 {
			msg = msg[0:198] + "..."
		}
		log := inst.Logger().WithNamespace("bi")
		log.Warnf("getNumberOfConnections [%d] cannot parse JSON %s: %s", res.StatusCode, msg, err)
		if log.IsDebug() {
			log.Debugf("getNumberOfConnections called with token %s", token)
			logFullHTML(log, string(body))
		}
		return 0, err
	}
	return data.Total, nil
}

func logFullHTML(log *logger.Entry, msg string) {
	i := 0
	for len(msg) > 0 {
		idx := len(msg)
		if idx > 1800 {
			idx = 1800
		}
		part := msg[:idx]
		log.Debugf("getNumberOfConnections %d: %s", i, part)
		i++
		msg = msg[idx:]
	}
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
	path := fmt.Sprintf("/users/me/connections/%d", connectionID)
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
