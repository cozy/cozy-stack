package lifecycle

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/ws"
	"github.com/mitchellh/mapstructure"
)

var managerHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Patch updates the given instance with the specified options if necessary. It
// can also update the settings document if provided in the options.
func Patch(i *instance.Instance, opts *Options) error {
	opts.Domain = i.Domain
	settings := buildSettings(opts)

	clouderyChanges := make(map[string]interface{})

	for {
		var err error
		if i == nil {
			i, err = GetInstance(opts.Domain)
			if err != nil {
				return err
			}
		}

		needUpdate := false
		if opts.Locale != "" && opts.Locale != i.Locale {
			i.Locale = opts.Locale
			clouderyChanges["locale"] = i.Locale
			needUpdate = true
		}

		if opts.Blocked != nil && *opts.Blocked != i.Blocked {
			i.Blocked = *opts.Blocked
			needUpdate = true
		}

		if opts.BlockingReason != "" && opts.BlockingReason != i.BlockingReason {
			i.BlockingReason = opts.BlockingReason
			needUpdate = true
		}

		if aliases := opts.DomainAliases; aliases != nil {
			i.DomainAliases, err = checkAliases(i, aliases)
			if err != nil {
				return err
			}
			needUpdate = true
		}

		if opts.UUID != "" && opts.UUID != i.UUID {
			i.UUID = opts.UUID
			needUpdate = true
		}

		if opts.ContextName != "" && opts.ContextName != i.ContextName {
			i.ContextName = opts.ContextName
			needUpdate = true
		}

		if opts.AuthMode != "" {
			var authMode instance.AuthMode
			authMode, err = instance.StringToAuthMode(opts.AuthMode)
			if err != nil {
				return err
			}
			if i.AuthMode != authMode {
				i.AuthMode = authMode
				needUpdate = true
			}
		}

		if opts.SwiftCluster > 0 && opts.SwiftCluster != i.SwiftCluster {
			i.SwiftCluster = opts.SwiftCluster
			needUpdate = true
		}

		if opts.DiskQuota > 0 && opts.DiskQuota != i.BytesDiskQuota {
			i.BytesDiskQuota = opts.DiskQuota
			needUpdate = true
		} else if opts.DiskQuota == -1 {
			i.BytesDiskQuota = 0
			needUpdate = true
		}

		if opts.AutoUpdate != nil && !(*opts.AutoUpdate) != i.NoAutoUpdate {
			i.NoAutoUpdate = !(*opts.AutoUpdate)
			needUpdate = true
		}

		if opts.OnboardingFinished != nil && *opts.OnboardingFinished != i.OnboardingFinished {
			i.OnboardingFinished = *opts.OnboardingFinished
			needUpdate = true
		}

		if opts.TOSLatest != "" {
			if _, date, ok := instance.ParseTOSVersion(opts.TOSLatest); !ok || date.IsZero() {
				return instance.ErrBadTOSVersion
			}
			if i.TOSLatest != opts.TOSLatest {
				if i.CheckTOSNotSigned(opts.TOSLatest) {
					i.TOSLatest = opts.TOSLatest
					needUpdate = true
				}
			}
		}

		if opts.TOSSigned != "" {
			if _, _, ok := instance.ParseTOSVersion(opts.TOSSigned); !ok {
				return instance.ErrBadTOSVersion
			}
			if i.TOSSigned != opts.TOSSigned {
				i.TOSSigned = opts.TOSSigned
				if !i.CheckTOSNotSigned() {
					i.TOSLatest = ""
				}
				needUpdate = true
			}
		}

		if !needUpdate {
			break
		}

		err = update(i)
		if couchdb.IsConflictError(err) {
			i = nil
			continue
		}
		if err != nil {
			return err
		}
		break
	}

	// Update the settings doc
	if ok, err := needsSettingsUpdate(i, settings.M); settings.Rev() != "" && err == nil && ok {
		if err := couchdb.UpdateDoc(i, settings); err != nil {
			return err
		}
		clouderyUpdateKeys := []string{"email", "public_name"}
		for _, key := range clouderyUpdateKeys {
			if v, ok := settings.M[key]; ok {
				clouderyChanges[key] = v
			}
		}
	}

	if debug := opts.Debug; debug != nil {
		var err error
		if *debug {
			err = logger.AddDebugDomain(i.Domain)
		} else {
			err = logger.RemoveDebugDomain(i.Domain)
		}
		if err != nil {
			return err
		}
	}

	managerUpdateSettings(i, clouderyChanges)

	return nil
}

