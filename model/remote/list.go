package remote

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/httpcache"
)

var listClient = &http.Client{
	Timeout:   20 * time.Second,
	Transport: httpcache.NewMemoryCacheTransport(32),
}

// https://docs.github.com/en/rest/repos/contents#get-repository-content
const listURL = "https://api.github.com/repos/cozy/cozy-doctypes/contents/"

type listEntries struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ListDoctypes returns the list of the known remote doctypes.
func ListDoctypes(inst *instance.Instance) ([]string, error) {
	var doctypes []string
	if config.GetConfig().Doctypes == "" {
		req, err := http.NewRequest(http.MethodGet, listURL, nil)
		if err != nil {
			return nil, err
		}
		res, err := listClient.Do(req)
		if err != nil {
			log.Warnf("cannot list doctypes: %s", err)
			return nil, ErrNotFoundRemote
		}
		defer res.Body.Close()
		var entries []listEntries
		if err := json.NewDecoder(res.Body).Decode(&entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Type != "dir" {
				continue
			}
			if entry.Name == "docs" || strings.HasPrefix(entry.Name, ".") {
				continue
			}
			doctypes = append(doctypes, entry.Name)
		}
	} else {
		entries, err := os.ReadDir(config.GetConfig().Doctypes)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == "docs" || strings.HasPrefix(name, ".") {
				continue
			}
			doctypes = append(doctypes, name)
		}
	}
	return doctypes, nil
}
