package permission

import (
	"fmt"
	"net/http"
	"unicode"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/labstack/echo/v4"
)

var readable = true
var none = false

var blackList = map[string]bool{
	consts.Instances:        none,
	consts.Sessions:         none,
	consts.Permissions:      none,
	consts.Intents:          none,
	consts.OAuthClients:     none,
	consts.OAuthAccessCodes: none,
	consts.Archives:         none,
	consts.Sharings:         none,
	consts.Shared:           none,

	// TODO: uncomment to restric jobs permissions (make these none instead of
	// readable).
	consts.Jobs:          readable,
	consts.Triggers:      readable,
	consts.TriggersState: readable,

	consts.Apps:           readable,
	consts.Konnectors:     readable,
	consts.Files:          readable,
	consts.FilesVersions:  readable,
	consts.Notifications:  readable,
	consts.RemoteRequests: readable,
	consts.SessionsLogins: readable,
	consts.NotesSteps:     readable,
}

// CheckReadable will abort the context and returns false if the doctype
// is unreadable
func CheckReadable(doctype string) error {
	if err := CheckDoctypeName(doctype); err != nil {
		return err
	}

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
	if err := CheckDoctypeName(doctype); err != nil {
		return err
	}

	_, inblacklist := blackList[doctype]
	if !inblacklist {
		return nil
	}

	return &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("reserved doctype %s unwritable", doctype),
	}
}

// CheckDoctypeName will return an error if the doctype name is invalid.
// A doctype name must be composed of lowercase letters, digits, . and _
// characters to be valid.
func CheckDoctypeName(doctype string) error {
	err := &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("%s is not a valid doctype name", doctype),
	}

	if len(doctype) == 0 {
		return err
	}
	for _, c := range doctype {
		if unicode.IsLower(c) || unicode.IsDigit(c) || c == '.' || c == '_' {
			continue
		}
		return err
	}
	return nil
}
