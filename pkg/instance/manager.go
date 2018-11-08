package instance

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"

	"github.com/cozy/cozy-stack/pkg/ws"

	"github.com/mitchellh/mapstructure"
)

var managerClient = &http.Client{Timeout: 30 * time.Second}

// ManagerURLKind is an enum type for the different kinds of manager URLs.
type ManagerURLKind int

const (
	// ManagerTOSURL is the kind for changes of TOS URL.
	ManagerTOSURL ManagerURLKind = iota
	// ManagerPremiumURL is the kind for changing the account type of the
	// instance.
	ManagerPremiumURL
	// ManagerBlockedURL is the kind for a redirection of a blocked instance.
	ManagerBlockedURL
)

type managerConfig struct {
	API *struct {
		URL   string
		Token string
	}
}

func (i *Instance) getManagerConfig() *managerConfig {
	contexts := config.GetConfig().Clouderies
	context, ok := i.getFromContexts(contexts)
	if !ok {
		return nil
	}

	var config managerConfig
	err := mapstructure.Decode(context, &config)
	if err != nil {
		return nil
	}

	return &config
}

func (i *Instance) managerClient() *ws.OAuthRestJSONClient {
	config := i.getManagerConfig()
	if config == nil {
		return nil
	}

	api := config.API
	if api == nil {
		return nil
	}

	url := api.URL
	token := api.Token
	if url == "" || token == "" {
		return nil
	}

	client := &ws.OAuthRestJSONClient{}
	client.Init(url, token)
	return client
}

func (i *Instance) managerUpdateSettings(changes map[string]interface{}) {
	if i.UUID == "" || len(changes) == 0 {
		return
	}

	client := i.managerClient()
	if client == nil {
		return
	}

	url := fmt.Sprintf("/api/v1/instances/%s", url.PathEscape(i.UUID))
	err := client.Put(url, changes, nil)
	if err != nil {
		i.Logger().Errorf("Error during cloudery settings update %s", err)
	}
}

// ManagerURL returns an external string for the given ManagerURL kind.
func (i *Instance) ManagerURL(k ManagerURLKind) (string, error) {
	if i.UUID == "" {
		return "", nil
	}

	config, err := i.SettingsContext()
	if err != nil {
		return "", nil
	}

	base, ok := config["manager_url"]
	if !ok {
		return "", nil
	}

	baseURL, err := url.Parse(base.(string))
	if err != nil {
		return "", err
	}

	var path string
	switch k {
	// TODO: we may want to rely on the contexts to avoid hardcoding the path
	// values of these kinds.
	case ManagerPremiumURL:
		path = fmt.Sprintf("/cozy/instances/%s/premium", url.PathEscape(i.UUID))
	case ManagerTOSURL:
		path = fmt.Sprintf("/cozy/instances/%s/tos", url.PathEscape(i.UUID))
	case ManagerBlockedURL:
		path = fmt.Sprintf("/cozy/instances/%s/blocked", url.PathEscape(i.UUID))
	default:
		panic("unknown ManagerURLKind")
	}
	baseURL.Path = path

	return baseURL.String(), nil
}

// ManagerSignTOS make a request to the manager in order to finalize the TOS
// signing flow.
func (i *Instance) ManagerSignTOS(originalReq *http.Request) error {
	if i.TOSLatest == "" {
		return nil
	}
	split := strings.SplitN(i.TOSLatest, "-", 2)
	if len(split) != 2 {
		return nil
	}
	u, err := i.ManagerURL(ManagerTOSURL)
	if err != nil {
		return Patch(i, &Options{TOSSigned: i.TOSLatest})
	}
	form := url.Values{"version": {split[0]}}
	res, err := doManagerRequest(http.MethodPut, u, form, originalReq)
	if err != nil {
		return err
	}
	return res.Body.Close()
}

func doManagerRequest(method string, url string, form url.Values, originalReq *http.Request) (*http.Response, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if form != nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}
	if originalReq != nil {
		var ip string
		if forwardedFor := req.Header.Get("X-Forwarded-For"); forwardedFor != "" {
			ip = strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
		}
		if ip == "" {
			ip = req.RemoteAddr
		}
		req.Header.Set("X-Forwarded-For", ip)
	}
	return managerClient.Do(req)
}
