package files

import (
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckAntivirusAction(t *testing.T) {
	config.UseTestFile(t)

	t.Run("NilFile", func(t *testing.T) {
		inst := &instance.Instance{}
		err := CheckAntivirusAction(inst, nil, ActionDownload)
		assert.NoError(t, err)
	})

	t.Run("AntivirusDisabled", func(t *testing.T) {
		inst := &instance.Instance{ContextName: "disabled-context"}
		file := &vfs.FileDoc{
			AntivirusScan: &vfs.AntivirusScan{
				Status: vfs.AVStatusInfected,
			},
		}
		// No antivirus config means disabled - should allow all actions
		err := CheckAntivirusAction(inst, file, ActionDownload)
		assert.NoError(t, err)
	})

	t.Run("FileWithNoScanInfo", func(t *testing.T) {
		// Configure antivirus that blocks download for pending but allows for clean
		conf := config.GetConfig()
		if conf.Contexts == nil {
			conf.Contexts = make(map[string]interface{})
		}
		conf.Contexts["no-scan-test"] = map[string]interface{}{
			"antivirus": map[string]interface{}{
				"enabled": true,
				"actions": map[string]interface{}{
					"pending": []interface{}{},                                         // Block all for pending
					"clean":   []interface{}{"download", "share", "preview", "delete"}, // Allow all for clean
				},
			},
		}
		t.Cleanup(func() {
			delete(conf.Contexts, "no-scan-test")
		})

		inst := &instance.Instance{ContextName: "no-scan-test"}
		file := &vfs.FileDoc{} // No AntivirusScan - defaults to "clean"

		// Should be allowed because default is "clean"
		err := CheckAntivirusAction(inst, file, ActionDownload)
		assert.NoError(t, err)
	})

	t.Run("CleanFileAllowsAllActions", func(t *testing.T) {
		conf := config.GetConfig()
		if conf.Contexts == nil {
			conf.Contexts = make(map[string]interface{})
		}
		conf.Contexts["clean-test"] = map[string]interface{}{
			"antivirus": map[string]interface{}{
				"enabled": true,
				"actions": map[string]interface{}{
					"clean": []interface{}{"download", "share", "preview", "delete"},
				},
			},
		}
		t.Cleanup(func() {
			delete(conf.Contexts, "clean-test")
		})

		inst := &instance.Instance{ContextName: "clean-test"}
		file := &vfs.FileDoc{
			AntivirusScan: &vfs.AntivirusScan{
				Status: vfs.AVStatusClean,
			},
		}

		assert.NoError(t, CheckAntivirusAction(inst, file, ActionDownload))
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionShare))
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionPreview))
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionDelete))
	})

	t.Run("InfectedFileBlocksDownload", func(t *testing.T) {
		conf := config.GetConfig()
		if conf.Contexts == nil {
			conf.Contexts = make(map[string]interface{})
		}
		conf.Contexts["infected-test"] = map[string]interface{}{
			"antivirus": map[string]interface{}{
				"enabled": true,
				"actions": map[string]interface{}{
					"infected": []interface{}{"delete"}, // Only delete allowed
				},
			},
		}
		t.Cleanup(func() {
			delete(conf.Contexts, "infected-test")
		})

		inst := &instance.Instance{ContextName: "infected-test"}
		file := &vfs.FileDoc{
			AntivirusScan: &vfs.AntivirusScan{
				Status:    vfs.AVStatusInfected,
				VirusName: "EICAR-Test-File",
			},
		}

		// Download should be blocked
		err := CheckAntivirusAction(inst, file, ActionDownload)
		require.Error(t, err)
		jsonErr, ok := err.(*jsonapi.Error)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnavailableForLegalReasons, jsonErr.Status)
		assert.Equal(t, "antivirus_blocked", jsonErr.Code)
		assert.Contains(t, jsonErr.Detail, "download")
		assert.Contains(t, jsonErr.Detail, "infected")
		assert.Contains(t, jsonErr.Detail, "EICAR-Test-File")

		// Share should be blocked
		err = CheckAntivirusAction(inst, file, ActionShare)
		require.Error(t, err)

		// Preview should be blocked
		err = CheckAntivirusAction(inst, file, ActionPreview)
		require.Error(t, err)

		// Delete should be allowed
		err = CheckAntivirusAction(inst, file, ActionDelete)
		assert.NoError(t, err)
	})

	t.Run("PendingFileConfigurable", func(t *testing.T) {
		conf := config.GetConfig()
		if conf.Contexts == nil {
			conf.Contexts = make(map[string]interface{})
		}
		conf.Contexts["pending-test"] = map[string]interface{}{
			"antivirus": map[string]interface{}{
				"enabled": true,
				"actions": map[string]interface{}{
					"pending": []interface{}{"preview", "delete"}, // Only preview and delete allowed
				},
			},
		}
		t.Cleanup(func() {
			delete(conf.Contexts, "pending-test")
		})

		inst := &instance.Instance{ContextName: "pending-test"}
		file := &vfs.FileDoc{
			AntivirusScan: &vfs.AntivirusScan{
				Status: vfs.AVStatusPending,
			},
		}

		// Download blocked
		err := CheckAntivirusAction(inst, file, ActionDownload)
		require.Error(t, err)

		// Share blocked
		err = CheckAntivirusAction(inst, file, ActionShare)
		require.Error(t, err)

		// Preview allowed
		err = CheckAntivirusAction(inst, file, ActionPreview)
		assert.NoError(t, err)

		// Delete allowed
		err = CheckAntivirusAction(inst, file, ActionDelete)
		assert.NoError(t, err)
	})

	t.Run("UnconfiguredStatusAllowsAll", func(t *testing.T) {
		conf := config.GetConfig()
		if conf.Contexts == nil {
			conf.Contexts = make(map[string]interface{})
		}
		conf.Contexts["unconfigured-test"] = map[string]interface{}{
			"antivirus": map[string]interface{}{
				"enabled": true,
				"actions": map[string]interface{}{
					"clean": []interface{}{"download"}, // Only clean is configured
				},
			},
		}
		t.Cleanup(func() {
			delete(conf.Contexts, "unconfigured-test")
		})

		inst := &instance.Instance{ContextName: "unconfigured-test"}
		file := &vfs.FileDoc{
			AntivirusScan: &vfs.AntivirusScan{
				Status: vfs.AVStatusSkipped, // Not configured in actions
			},
		}

		// Should allow all actions when status not configured
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionDownload))
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionShare))
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionPreview))
		assert.NoError(t, CheckAntivirusAction(inst, file, ActionDelete))
	})
}
