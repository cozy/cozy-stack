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
		Name       string `json:"name"`
		NamePrefix string `json:"name_prefix,omitempty"`
		Editor     string `json:"editor"`
		Icon       string `json:"icon"`

		Type        string           `json:"type,omitempty"`
		License     string           `json:"license,omitempty"`
		Language    string           `json:"language,omitempty"`
		Category    string           `json:"category,omitempty"`
		VendorLink  string           `json:"vendor_link"`
		Locales     *json.RawMessage `json:"locales,omitempty"`
		Langs       *json.RawMessage `json:"langs,omitempty"`
		Platforms   *json.RawMessage `json:"platforms,omitempty"`
		Categories  *json.RawMessage `json:"categories,omitempty"`
		Developer   *json.RawMessage `json:"developer,omitempty"`
		Screenshots *json.RawMessage `json:"screenshots,omitempty"`
		Tags        *json.RawMessage `json:"tags,omitempty"`

		Frequency    string           `json:"frequency,omitempty"`
		DataTypes    *json.RawMessage `json:"data_types,omitempty"`
		Doctypes     *json.RawMessage `json:"doctypes,omitempty"`
		Fields       *json.RawMessage `json:"fields,omitempty"`
		Messages     *json.RawMessage `json:"messages,omitempty"`
		OAuth        *json.RawMessage `json:"oauth,omitempty"`
		TimeInterval *json.RawMessage `json:"time_interval,omitempty"`

		Slug             string           `json:"slug"`
		State            string           `json:"state"`
		Source           string           `json:"source"`
		Version          string           `json:"version"`
		Permissions      *permissions.Set `json:"permissions"`
		AvailableVersion string           `json:"available_version,omitempty"`

		Parameters json.RawMessage `json:"parameters,omitempty"`

		Intents []struct {
			Action string   `json:"action"`
			Types  []string `json:"type"`
			Href   string   `json:"href"`
		} `json:"intents"`

		Routes *map[string]struct {
			Folder string `json:"folder"`
			Index  string `json:"index"`
			Public bool   `json:"public"`
		} `json:"routes,omitempty"`

		Services *map[string]struct {
			Type           string `json:"type"`
			File           string `json:"file"`
			Debounce       string `json:"debounce"`
			TriggerOptions string `json:"trigger"`
			TriggerID      string `json:"trigger_id"`
		} `json:"services"`
		Notifications map[string]struct {
			Description     string            `json:"description,omitempty"`
			Collapsible     bool              `json:"collapsible,omitempty"`
			Multiple        bool              `json:"multiple,omitempty"`
			Stateful        bool              `json:"stateful,omitempty"`
			DefaultPriority string            `json:"default_priority,omitempty"`
			TimeToLive      time.Duration     `json:"time_to_live,omitempty"`
			Templates       map[string]string `json:"templates,omitempty"`
			MinInterval     time.Duration     `json:"min_interval,omitempty"`
		} `json:"notifications,omitempty"`

		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`

		Error string `json:"error,omitempty"`
	} `json:"attributes,omitempty"`
}

// AppOptions holds the options to install an application.
type AppOptions struct {
	AppType             string
	Slug                string
	SourceURL           string
	Deactivated         bool
	OverridenParameters *json.RawMessage
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
	q := url.Values{
		"Source":      {opts.SourceURL},
		"Deactivated": {strconv.FormatBool(opts.Deactivated)},
	}
	if opts.OverridenParameters != nil {
		b, err := json.Marshal(opts.OverridenParameters)
		if err != nil {
			return nil, err
		}
		q["Parameters"] = []string{string(b)}
	}
	res, err := c.Req(&request.Options{
		Method:  "POST",
		Path:    makeAppsPath(opts.AppType, url.PathEscape(opts.Slug)),
		Queries: q,
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
	q := url.Values{
		"Source":           {opts.SourceURL},
		"PermissionsAcked": {strconv.FormatBool(true)},
	}
	if opts.OverridenParameters != nil {
		b, err := json.Marshal(opts.OverridenParameters)
		if err != nil {
			return nil, err
		}
		q["Parameters"] = []string{string(b)}
	}
	res, err := c.Req(&request.Options{
		Method:  "PUT",
		Path:    makeAppsPath(opts.AppType, url.PathEscape(opts.Slug)),
		Queries: q,
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
