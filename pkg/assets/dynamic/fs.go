package dynamic

import (
	"context"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

var assetFS assetsFS

// DynamicAssetsContainerName is the Swift container name for dynamic assets
const DynamicAssetsContainerName = "__dyn-assets__"

// DynamicAssetsFolderName is the folder name for dynamic assets
const DynamicAssetsFolderName = "dyn-assets"

type assetsFS interface {
	Add(string, string, *model.Asset) error
	Get(string, string) ([]byte, error)
	Remove(string, string) error
	List() (map[string][]*model.Asset, error)
	CheckStatus(ctx context.Context) (time.Duration, error)
}

// InitDynamicAssetFS initializes the dynamic asset FS.
func InitDynamicAssetFS() error {
	var err error
	scheme := config.FsURL().Scheme

	switch scheme {
	case config.SchemeFile, config.SchemeMem:
		assetFS, err = NewOsFS()
		if err != nil {
			return err
		}
	case config.SchemeSwift, config.SchemeSwiftSecure:
		assetFS, err = newswiftFS()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Invalid scheme %s for dynamic assets FS", scheme)
	}

	return nil
}
