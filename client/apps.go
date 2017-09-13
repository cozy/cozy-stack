package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// AppManifest holds the JSON-API representation of an application.
type AppManifest struct {
	ID    string `json:"id"`
	Rev   string `json:"rev"`
	Attrs struct {
		Name        string    `json:"name"`
		Editor      string    `json:"editor"`
		Slug        string    `json:"slug"`
		Source      string    `json:"source"`
		State       string    `json:"state"`
		Error       string    `json:"error,omitempty"`
		Icon        string    `json:"icon"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		Category    string    `json:"category"`
		Description string    `json:"description"`
		Developer   struct {
			Name string `json:"name"`
			URL  string `json:"url,omitempty"`
		} `json:"developer"`

		DefaultLocale string `json:"default_locale"`
		Locales       map[string]struct {
			Description string `json:"description"`
		} `json:"locales"`

		Version     string           `json:"version"`
		License     string           `json:"license"`
		Permissions *permissions.Set `json:"permissions"`
		Routes      *map[string]struct {
			Folder string `json:"folder"`
			Index  string `json:"index"`
			Public bool   `json:"public"`
		} `json:"routes,omitempty"`

		Services *struct {
			Type           string `json:"type"`
			File           string `json:"file"`
			Debounce       string `json:"debounce"`
			TriggerOptions string `json:"trigger"`
			TriggerID      string `json:"trigger_id"`
		} `json:"services"`
	} `json:"attributes,omitempty"`
}

// AppOptions holds the options to install an application.
type AppOptions struct {
	AppType     string
	Slug        string
	SourceURL   string
	Deactivated bool
}

// ListApps is used to get the list of all installed applications.
func (c *Client) ListApps(appType string) ([]*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   makeAppsPath(appType, ""),
	})
	if err != nil {
		return nil, err
	}
	var mans []*AppManifest
	if err := readJSONAPI(res.Body, &mans); err != nil {
		return nil, err
	}
	return mans, nil
}

// GetApp is used to fetch an application manifest with specified slug
func (c *Client) GetApp(opts *AppOptions) (*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   makeAppsPath(opts.AppType, url.PathEscape(opts.Slug)),
	})
	if err != nil {
		return nil, err
	}
	return readAppManifest(res)
}

// InstallApp is used to install an application.
func (c *Client) InstallApp(opts *AppOptions) (*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method: "POST",
		Path:   makeAppsPath(opts.AppType, url.PathEscape(opts.Slug)),
		Queries: url.Values{
			"Source":      {opts.SourceURL},
			"Deactivated": {strconv.FormatBool(opts.Deactivated)},
		},
		Headers: request.Headers{
			"Accept": "text/event-stream",
		},
	})
	if err != nil {
		return nil, err
	}
	return readAppManifestStream(res)
}

// UpdateApp is used to update an application.
func (c *Client) UpdateApp(opts *AppOptions) (*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method:  "PUT",
		Path:    makeAppsPath(opts.AppType, url.PathEscape(opts.Slug)),
		Queries: url.Values{"Source": {opts.SourceURL}},
		Headers: request.Headers{
			"Accept": "text/event-stream",
		},
	})
	if err != nil {
		return nil, err
	}
	return readAppManifestStream(res)
}

// UninstallApp is used to uninstall an application.
func (c *Client) UninstallApp(opts *AppOptions) (*AppManifest, error) {
	res, err := c.Req(&request.Options{
		Method: "DELETE",
		Path:   makeAppsPath(opts.AppType, url.PathEscape(opts.Slug)),
	})
	if err != nil {
		return nil, err
	}
	return readAppManifest(res)
}

func makeAppsPath(appType, path string) string {
	switch appType {
	case consts.Apps:
		return "/apps/" + path
	case consts.Konnectors:
		return "/konnectors/" + path
	}
	panic(fmt.Errorf("Unknown application type %s", appType))
}

func readAppManifestStream(res *http.Response) (*AppManifest, error) {
	evtch := make(chan *request.SSEEvent)
	go request.ReadSSE(res.Body, evtch)
	var lastevt *request.SSEEvent
	// get the last sent event
	for evt := range evtch {
		if evt.Error != nil {
			return nil, evt.Error
		}
		if evt.Name == "error" {
			var stringError string
			if err := json.Unmarshal(evt.Data, &stringError); err != nil {
				return nil, fmt.Errorf("Could not parse error from event-stream: %s", err.Error())
			}
			return nil, errors.New(stringError)
		}
		lastevt = evt
	}
	if lastevt == nil {
		return nil, errors.New("No application data was sent")
	}
	app := &AppManifest{}
	if err := readJSONAPI(bytes.NewReader(lastevt.Data), &app); err != nil {
		return nil, err
	}
	return app, nil
}

func readAppManifest(res *http.Response) (*AppManifest, error) {
	app := &AppManifest{}
	if err := readJSONAPI(res.Body, &app); err != nil {
		return nil, err
	}
	return app, nil
}
