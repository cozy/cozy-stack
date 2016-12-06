package data

import (
	"net/http"

	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/jsonapi"
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

	return jsonapi.NewError(http.StatusForbidden,
		"reserved doctype %v unreadable", doctype)
}

// CheckWritable will abort the echo context if the doctype
// is unwritable
func CheckWritable(c echo.Context, doctype string) error {
	_, inblacklist := blackList[doctype]
	if !inblacklist {
		return nil
	}

	return jsonapi.NewError(http.StatusForbidden,
		"reserved doctype %v unwritable", doctype)
}
