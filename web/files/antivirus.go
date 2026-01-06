package files

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

const (
	ActionDownload = "download"
	ActionShare    = "share"
	ActionPreview  = "preview"
	ActionDelete   = "delete"
)

// CheckAntivirusAction checks if the requested action is allowed based on the file's
// antivirus scan status and the instance's antivirus configuration.
// Returns nil if allowed, or a jsonapi.Error with status 451 if blocked.
func CheckAntivirusAction(inst *instance.Instance, file *vfs.FileDoc, action string) error {
	if file == nil {
		return nil
	}
	avConfig := config.GetAntivirusConfig(inst.ContextName)
	if avConfig == nil || !avConfig.Enabled {
		return nil
	}
	status := getFileAntivirusStatus(file)
	if isActionAllowed(avConfig, status, action) {
		return nil
	}
	return newAntivirusBlockedError(file, action, status)
}

// getFileAntivirusStatus returns the antivirus status of a file.
// If the file has no AntivirusScan data, it returns "clean" as the default status.
func getFileAntivirusStatus(file *vfs.FileDoc) string {
	if file == nil {
		return vfs.AVStatusClean
	}
	if file.AntivirusScan == nil {
		return vfs.AVStatusClean
	}
	if file.AntivirusScan.Status == "" {
		return vfs.AVStatusClean
	}
	return file.AntivirusScan.Status
}

// isActionAllowed checks if the given action is in the list of allowed actions
// for the given antivirus status.
func isActionAllowed(cfg *config.AntivirusContextConfig, status, action string) bool {
	if cfg == nil || cfg.Actions == nil {
		return true
	}

	allowedActions, ok := cfg.Actions[status]
	if !ok {
		// Status not configured - default to allow for safety
		return true
	}

	for _, allowed := range allowedActions {
		if strings.EqualFold(allowed, action) {
			return true
		}
	}
	return false
}

// newAntivirusBlockedError creates a jsonapi.Error with status 451
// (Unavailable For Legal Reasons) for blocked antivirus actions.
func newAntivirusBlockedError(file *vfs.FileDoc, action, status string) *jsonapi.Error {
	detail := "Action '" + action + "' is blocked for files with antivirus status '" + status + "'"
	if status == vfs.AVStatusInfected && file != nil && file.AntivirusScan != nil && file.AntivirusScan.VirusName != "" {
		detail += " (detected: " + file.AntivirusScan.VirusName + ")"
	}

	return &jsonapi.Error{
		Status: http.StatusUnavailableForLegalReasons, // 451
		Title:  "Unavailable For Legal Reasons",
		Code:   "antivirus_blocked",
		Detail: detail,
	}
}
