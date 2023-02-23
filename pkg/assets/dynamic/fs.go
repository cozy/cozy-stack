package dynamic

import (
	"context"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

var assetFS AssetsFS

// AssetsFS is the interface implemented by all the implementations handling assets.
//
// At the moment there two separate implementations:
// - [SwiftFS] allowing to manage assets via an OpenStack Swift API.
// - [OsFS] allowing to manage assets directly via the host Operating System.
type AssetsFS interface {
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
	case config.SchemeMem:
		assetFS = NewInMemoryFS()

	case config.SchemeFile:
		assetFS, err = NewOsFS()
		if err != nil {
			return err
		}

	case config.SchemeSwift, config.SchemeSwiftSecure:
		assetFS, err = NewSwiftFS()
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("Invalid scheme %s for dynamic assets FS", scheme)
	}

	return nil
}
