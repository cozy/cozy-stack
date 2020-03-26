package bitwarden

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
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
	candidates := make(map[string]int)

	// Consider only the first 1000 tokens, as the candidates icons must be in
	// the <head>, and it avoid reading the whole html page.
	for i := 0; i < 1000; i++ {
		switch tokenizer.Next() {
		case html.ErrorToken:
			// End of the document, we're done
			break
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tokenizer.Token()
			if u, p := getLinkIcon(domain, t); p >= 0 {
				candidates[u] = p
			}
		}
	}

	sorted := make([]string, 0, len(candidates))
	for k := range candidates {
		sorted = append(sorted, k)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return candidates[sorted[i]] > candidates[sorted[j]]
	})
	return sorted
}

// getLinkIcon returns the href and the priority for the link.
// -1 means that it is not a suitable icon link.
// Higher priority is better.
func getLinkIcon(domain string, t html.Token) (string, int) {
	if strings.ToLower(t.Data) != "link" {
		return "", -1
	}

	isIcon := false
	href := ""
	priority := 100
	for _, attr := range t.Attr {
		switch strings.ToLower(attr.Key) {
		case "rel":
			vals := strings.Split(strings.ToLower(attr.Val), " ")
			for _, val := range vals {
				if val == "icon" || val == "apple-touch-icon" {
					isIcon = true
					if val == "icon" {
						priority += 10
					}
				}
			}

		case "href":
			href = attr.Val
			if strings.HasSuffix(href, ".png") {
				priority += 2
			}

		case "sizes":
			w, h := parseSizes(attr.Val)
			if w != h {
				priority -= 100
			} else if w == 32 {
				priority += 400
			} else if w == 64 {
				priority += 300
			} else if w >= 24 && w <= 128 {
				priority += 200
			} else if w == 16 {
				priority += 100
			}
		}
	}

	if !isIcon || href == "" {
		return "", -1
	}
	if !strings.Contains(href, "://") {
		href = strings.TrimPrefix(href, "./")
		href = strings.TrimPrefix(href, "/")
		href = "https://" + domain + "/" + href
	}
	return href, priority
}

func parseSizes(val string) (int, int) {
	parts := strings.Split(val, "x")
	if len(parts) != 2 {
		return 0, 0
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0
	}
	return w, h
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
	if strings.Split(ico.Mime, "/")[0] != "image" {
		return nil, errors.New("Invalid mime-type")
	}
	return &ico, nil
}
