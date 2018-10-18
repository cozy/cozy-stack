package instance

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/utils"
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

// ManagerURL returns an external string for the given ManagerURL kind.
func (i *Instance) ManagerURL(k ManagerURLKind) (s string, ok bool) {
	defer func() {
		if !ok {
			s = i.PageURL("/manager_url_is_not_specified", nil)
		}
	}()

	if i.UUID == "" {
		return
	}

	if i.managerURL == nil {
		ctx, err := i.SettingsContext()
		if err != nil {
			return
		}
		var u string
		u, ok = ctx["manager_url"].(string)
		if !ok {
			return
		}
		i.managerURL, err = url.Parse(u)
		if err != nil {
			return
		}
	}

	managerURL := utils.CloneURL(i.managerURL)
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
	managerURL.Path = path

	s = managerURL.String()
	ok = true
	return
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
	u, ok := i.ManagerURL(ManagerTOSURL)
	if !ok {
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
