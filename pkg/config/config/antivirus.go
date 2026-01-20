package config

import (
	"time"

	"github.com/go-viper/mapstructure/v2"
)

// AntivirusContextConfig contains the antivirus settings for a context.
type AntivirusContextConfig struct {
	// Enabled enables or disables antivirus scanning for this context
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Address is the ClamAV daemon TCP address (e.g., "localhost:3310")
	Address string `json:"address,omitempty" mapstructure:"address"`
	// Timeout is the maximum time to wait for a scan to complete
	Timeout time.Duration `json:"timeout,omitempty" mapstructure:"timeout"`
	// MaxFileSize is the maximum file size to scan (larger files are skipped)
	MaxFileSize int64 `json:"max_file_size,omitempty" mapstructure:"max_file_size"`
	// OnInfected defines the action when an infected file is detected: "warn" or "block".
	OnInfected    string                       `json:"on_infected,omitempty" mapstructure:"on_infected"`
	Notifications AntivirusNotificationsConfig `json:"notifications,omitempty" mapstructure:"notifications"`
	Actions       map[string][]string          `json:"actions,omitempty" mapstructure:"actions"`
}

// AntivirusNotificationsConfig contains notification settings for antivirus.
type AntivirusNotificationsConfig struct {
	EmailOnInfected bool `json:"email_on_infected" mapstructure:"email_on_infected"`
}

// GetAntivirusConfig returns the antivirus configuration for a given context.
func GetAntivirusConfig(contextName string) *AntivirusContextConfig {
	if config == nil || config.Contexts == nil {
		return &AntivirusContextConfig{Enabled: false}
	}

	// Try to get context-specific config
	ctxConfig := getAntivirusFromContext(contextName)
	if ctxConfig == nil && contextName != DefaultInstanceContext {
		// Fall back to default context
		ctxConfig = getAntivirusFromContext(DefaultInstanceContext)
	}

	// If no context config found, antivirus is disabled
	if ctxConfig == nil {
		return &AntivirusContextConfig{Enabled: false}
	}

	// If no actions are configured, use defaults
	if ctxConfig.Actions == nil || len(ctxConfig.Actions) == 0 {
		ctxConfig.Actions = getDefaultActions()
	}

	return ctxConfig
}

func getAntivirusFromContext(contextName string) *AntivirusContextConfig {
	if config == nil || config.Contexts == nil {
		return nil
	}

	ctxData, ok := config.Contexts[contextName].(map[string]interface{})
	if !ok {
		return nil
	}

	avData, ok := ctxData["antivirus"].(map[string]interface{})
	if !ok {
		return nil
	}

	var cfg AntivirusContextConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
		Result:     &cfg,
	})
	if err != nil {
		log.Warnf("Failed to create antivirus config decoder for context %q: %v", contextName, err)
		return nil
	}
	if err := decoder.Decode(avData); err != nil {
		log.Warnf("Failed to decode antivirus config for context %q: %v", contextName, err)
		return nil
	}

	return &cfg
}

// getDefaultActions returns the default allowed actions for each scan status.
// This is a permissive default - administrators can restrict actions via configuration.
func getDefaultActions() map[string][]string {
	return map[string][]string{
		"pending":  {"download", "share", "preview", "delete"},
		"clean":    {"download", "share", "preview", "delete"},
		"infected": {"delete"},
		"error":    {"download", "share", "preview", "delete"},
		"skipped":  {"download", "share", "preview", "delete"},
	}
}
