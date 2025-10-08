package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/safehttp"
)

// DefaultTimezone is used when no timezone is specified, as this parameter is
// required.
const DefaultTimezone = "Europe/Paris"

// UserSettingsPayload represents the payload structure for user settings
type UserSettingsPayload struct {
	Language    string `json:"language,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	FirstName   string `json:"first_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	MatrixID    string `json:"matrix_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// UserSettingsRequest represents the complete request structure
type UserSettingsRequest struct {
	Source    string              `json:"source"`
	Nickname  string              `json:"nickname"`
	RequestID string              `json:"request_id"`
	Timestamp int64               `json:"timestamp"`
	Version   int                 `json:"version"`
	Payload   UserSettingsPayload `json:"payload"`
}

// DoCommonHTTP is the indirection used to perform HTTP calls to the common
// settings API. Tests can override this variable to stub network calls.
var DoCommonHTTP = doCommonSettingsRequest

// DoCommonHTTPResp allows tests to override the HTTP caller for GET requests
// that returns the decoded remote settings with its Version field.
var DoCommonHTTPResp = doCommonSettingsRequestResp

var GetRemoteCommonSettings = getRemoteCommonSettings

// CreateCommonSettings creates user settings for an instance via the common
// settings API. The common settings version is increased on the instance, but
// it's the caller's responsibility to persist it.
func CreateCommonSettings(inst *instance.Instance, settings *couchdb.JSONDoc) error {
	cfg := getCommonSettings(inst)
	if cfg == nil {
		return nil
	}

	inst.CommonSettingsVersion = 1
	request := buildRequest(inst, settings)
	addAvatarURL(inst, &request)
	requestBody, err := json.Marshal(request)
	if err != nil {
		return err
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return err
	}
	u.Path = "/api/admin/user/settings"
	inst.Logger().WithNamespace("common_settings").WithDomain(inst.Domain).
		Debugf("HTTP %s %s v=%d payload=%+v", "POST", u.String(), inst.CommonSettingsVersion, request.Payload)
	return DoCommonHTTP("POST", u.String(), cfg.Token, requestBody)
}

// UpdateCommonSettings updates user settings for an instance via the common
// settings API. The common settings version is increased on the instance, but
// it's the caller's responsibility to persist it when the bool returned is true
// (aka a common settings call has been made).
func UpdateCommonSettings(inst *instance.Instance, settings *couchdb.JSONDoc) (bool, error) {
	log := inst.Logger().WithNamespace("common_settings").WithDomain(inst.Domain)
	cfg := getCommonSettings(inst)
	if cfg == nil {
		return false, nil
	}

	if inst.CommonSettingsVersion == 0 {
		err := CreateCommonSettings(inst, settings)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	// Build the request we intend to send ("new" settings to save)
	request := buildRequest(inst, settings)

	// Check remote version, and when out of sync, compare remote payload with
	// the new payload we want to save to help diagnose conflicts.
	if remote, err := GetRemoteCommonSettings(inst); err != nil {
		return false, err
	} else if remote != nil && remote.Version != inst.CommonSettingsVersion {
		diffs := make([]string, 0, 8)
		addDiff := func(name, lv, rv string) {
			if lv != rv {
				diffs = append(diffs, fmt.Sprintf("%s: '%s' != '%s'", name, lv, rv))
			}
		}
		if tz, ok := settings.M["tz"].(string); ok {
			addDiff("timezone", remote.Payload.Timezone, tz)
		}
		if name, ok := settings.M["public_name"].(string); ok {
			addDiff("display_name", remote.Payload.DisplayName, name)
		}
		if email, ok := settings.M["email"].(string); ok {
			addDiff("email", remote.Payload.Email, email)
		}

		log.Warnf("common settings out of sync: local=%d remote=%d", inst.CommonSettingsVersion, remote.Version)
		if len(diffs) > 0 {
			log.Warn("diffs: " + strings.Join(diffs, "; "))
		} else {
			log.Warn("no changes in remote and local versions, skip common settings version bump")
		}
		return false, errors.New("common settings version mismatch")
	}

	inst.CommonSettingsVersion++

	requestBody, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return false, err
	}
	u.Path = fmt.Sprintf("/api/admin/user/settings/%s", request.Nickname)
	log.
		Debugf("HTTP %s %s v=%d payload=%+v", "PUT", u.String(), inst.CommonSettingsVersion, request.Payload)
	if err := DoCommonHTTP("PUT", u.String(), cfg.Token, requestBody); err != nil {
		return false, err
	}
	return true, nil
}

// UpdateAvatar updates user settings for an instance via the common settings
// API when the avatar has been updated. The common settings version is
// increased on the instance, but it's the caller's responsibility to persist it
// when the bool returned is true (aka a common settings call has been made).
func UpdateAvatar(inst *instance.Instance) (bool, error) {
	cfg := getCommonSettings(inst)
	if cfg == nil {
		return false, nil
	}

	// Ensure local version is in sync with remote before updating
	if err := CheckCommonSettingsVersionSync(inst); err != nil {
		inst.Logger().WithNamespace("common_settings").WithDomain(inst.Domain).
			Errorf("Version mismatch before avatar update: %v", err)
		return false, err
	}
	inst.CommonSettingsVersion++
	parts := strings.Split(inst.Domain, ".")
	nickname := parts[0]
	requestID := fmt.Sprintf("%s_%d", inst.Domain, time.Now().UnixNano())
	request := UserSettingsRequest{
		Source:    "cozy-stack",
		Nickname:  nickname,
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
		Version:   inst.CommonSettingsVersion,
	}
	addAvatarURL(inst, &request)
	requestBody, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return false, err
	}
	u.Path = fmt.Sprintf("/api/admin/user/settings/%s", request.Nickname)
	inst.Logger().WithNamespace("common_settings").WithDomain(inst.Domain).
		Debugf("HTTP %s %s v=%d avatar_url=%s", "PUT", u.String(), inst.CommonSettingsVersion, request.Payload.Avatar)
	if err := DoCommonHTTP("PUT", u.String(), cfg.Token, requestBody); err != nil {
		return false, err
	}
	return true, nil
}

