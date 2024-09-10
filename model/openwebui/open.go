package openwebui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/hkdf"
)

type OpenURL struct {
	DocID string `json:"id,omitempty"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

func (o *OpenURL) ID() string                             { return o.DocID }
func (o *OpenURL) Rev() string                            { return "" }
func (o *OpenURL) DocType() string                        { return consts.AIOpenURL }
func (o *OpenURL) Clone() couchdb.Doc                     { cloned := *o; return &cloned }
func (o *OpenURL) SetID(id string)                        { o.DocID = id }
func (o *OpenURL) SetRev(rev string)                      {}
func (o *OpenURL) Relationships() jsonapi.RelationshipMap { return nil }
func (o *OpenURL) Included() []jsonapi.Object             { return nil }
func (o *OpenURL) Links() *jsonapi.LinksList              { return nil }
func (o *OpenURL) Fetch(field string) []string            { return nil }

var openwebuiClient = &http.Client{
	Timeout: 60 * time.Second,
}

func Open(inst *instance.Instance) (*OpenURL, error) {
	cfg := getConfig(inst.ContextName)
	if cfg == nil || cfg.URL == "" {
		return nil, errors.New("Not configured")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	admin, err := signin(u, cfg.Admin)
	if err != nil {
		return nil, err
	}
	open, err := fetchOpen(inst, admin.Token, u)
	if err != nil {
		return nil, err
	}
	open.URL = cfg.URL
	return open, nil
}

func getConfig(contextName string) *config.OpenWebUI {
	configuration := config.GetConfig().OpenWebUI
	if c, ok := configuration[contextName]; ok {
		return &c
	} else if c, ok := configuration[config.DefaultInstanceContext]; ok {
		return &c
	}
	return nil
}

func signin(u *url.URL, account map[string]interface{}) (*OpenURL, error) {
	u.Path = "/api/v1/auths/signin"
	payload, err := json.Marshal(account)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationJSON)
	res, err := openwebuiClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("cannot authenticate")
	}
	openURL := &OpenURL{}
	if err := json.NewDecoder(res.Body).Decode(openURL); err != nil {
		return nil, err
	}
	return openURL, nil
}

func fetchOpen(inst *instance.Instance, adminToken string, u *url.URL) (*OpenURL, error) {
	email := "me@" + inst.Domain
	password, err := computePassword(inst)
	if err != nil {
		return nil, err
	}
	signinPayload := map[string]interface{}{
		"email":    email,
		"password": password,
	}
	open, err := signin(u, signinPayload)
	if err == nil {
		return open, nil
	}
	name, err := inst.SettingsPublicName()
	if err != nil || name == "" {
		name = "Cozy"
	}
	image_url := inst.PageURL("/public/avatar", url.Values{
		"fallback": {"initials"},
	})
	account := map[string]interface{}{
		"email":             email,
		"name":              name,
		"password":          password,
		"role":              "user",
		"profile_image_url": image_url,
	}
	return createAccount(adminToken, u, account)
}

func createAccount(adminToken string, u *url.URL, account map[string]interface{}) (*OpenURL, error) {
	u.Path = "/api/v1/auths/add"
	payload, err := json.Marshal(account)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+adminToken)
	res, err := openwebuiClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("cannot create account")
	}
	openURL := &OpenURL{}
	if err := json.NewDecoder(res.Body).Decode(openURL); err != nil {
		return nil, err
	}
	return openURL, nil
}

func computePassword(inst *instance.Instance) (string, error) {
	salt := []byte("OpenWebUI")
	h := hkdf.New(sha256.New, inst.SessionSecret(), salt, nil)
	raw := make([]byte, 32)
	if _, err := io.ReadFull(h, raw); err != nil {
		return "", err
	}
	password := base64.StdEncoding.EncodeToString(raw)[0:32]
	return password, nil
}
