package move

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

// ImportOptions contains the options for launching the import worker.
type ImportOptions struct {
	SettingsURL string `json:"url,omitempty"`
	ManifestURL string `json:"manifest_url,omitempty"`
}

// CheckImport returns an error if an exports cannot be found at the given URL,
// or if the instance has not enough disk space to import the files.
func CheckImport(inst *instance.Instance, settingsURL string) error {
	manifestURL, err := transformSettingsURLToManifestURL(settingsURL)
	if err != nil {
		inst.Logger().WithField("nspace", "move").
			Debugf("Invalid settings URL %s: %s", settingsURL, err)
		return ErrExportNotFound
	}
	manifest, err := fetchManifest(manifestURL)
	if err != nil {
		inst.Logger().WithField("nspace", "move").
			Warnf("Cannot fetch manifest: %s", err)
		return ErrExportNotFound
	}
	if inst.BytesDiskQuota > 0 && manifest.TotalSize > inst.BytesDiskQuota {
		return ErrNotEnoughSpace
	}
	return nil
}

// ScheduleImport blocks the instance and adds a job to import the data from
// the given URL.
func ScheduleImport(inst *instance.Instance, options ImportOptions) error {
	manifestURL, err := transformSettingsURLToManifestURL(options.SettingsURL)
	if err != nil {
		return ErrExportNotFound
	}
	options.ManifestURL = manifestURL
	options.SettingsURL = ""
	msg, err := job.NewMessage(options)
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "import",
		Message:    msg,
	})
	if err != nil {
		return err
	}

	settings, err := inst.SettingsDocument()
	if err != nil {
		return nil
	}
	settings.M["importing"] = true
	_ = couchdb.UpdateDoc(inst, settings)
	return nil
}

func transformSettingsURLToManifestURL(settingsURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(settingsURL))
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(u.Host, consts.SettingsSlug+".") {
		// Nested subdomains
		u.Host = strings.TrimPrefix(u.Host, consts.SettingsSlug+".")
	} else {
		// Flat subdomains
		parts := strings.Split(u.Host, ".")
		parts[0] = strings.TrimSuffix(parts[0], "-"+consts.SettingsSlug)
		u.Host = strings.Join(parts, ".")
	}
	if !strings.HasPrefix(u.Fragment, "/exports/") {
		return "", fmt.Errorf("Fragment is not in the expected format")
	}
	mac := strings.TrimPrefix(u.Fragment, "/exports/")
	u.Fragment = ""
	u.Path = "/move/exports/" + mac
	u.RawPath = ""
	u.RawQuery = ""
	return u.String(), nil
}

func fetchManifest(manifestURL string) (*ExportDoc, error) {
	res, err := http.Get(manifestURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ErrExportNotFound
	}
	doc := &ExportDoc{}
	if _, err = jsonapi.Bind(res.Body, doc); err != nil {
		return nil, err
	}
	if doc.State != ExportStateDone {
		return nil, ErrExportNotFound
	}
	return doc, nil
}

// Import downloads the documents and files from an export and add them to the
// local instance. It returns the list of slugs for apps/konnectors that have
// not been installed.
func Import(inst *instance.Instance, options ImportOptions) ([]string, error) {
	defer func() {
		settings, err := inst.SettingsDocument()
		if err == nil {
			delete(settings.M, "importing")
			_ = couchdb.UpdateDoc(inst, settings)
		}
	}()

	doc, err := fetchManifest(options.ManifestURL)
	if err != nil {
		return nil, err
	}

	if err = lifecycle.Reset(inst); err != nil {
		return nil, err
	}

	im := &importer{
		inst:    inst,
		fs:      inst.VFS(),
		options: options,
		doc:     doc,
	}
	if err = im.importPart(""); err != nil {
		return nil, err
	}
	for _, cursor := range doc.PartsCursors {
		if err = im.importPart(cursor); err != nil {
			return nil, err
		}
	}

	return im.appsNotInstalled, nil
}

// ImportIsFinished returns true unless an import is running
func ImportIsFinished(inst *instance.Instance) bool {
	settings, err := inst.SettingsDocument()
	if err != nil {
		return false
	}
	importing, _ := settings.M["importing"].(bool)
	return !importing
}
