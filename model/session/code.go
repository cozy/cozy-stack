package session

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
)

// CodeLen is the number of random bytes used for the session codes
const CodeLen = 16

// CodeTTL is the lifetime of a SessionCode
const CodeTTL = 1 * time.Minute

// A Code is used to transfer a session from the domain with the stack to a
// subdomain of an application, when the flat subdomains structure is used.
// The code is valid:
//   - only once
//   - for a short time span (1 minute)
//   - just for one application.
type Code struct {
	Value     string
	SessionID string
	AppHost   string
	ExpiresAt int64
}

// BuildCode creates a session code for the given session and app
func BuildCode(sessionID, app string) *Code {
	value := crypto.GenerateRandomBytes(CodeLen)
	value = crypto.Base64Encode(value)
	code := &Code{
		Value:     string(value),
		SessionID: sessionID,
		AppHost:   app,
	}
	_ = getStorage().Add(code)
	return code
}

// FindCode tries to find a valid pending code.
// It also clears the pending codes of all the expired codes.
func FindCode(value, app string) *Code {
	return getStorage().FindAndDelete(value, app)
}
