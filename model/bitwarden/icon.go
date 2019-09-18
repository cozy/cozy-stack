package bitwarden

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"golang.org/x/net/html"
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

	icon, err := fetchIcon(domain)
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

func fetchIcon(domain string) (*Icon, error) {
	if html, err := getPage(domain); err == nil {
		candidates := getCandidateIcons(domain, html)
		html.Close()
		for _, candidate := range candidates {
			if icon, err := downloadIcon(candidate); err == nil {
				return icon, nil
			}
		}
	}
	return downloadFavicon(domain)
}

func getPage(domain string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, "https://"+domain, nil)
	if err != nil {
		return nil, err
	}
	res, err := iconClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, errors.New("Not status OK")
	}
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if !strings.Contains(ct, "text/html") {
		res.Body.Close()
		return nil, errors.New("Not html")
	}
	return res.Body, nil
}

func getCandidateIcons(domain string, r io.Reader) []string {
	tokenizer := html.NewTokenizer(r)
	var candidates []string

	// Consider only the first 1000 tokens, as the candidates icons must be in
	// the <head>, and it avoid reading the whole html page.
	for i := 0; i < 1000; i++ {
		switch tokenizer.Next() {
		case html.ErrorToken:
			// End of the document, we're done
			break
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tokenizer.Token()
			if !isLinkIcon(t) {
				continue
			}
			if u := getHref(domain, t); u != "" {
				candidates = append(candidates, u)
			}
		}
	}

	return candidates
}

func isLinkIcon(t html.Token) bool {
	if strings.ToLower(t.Data) != "link" {
		return false
	}
	for _, attr := range t.Attr {
		if strings.ToLower(attr.Key) == "rel" {
			vals := strings.Split(strings.ToLower(attr.Val), " ")
			for _, val := range vals {
				if val == "icon" || val == "apple-touch-icon" {
					return true
				}
			}
		}
	}
	return false
}

func getHref(domain string, t html.Token) string {
	href := ""
	for _, attr := range t.Attr {
		if strings.ToLower(attr.Key) == "href" {
			href = attr.Val
			break
		}
	}
	if strings.Contains(href, "://") {
		return href
	}
	return "https://" + domain + "/" + strings.TrimPrefix(href, "/")
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
