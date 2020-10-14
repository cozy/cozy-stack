package move

import (
	"github.com/cozy/cozy-stack/model/instance"
)

// ImportOptions contains the options for launching the import worker.
// TODO document it in docs/workers.md
type ImportOptions struct {
	SettingsURL string `json:"url,omitempty"`
	ManifestURL string `json:"manifest_url,omitempty"`
}

// CheckImport returns an error if an exports cannot be found at the given URL,
// or if the instance has not enough disk space to import the files.
func CheckImport(inst *instance.Instance, settingsURL string) error {
	return nil
}

// ScheduleImport blocks the instance and adds a job to import the data from
// the given URL.
func ScheduleImport(inst *instance.Instance, options ImportOptions) error {
	return nil
}

// Import downloads the documents and files from an export and add them to the
// local instance. It returns the list of slugs for apps/konnectors that have
// not been installed.
func Import(inst *instance.Instance, options ImportOptions) ([]string, error) {
	notInstalled := []string{}
	return notInstalled, nil
}
