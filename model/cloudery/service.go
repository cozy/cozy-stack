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
	client, err := s.getClient(inst)
	if err != nil {
		return err
	}

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

type BlockingSubscription struct {
	Vendor string `json:"vendor,omitempty"`
}

func (s *ClouderyService) BlockingSubscription(inst *instance.Instance) (*BlockingSubscription, error) {
	client, err := s.getClient(inst)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("/api/v1/instances/%s", url.PathEscape(inst.UUID))
	res, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if hasBlockingSubscription(res) {
		vendor, err := blockingSubscriptionVendor(res)
		if err != nil {
			return nil, err
		}

		return &BlockingSubscription{
			Vendor: vendor,
		}, nil
	}

	return nil, nil
}

func hasBlockingSubscription(clouderyInstance map[string]interface{}) bool {
	return clouderyInstance["has_blocking_subscription"] == true
}

func blockingSubscriptionVendor(clouderyInstance map[string]interface{}) (string, error) {
	if vendor, ok := clouderyInstance["blocking_subscription_vendor"].(string); ok {
		return vendor, nil
	}

	return "", fmt.Errorf("invalid blocking subscription vendor")
}

func (s *ClouderyService) getClient(inst *instance.Instance) (*manager.APIClient, error) {
	cfg, ok := s.contexts[inst.ContextName]
	if !ok {
		cfg, ok = s.contexts[config.DefaultInstanceContext]
	}

	if !ok {
		return nil, fmt.Errorf("%w: tried %q and %q", ErrInvalidContext, inst.ContextName, config.DefaultInstanceContext)
	}

	client := manager.NewAPIClient(cfg.API.URL, cfg.API.Token)

	return client, nil
}
