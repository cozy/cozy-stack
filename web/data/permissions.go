package data

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/labstack/echo"
)

var readable = true
var none = false

var blackList = map[string]bool{
	consts.Sessions:         none,
	consts.Permissions:      none,
	consts.OAuthClients:     none,
	consts.OAuthAccessCodes: none,
	consts.Files:            readable,
	consts.Instances:        readable,
}

// CheckReadable will abort the context and returns false if the doctype
// is unreadable
func CheckReadable(doctype string) error {
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
func CheckWritable(doctype string) error {
	_, inblacklist := blackList[doctype]
	if !inblacklist {
		return nil
	}

	return &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("reserved doctype %s unwritable", doctype),
	}
}
