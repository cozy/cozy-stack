package instance

import (
	"strconv"
	"strings"
	"time"
)

// Warning is a struct describing a warning associated with the instance. For
// example, when the TOS have not been signed yet by the user.
type Warning struct {
	Title   string `json:"title"`
	Error   string `json:"error"`
	Details string `json:"details"`
	Link    string `json:"link"`
}

// Warnings returns a list of possible warnings associated with the instance.
func (i *Instance) Warnings() (warnings []*Warning) {
	notSigned, _ := i.CheckTOSSigned()
	if notSigned {
		tosLink, _ := i.ManagerURL(ManagerTOSURL)
		warnings = append(warnings, &Warning{
			Title:   "TOS Updated",
			Error:   "tos-updated",
			Details: "Terms of services have been updated",
			Link:    tosLink,
		})
	}
	return
}

// CheckTOSSigned checks whether or not the current Term of Services have been
// signed by the user.
func (i *Instance) CheckTOSSigned(args ...string) (notSigned, deadlineReached bool) {
	tosLatest := i.TOSLatest
	if len(args) > 0 {
		tosLatest = args[0]
	}
	latest, latestDate, ok := parseTOSVersion(tosLatest)
	if !ok {
		return
	}
	signed, _, ok := parseTOSVersion(i.TOSSigned)
	if !ok {
		notSigned = true
		return
	}
	if signed >= latest {
		return
	}
	notSigned = true
	deadlineReached = time.Since(latestDate) > 15*24*time.Hour
	return
}

// parseTOSVersion returns the "major" and the date encoded in a version string
// with the following format:
//    parseTOSVersion(A.B.C-YYYYMMDD) -> (A, YYYY, true)
func parseTOSVersion(v string) (major int, date time.Time, ok bool) {
	if v == "" {
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
		return
	}
	date, err = time.Parse("20060102", suffix[1])
	ok = err == nil
	return
}
