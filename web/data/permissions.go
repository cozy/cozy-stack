package data

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/gin-gonic/gin"
)

var readable = true
var none = false

var blackList = map[string]bool{
	auth.SessionsType:     none,
	vfs.FsDocType:         readable,
	instance.InstanceType: readable,
}

// CheckReadable will abort the context and returns false if the doctype
// is unreadable
func CheckReadable(c *gin.Context, doctype string) bool {
	readable, inblacklist := blackList[doctype]
	if !inblacklist || readable {
		return true
	}

	err := fmt.Errorf("reserved doctype %v unreadable", doctype)
	c.AbortWithError(http.StatusForbidden, err)

	return false
}

// CheckWritable will abort the gin context if the doctype
// is unwritable
func CheckWritable(c *gin.Context, doctype string) bool {
	_, inblacklist := blackList[doctype]
	if !inblacklist {
		return true
	}

	err := fmt.Errorf("reserved doctype %v unwritable", doctype)
	c.AbortWithError(http.StatusForbidden, err)

	return false
}
