package middlewares

import "github.com/gin-gonic/gin"

// ServeApp creates a gin middleware that serves app files when the request is
// on a sub-domain
func ServeApp(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, ok := c.Get("app_slug")
		if ok {
			handler(c)
		}
	}
}
