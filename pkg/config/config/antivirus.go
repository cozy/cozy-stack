package config

import "github.com/go-viper/mapstructure/v2"

// AntivirusContextConfig contains the context-specific antivirus settings
type AntivirusContextConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// OnInfected defines the action when an infected file is detected: "warn" or "block".
	OnInfected    string                       `json:"on_infected,omitempty" mapstructure:"on_infected"`
	Notifications AntivirusNotificationsConfig `json:"notifications,omitempty" mapstructure:"notifications"`
	Actions       map[string][]string          `json:"actions" mapstructure:"actions"`
}

// AntivirusNotificationsConfig contains notification settings for antivirus.
type AntivirusNotificationsConfig struct {
	EmailOnInfected bool `json:"email_on_infected" mapstructure:"email_on_infected"`
}

// GetAntivirusConfig returns the antivirus configuration for a given context.
// It reads from contexts.<contextName>.antivirus and returns the configuration for the UI.
// If global antivirus is enabled but no context-specific config exists, returns enabled with defaults.
func GetAntivirusConfig(contextName string) *AntivirusContextConfig {
	if config == nil || !config.Antivirus.Enabled {
		return &AntivirusContextConfig{Enabled: false}
	}

	// Try to get context-specific config
	var ctxConfig *AntivirusContextConfig
	if config.Contexts != nil {
		ctxConfig = getAntivirusFromContext(contextName)
		if ctxConfig == nil && contextName != DefaultInstanceContext {
			// Fall back to default context
			ctxConfig = getAntivirusFromContext(DefaultInstanceContext)
		}
	}

	// If no context config, use defaults (global is enabled)
	if ctxConfig == nil {
		return &AntivirusContextConfig{
			Enabled: true,
			Actions: getDefaultActions(),
		}
	}

	// If no actions are configured, use defaults
	if ctxConfig.Actions == nil || len(ctxConfig.Actions) == 0 {
		ctxConfig.Actions = getDefaultActions()
	}

	ctxConfig.Enabled = true

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
	if err := mapstructure.Decode(avData, &cfg); err != nil {
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
