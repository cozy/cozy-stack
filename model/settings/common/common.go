package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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
	u.Path = "/api/admin/user/settings"
	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	res, err := safehttp.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusBadRequest:
		return fmt.Errorf("bad request: invalid data")
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: missing or invalid token")
	default:
		return fmt.Errorf("unexpected response status: %d", res.StatusCode)
	}
}

// UpdateCommonSettings updates user settings for an instance via the common
// settings API. The common settings version is increased on the instance, but
// it's the caller's responsibility to persist it when the bool returned is true
// (aka a common settings call has been made).
func UpdateCommonSettings(inst *instance.Instance, settings *couchdb.JSONDoc) (bool, error) {
	cfg := getCommonSettings(inst)
	if cfg == nil {
		return false, nil
	}

	if inst.CommonSettingsVersion == 0 {
		CreateCommonSettings(inst, settings)

		return true, nil
	}
	inst.CommonSettingsVersion++
	request := buildRequest(inst, settings)
	requestBody, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	u, err := url.Parse(cfg.URL)
	u.Path = fmt.Sprintf("/api/admin/user/settings/%s", request.Nickname)
	req, err := http.NewRequest("PUT", u.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	res, err := safehttp.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusBadRequest:
		return false, fmt.Errorf("bad request: invalid data")
	case http.StatusUnauthorized:
		return false, fmt.Errorf("unauthorized: missing or invalid token")
	default:
		return false, fmt.Errorf("unexpected response status: %d", res.StatusCode)
	}
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
	u.Path = fmt.Sprintf("/api/admin/user/settings/%s", request.Nickname)
	req, err := http.NewRequest("PUT", u.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	res, err := safehttp.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusBadRequest:
		return false, fmt.Errorf("bad request: invalid data")
	case http.StatusUnauthorized:
		return false, fmt.Errorf("unauthorized: missing or invalid token")
	default:
		return false, fmt.Errorf("unexpected response status: %d", res.StatusCode)
	}
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
