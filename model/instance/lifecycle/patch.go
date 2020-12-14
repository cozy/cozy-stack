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
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
)

var managerHTTPClient = &http.Client{Timeout: 30 * time.Second}

// AskReupload is the function that will be called when the disk quota is
// increased to ask reuploading files from the sharings. A package variable is
// used to avoid a dependency on the model/sharing package (which would lead to
// circular import issue).
var AskReupload func(*instance.Instance) error

// Patch updates the given instance with the specified options if necessary. It
// can also update the settings document if provided in the options.
func Patch(i *instance.Instance, opts *Options) error {
	opts.Domain = i.Domain
	settings, err := buildSettings(i, opts)
	if err != nil {
		return err
	}

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
		needSharingReupload := false

		if opts.Locale != "" && opts.Locale != i.Locale {
			i.Locale = opts.Locale
			clouderyChanges["locale"] = i.Locale
			needUpdate = true
		}

		if opts.Blocked != nil && *opts.Blocked != i.Blocked {
			i.Blocked = *opts.Blocked
			needUpdate = true
		}

		if opts.Deleting != nil && *opts.Deleting != i.Deleting {
			i.Deleting = *opts.Deleting
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

		if opts.OIDCID != "" && opts.OIDCID != i.OIDCID {
			i.OIDCID = opts.OIDCID
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

		if opts.DiskQuota > 0 && opts.DiskQuota != i.BytesDiskQuota {
			needUpdate = true
			needSharingReupload = opts.DiskQuota > i.BytesDiskQuota
			i.BytesDiskQuota = opts.DiskQuota
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
		if needSharingReupload && AskReupload != nil {
			go func() {
				inst := i.Clone().(*instance.Instance)
				if err := AskReupload(inst); err != nil {
					inst.Logger().WithField("nspace", "lifecycle").
						Warnf("sharing.AskReupload failed with %s", err)
				}
			}()
		}
		break
	}

	// Update the settings doc
	if ok := needsSettingsUpdate(i, settings.M); ok {
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
			err = logger.AddDebugDomain(i.Domain, 24*time.Hour)
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

	client := instance.APIManagerClient(inst)
	if client == nil {
		return
	}

	url := fmt.Sprintf("/api/v1/instances/%s", url.PathEscape(inst.UUID))
	if err := client.Put(url, changes); err != nil {
		inst.Logger().Errorf("Error during cloudery settings update %s", err)
	}
}

// needsSettingsUpdate compares the old instance io.cozy.settings with the new
// bunch of settings and tells if it needs an update
func needsSettingsUpdate(inst *instance.Instance, newSettings map[string]interface{}) bool {
	oldSettings, err := inst.SettingsDocument()
	if err != nil {
		return false
	}

	if oldSettings.M == nil {
		return true
	}

	for k, newValue := range newSettings {
		if k == "_id" || k == "_rev" {
			continue
		}
		// Check if we have the key in old settings and the value is different,
		// or if we don't have the key at all
		if oldValue, ok := oldSettings.M[k]; !ok || !reflect.DeepEqual(oldValue, newValue) {
			return true
		}
	}

	// Handles if a key was removed in the new settings but exists in the old
	// settings, and therefore needs an update
	for oldKey := range oldSettings.M {
		if _, ok := newSettings[oldKey]; !ok {
			return true
		}
	}

	return false
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

// Unblock reverts the blocking of an instance
func Unblock(inst *instance.Instance) error {
	blocked := false
	return Patch(inst, &Options{
		Blocked:        &blocked,
		BlockingReason: "",
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