// GetRemoteCommonSettings fetches user settings for an instance from the common
// settings API using a GET request.
//
// It returns a populated UserSettingsPayload (as returned by the remote API)
// on success. If the common settings are not configured for the context, it
// returns (nil, nil).
func getRemoteCommonSettings(inst *instance.Instance) (*UserSettingsRequest, error) {
	cfg := getCommonSettings(inst)
	if cfg == nil {
		return nil, nil
	}

	parts := strings.Split(inst.Domain, ".")
	nickname := parts[0]

	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	u.Path = fmt.Sprintf("/api/admin/user/settings/%s", nickname)

	inst.Logger().WithNamespace("common_settings").WithDomain(inst.Domain).
		Debugf("HTTP %s %s", http.MethodGet, u.String())

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	res, err := safehttp.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, mapStatusError(res.StatusCode)
	}
	defer res.Body.Close()
	var remote UserSettingsRequest
	if err := json.NewDecoder(res.Body).Decode(&remote); err != nil {
		return nil, err
	}
	return &remote, nil
}

// CheckCommonSettingsVersionSync ensures local and remote common settings
// versions are equal; returns nil if not configured or remote missing.
func CheckCommonSettingsVersionSync(inst *instance.Instance) error {
	cfg := getCommonSettings(inst)
	if cfg == nil {
		return nil
	}
	remote, err := GetRemoteCommonSettings(inst)
	if err != nil {
		return err
	}
	if remote == nil {
		return nil
	}
	if remote.Version != inst.CommonSettingsVersion {
		// Build local payload to compare fields with remote
		localDoc, lerr := inst.SettingsDocument()
		if lerr != nil {
			return fmt.Errorf("common settings out of sync: local=%d remote=%d (failed to load local settings: %v)", inst.CommonSettingsVersion, remote.Version, lerr)
		}
		localReq := buildRequest(inst, localDoc)

		// Collect differences field by field
		diffs := make([]string, 0, 8)
		addDiff := func(name, lv, rv string) {
			if lv != rv {
				diffs = append(diffs, fmt.Sprintf("%s: '%s' != '%s'", name, lv, rv))
			}
		}
		addDiff("language", localReq.Payload.Language, remote.Payload.Language)
		addDiff("timezone", localReq.Payload.Timezone, remote.Payload.Timezone)
		addDiff("display_name", localReq.Payload.DisplayName, remote.Payload.DisplayName)
		addDiff("first_name", localReq.Payload.FirstName, remote.Payload.FirstName)
		addDiff("last_name", localReq.Payload.LastName, remote.Payload.LastName)
		addDiff("email", localReq.Payload.Email, remote.Payload.Email)
		addDiff("phone", localReq.Payload.Phone, remote.Payload.Phone)
		addDiff("matrix_id", localReq.Payload.MatrixID, remote.Payload.MatrixID)
		// Avatar URL contains version query param; still useful for diagnostics
		addDiff("avatar", localReq.Payload.Avatar, remote.Payload.Avatar)

		if len(diffs) == 0 {
			return fmt.Errorf("common settings out of sync: local=%d remote=%d (no payload diffs)", inst.CommonSettingsVersion, remote.Version)
		}
		return fmt.Errorf("common settings out of sync: local=%d remote=%d; diffs: %s", inst.CommonSettingsVersion, remote.Version, strings.Join(diffs, "; "))
	}
	return nil
}