func managerUpdateSettings(inst *instance.Instance, changes map[string]interface{}) {
	if inst.UUID == "" || len(changes) == 0 {
		return
	}

	client := managerClient(inst)
	if client == nil {
		return
	}

	url := fmt.Sprintf("/api/v1/instances/%s", url.PathEscape(inst.UUID))
	err := client.Put(url, changes, nil)
	if err != nil {
		inst.Logger().Errorf("Error during cloudery settings update %s", err)
	}
}

// needsSettingsUpdate compares the old instance io.cozy.settings with the new
// bunch of settings and tells if it needs an update
func needsSettingsUpdate(inst *instance.Instance, newSettings map[string]interface{}) (bool, error) {
	oldSettings, err := inst.SettingsDocument()

	if err != nil {
		return false, err
	}

	if oldSettings.M == nil {
		return true, nil
	}

	for k, newValue := range newSettings {
		if k == "_id" || k == "_rev" {
			continue
		}
		// Check if we have the key in old settings and the value is different,
		// or if we don't have the key at all
		if oldValue, ok := oldSettings.M[k]; !ok || !reflect.DeepEqual(oldValue, newValue) {
			return true, nil
		}
	}

	// Handles if a key was removed in the new settings but exists in the old
	// settings, and therefore needs an update
	for oldKey := range oldSettings.M {
		if _, ok := newSettings[oldKey]; !ok {
			return true, nil
		}
	}

	return false, nil
}

// Block function blocks an instance with an optional reason parameter
func Block(inst *instance.Instance, reason ...string) error {
	var r string
	if len(reason) == 1 {
		r = reason[0]
	} else {
		r = instance.BlockedUnknown.Code
	}
	blocked := true
	return Patch(inst, &Options{
		Blocked:        &blocked,
		BlockingReason: r,
	})
}

// ManagerSignTOS make a request to the manager in order to finalize the TOS
// signing flow.
func ManagerSignTOS(inst *instance.Instance, originalReq *http.Request) error {
	if inst.TOSLatest == "" {
		return nil
	}
	split := strings.SplitN(inst.TOSLatest, "-", 2)
	if len(split) != 2 {
		return nil
	}
	u, err := inst.ManagerURL(instance.ManagerTOSURL)
	if err != nil {
		return Patch(inst, &Options{TOSSigned: inst.TOSLatest})
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
	return managerHTTPClient.Do(req)
}

type managerConfig struct {
	API *struct {
		URL   string
		Token string
	}
}

func getManagerConfig(inst *instance.Instance) *managerConfig {
	contexts := config.GetConfig().Clouderies
	context, ok := inst.GetFromContexts(contexts)
	if !ok {
		return nil
	}

	var config managerConfig
	err := mapstructure.Decode(context, &config)
	if err != nil {
		return nil
	}

	return &config
}

func managerClient(inst *instance.Instance) *ws.OAuthRestJSONClient {
	config := getManagerConfig(inst)
	if config == nil {
		return nil
	}

	api := config.API
	if api == nil {
		return nil
	}

	url := api.URL
	token := api.Token
	if url == "" || token == "" {
		return nil
	}

	client := &ws.OAuthRestJSONClient{}
	client.Init(url, token)
	return client
}
