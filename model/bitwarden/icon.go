package bitwarden

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
)

const cacheTTL = 7 * 24 * time.Hour // 1 week

var iconClient = &http.Client{
	Timeout: 10 * time.Second,
}

// Icon is a simple struct with a content-type and the content of an icon.
type Icon struct {
	Mime string `json:"mime"`
	Body []byte `json:"body"`
}

// GetIcon returns an icon for the given domain.
func GetIcon(domain string) (*Icon, error) {
	if err := validateDomain(domain); err != nil {
		return nil, err
	}

	cache := config.GetConfig().CacheStorage
	key := "bw-icons:" + domain
	if data, ok := cache.Get(key); ok {
		if len(data) == 0 {
			return nil, errors.New("No icon")
		}
		icon := &Icon{}
		if err := json.Unmarshal(data, icon); err != nil {
			return nil, err
		}
		return icon, nil
	}

	icon, err := downloadFavicon(domain)
	if err != nil {
		cache.Set(key, nil, cacheTTL)
	} else {
		if data, err := json.Marshal(icon); err == nil {
			cache.Set(key, data, cacheTTL)
		}
	}
	return icon, err
}

func validateDomain(domain string) error {
	if domain == "" || len(domain) > 255 || strings.Contains(domain, "..") {
		return errors.New("Unauthorized domain")
	}

	for _, c := range domain {
		if c == ' ' || !strconv.IsPrint(c) {
			return errors.New("Invalid domain")
		}
	}

	if _, _, err := net.ParseCIDR(domain + "/24"); err == nil {
		return errors.New("IP address are not authorized")
	}

	return nil
}

func downloadFavicon(domain string) (*Icon, error) {
	icon, err := downloadIcon("https://" + domain + "/favicon.ico")
	if err == nil {
		return icon, nil
	}
	// Try again
	time.Sleep(1 * time.Second)
	return downloadIcon("https://" + domain + "/favicon.ico")
}

func downloadIcon(u string) (*Icon, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := iconClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("Not status OK")
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	ico := Icon{
		Mime: res.Header.Get("Content-Type"),
		Body: b,
	}
	return &ico, nil
}
