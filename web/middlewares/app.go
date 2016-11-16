package middlewares

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServeApp creates a gin middleware that serves app files when the request is
// on a sub-domain
func ServeApp(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, ok := c.Get("app_slug")
		if ok {
			if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
				err := fmt.Errorf("Method %s not allowed", c.Request.Method)
				c.AbortWithError(http.StatusMethodNotAllowed, err)
				return
			}
			handler(c)
		}
	}
}
