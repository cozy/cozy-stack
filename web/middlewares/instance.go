package middlewares

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
)

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
type Instance struct {
	Domain     string // The main DNS domain, like example.cozycloud.cc
	StorageURL string // Where the binaries are persisted
	storage    afero.Fs
}

// GetStorageProvider returns the afero storage provider where the binaries for
// the current instance are persisted
func (instance *Instance) GetStorageProvider() (afero.Fs, error) {
	if instance.storage != nil {
		return instance.storage, nil
	}
	u, err := url.Parse(instance.StorageURL)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "file":
		instance.storage = afero.NewBasePathFs(afero.NewOsFs(), u.Path)
	case "mem":
		instance.storage = afero.NewMemMapFs()
	default:
		return nil, fmt.Errorf("Unknown storage provider: %v", u.Scheme)
	}
	return instance.storage, nil
}

// SetInstance creates a gin middleware to put the instance in the gin context
// for next handlers
func SetInstance() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO the instance should be fetched, using the request parameters
		domain := "dev"
		wd, err := os.Getwd()
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		storageURL := "file://localhost" + wd + "/" + domain + "/"
		instance := Instance{
			Domain:     domain,
			StorageURL: storageURL,
		}
		c.Set("instance", instance)
	}
}
