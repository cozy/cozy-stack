package instance

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

// BlockingReason structs holds a reason why an instance had been blocked
type BlockingReason struct {
	Code    string
	Message string
}

var (
	// BlockedLoginFailed is used when a security issue has been detected on the instance
	BlockedLoginFailed = BlockingReason{Code: "LOGIN_FAILED", Message: "Instance Blocked Login"}
	// BlockedPaymentFailed is used when a payment is missing for the instance
	BlockedPaymentFailed = BlockingReason{Code: "PAYMENT_FAILED", Message: "Instance Blocked Payment"}
	// BlockedUnknown is used when an instance is blocked but the reason is unknown
	BlockedUnknown = BlockingReason{Code: "UNKNOWN", Message: "Instance Blocked Unknown"}
)

// Warnings returns a list of possible warnings associated with the instance.
func (i *Instance) Warnings() (warnings []*jsonapi.Error) {
	notSigned, deadline := i.CheckTOSNotSignedAndDeadline()
	if notSigned && deadline >= TOSWarning {
		tosLink, _ := i.ManagerURL(ManagerTOSURL)
		warnings = append(warnings, &jsonapi.Error{
			Status: http.StatusPaymentRequired,
			Title:  "TOS Updated",
			Code:   "tos-updated",
			Detail: i.Translate("Terms of services have been updated"),
			Links:  &jsonapi.LinksList{Self: tosLink},
		})
	}
	return
}

// TOSDeadline represent the state for reaching the TOS deadline.
type TOSDeadline int

const (
	// TOSNone when no deadline is reached.
	TOSNone TOSDeadline = iota
	// TOSWarning when the warning deadline is reached, 2 weeks before the actual
	// activation of the CGU.
	TOSWarning
	// TOSBlocked when the deadline is reached and the access should be blocked.
	TOSBlocked
)

// CheckInstanceBlocked returns whether or not the instance is currently in a
// blocked state: meaning it should be accessible.
func (i *Instance) CheckInstanceBlocked() bool {
	return i.Blocked
}

// CheckTOSNotSigned checks whether or not the current Term of Services have
// been signed by the user.
func (i *Instance) CheckTOSNotSigned(args ...string) (notSigned bool) {
	notSigned, _ = i.CheckTOSNotSignedAndDeadline(args...)
	return
}

// CheckTOSNotSignedAndDeadline checks whether or not the current Term of
// Services have been signed by the user and returns the deadline state to
// perform this signature.
func (i *Instance) CheckTOSNotSignedAndDeadline(args ...string) (notSigned bool, deadline TOSDeadline) {
	tosLatest := i.TOSLatest
	if len(args) > 0 {
		tosLatest = args[0]
	}
	latest, latestDate, ok := ParseTOSVersion(tosLatest)
	if !ok || latestDate.IsZero() {
		return
	}
	defer func() {
		if notSigned {
			now := time.Now()
			if now.After(latestDate) {
				deadline = TOSBlocked
			} else if now.After(latestDate.Add(-30 * 24 * time.Hour)) {
				deadline = TOSWarning
			} else {
				deadline = TOSNone
			}
		}
	}()
	signed, _, ok := ParseTOSVersion(i.TOSSigned)
	if !ok {
		notSigned = true
	} else {
		notSigned = signed < latest
	}
	return
}

// ParseTOSVersion returns the "major" and the date encoded in a version string
// with the following format:
//    parseTOSVersion(A.B.C-YYYYMMDD) -> (A, YYYY, true)
func ParseTOSVersion(v string) (major int, date time.Time, ok bool) {
	if v == "" {
		return
	}
	if len(v) == 8 {
		var err error
		major = 1
		date, err = time.Parse("20060102", v)
		ok = err == nil
		return
	}
	if v[0] == 'v' {
		v = v[1:]
	}
	a := strings.SplitN(v, ".", 3)
	if len(a) != 3 {
		return
	}
	major, err := strconv.Atoi(a[0])
	if err != nil {
		return
	}
	suffix := strings.SplitN(a[2], "-", 2)
	if len(suffix) < 2 {
		ok = true
		return
	}
	date, err = time.Parse("20060102", suffix[1])
	ok = err == nil
	return
}
