package middlewares

import "github.com/gin-gonic/gin"

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
type Instance struct {
	Domain string // The main DNS domain, like example.cozycloud.cc
}

// SetInstance creates a gin middleware to put the instance in the gin context
// for next handlers
func SetInstance() gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := Instance{
			Domain: "dev", // TODO it should be extracted from the request
		}
		c.Set("instance", instance)
	}
}
