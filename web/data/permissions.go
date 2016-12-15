package data

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/labstack/echo"
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
func CheckReadable(c echo.Context, doctype string) error {
	readable, inblacklist := blackList[doctype]
	if !inblacklist || readable {
		return nil
	}

	return &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("reserved doctype %s unreadable", doctype),
	}
}

// CheckWritable will abort the echo context if the doctype
// is unwritable
func CheckWritable(c echo.Context, doctype string) error {
	_, inblacklist := blackList[doctype]
	if !inblacklist {
		return nil
	}

	return &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("reserved doctype %s unwritable", doctype),
	}
}
