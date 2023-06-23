package cloudery

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/manager"
)

var (
	ErrInvalidContext = errors.New("missing or invalid context")
)

// ClouderyService handle all the Cloudery actions.
type ClouderyService struct {
	contexts map[string]config.ClouderyConfig
}

// NewService instantiate a new [ClouderyService].
//
// If contexts arg is nil, nil will be returned.
func NewService(contexts map[string]config.ClouderyConfig) *ClouderyService {
	if contexts == nil {
		return nil
	}

	return &ClouderyService{contexts}
}

type SaveCmd struct {
	Locale     string
	Email      string
	PublicName string
}

// SaveInstance data into the cloudery matching the instance context.
func (s *ClouderyService) SaveInstance(inst *instance.Instance, cmd *SaveCmd) error {
	cfg, ok := s.contexts[inst.ContextName]
	if !ok {
		cfg, ok = s.contexts[config.DefaultInstanceContext]
	}

	if !ok {
		return fmt.Errorf("%w: tried %q and %q", ErrInvalidContext, inst.ContextName, config.DefaultInstanceContext)
	}

	client := manager.NewAPIClient(cfg.API.URL, cfg.API.Token)

	url := fmt.Sprintf("/api/v1/instances/%s?source=stack", url.PathEscape(inst.UUID))
	if err := client.Put(url, map[string]interface{}{
		"locale":      cmd.Locale,
		"email":       cmd.Email,
		"public_name": cmd.PublicName,
	}); err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	return nil
}
