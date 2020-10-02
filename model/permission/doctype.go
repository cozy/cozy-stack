package permission

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/labstack/echo/v4"
)

var readable = true
var none = false

var blockList = map[string]bool{
	consts.Instances:        none,
	consts.Sessions:         none,
	consts.Permissions:      none,
	consts.Intents:          none,
	consts.OAuthClients:     none,
	consts.OAuthAccessCodes: none,
	consts.Archives:         none,
	consts.Sharings:         none,
	consts.Shared:           none,

	consts.Jobs:           readable,
	consts.Triggers:       readable,
	consts.TriggersState:  readable,
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
	if err := CheckDoctypeName(doctype, false); err != nil {
		return err
	}

	readable, inblocklist := blockList[doctype]
	if !inblocklist || readable {
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
	if err := CheckDoctypeName(doctype, false); err != nil {
		return err
	}

	_, inblocklist := blockList[doctype]
	if !inblocklist {
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
func CheckDoctypeName(doctype string, authorizeWildcard bool) error {
	err := &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("%s is not a valid doctype name", doctype),
	}

	if len(doctype) == 0 {
		return err
	}

	if authorizeWildcard && isWildcard(doctype) {
		// Wildcards on too large domains are not allowed
		if strings.Count(doctype, ".") < 3 {
			return err
		}
		doctype = trimWildcard(doctype)
	}

	for _, c := range doctype {
		if unicode.IsLower(c) || unicode.IsDigit(c) || c == '.' || c == '_' {
			continue
		}
		return err
	}

	// A dot at the beginning or the end of the doctype name is not allowed
	if doctype[0] == '.' || doctype[len(doctype)-1] == '.' {
		return err
	}
	// Two dots side-by-side are not allowed
	if strings.Contains(doctype, "..") {
		return err
	}

	return nil
}

const wildcardSuffix = ".*"

func isWildcard(doctype string) bool {
	return strings.HasSuffix(doctype, wildcardSuffix)
}

func trimWildcard(doctype string) string {
	return strings.TrimSuffix(doctype, wildcardSuffix)
}