func getCommonSettings(inst *instance.Instance) *config.CommonSettings {
	commonSettings := config.GetCommonSettings()
	if len(commonSettings) == 0 {
		return nil
	}
	cfg, ok := commonSettings[inst.ContextName]
	if !ok {
		cfg, ok = commonSettings[config.DefaultInstanceContext]
	}
	if !ok || cfg.URL == "" {
		return nil
	}
	return &cfg
}

func buildRequest(inst *instance.Instance, settings *couchdb.JSONDoc) UserSettingsRequest {
	parts := strings.Split(inst.Domain, ".")
	nickname := parts[0]
	requestID := fmt.Sprintf("%s_%d", inst.Domain, time.Now().UnixNano())
	request := UserSettingsRequest{
		Source:    "cozy-stack",
		Nickname:  nickname,
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
		Version:   inst.CommonSettingsVersion,
		Payload: UserSettingsPayload{
			Language: inst.Locale,
			Timezone: DefaultTimezone,
		},
	}

	if tz, ok := settings.M["tz"].(string); ok {
		request.Payload.Timezone = tz
	}
	if name, ok := settings.M["public_name"].(string); ok {
		request.Payload.DisplayName = name
		parts := strings.Split(name, " ")
		request.Payload.FirstName = parts[0]
		request.Payload.LastName = parts[len(parts)-1]
	}
	if email, ok := settings.M["email"].(string); ok {
		request.Payload.Email = email
	}
	if phone, ok := settings.M["phone"].(string); ok {
		request.Payload.Phone = phone
	}
	if len(parts) > 1 {
		id := fmt.Sprintf("@%s:%s", nickname, strings.Join(parts[1:], "."))
		request.Payload.MatrixID = id
	}

	return request
}

func addAvatarURL(inst *instance.Instance, request *UserSettingsRequest) {
	avatarURL := inst.PageURL("/public/avatar", url.Values{
		"v": {fmt.Sprintf("%d", inst.CommonSettingsVersion+1)},
	})
	request.Payload.Avatar = avatarURL
}

// doCommonSettingsRequest sends an HTTP request to the common settings API and
// maps status codes to errors.
func doCommonSettingsRequest(method, urlStr, token string, body []byte) error {
	req, err := http.NewRequest(method, urlStr, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := safehttp.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	logger.WithNamespace("common_settings").
		Debugf("HTTP %s %s -> status=%d", method, urlStr, res.StatusCode)

	return mapStatusError(res.StatusCode)
}

// doCommonSettingsRequestResp performs the HTTP request and returns the
// response to the caller when status is 200. On non-200, it returns a mapped
// error and closes the response body.
func doCommonSettingsRequestResp(urlStr, token string) (*UserSettingsPayload, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := safehttp.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	logger.WithNamespace("common_settings").
		Debugf("HTTP GET %s -> status=%d", urlStr, res.StatusCode)

	var settings *UserSettingsPayload
	if res.StatusCode == http.StatusOK {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(body, &settings)
		return settings, err
	}
	defer res.Body.Close()
	return nil, mapStatusError(res.StatusCode)
}

func mapStatusError(status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusBadRequest:
		return fmt.Errorf("bad request: invalid data")
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: missing or invalid token")
	case http.StatusNotFound:
		return fmt.Errorf("not found")
	default:
		return fmt.Errorf("unexpected response status: %d", status)
	}
}
