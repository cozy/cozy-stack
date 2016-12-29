package sessions

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
)

// CodeLen is the number of random bytes used for the session codes
const CodeLen = 16

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

// The list of pending codes
var codes []*Code

// BuildCode creates a session code for the given session and app
func BuildCode(sessionID, app string) *Code {
	value := crypto.GenerateRandomBytes(CodeLen)
	value = crypto.Base64Encode(value)
	expires := time.Now().UTC().Add(1 * time.Minute).Unix()
	code := &Code{
		Value:     string(value),
		SessionID: sessionID,
		AppHost:   app,
		ExpiresAt: expires,
	}
	codes = append(codes, code)
	return code
}

// FindCode tries to find a valid pending code.
// It also clears the pending codes of all the expired codes.
func FindCode(value, app string) *Code {
	var found *Code
	if len(codes) == 0 {
		return nil
	}

	// See https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
	validCodes := codes[:0]
	now := time.Now().UTC().Unix()
	for _, c := range codes {
		if now < c.ExpiresAt {
			validCodes = append(validCodes, c)
			if c.Value == value && c.AppHost == app {
				found = c
			}
		}
	}
	codes = validCodes
	return found
}
