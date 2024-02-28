package office

import "github.com/cozy/cozy-stack/pkg/config/config"

func GetConfig(contextName string) *config.Office {
	configuration := config.GetConfig().Office
	if c, ok := configuration[contextName]; ok {
		return &c
	} else if c, ok := configuration[config.DefaultInstanceContext]; ok {
		return &c
	}
	return nil
}
