package middlewares

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/echo"
)

// NeedInstance is an echo middleware which will display an error
// if there is no instance.
func NeedInstance(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Get("instance") != nil {
			return next(c)
		}
		i, err := instance.Get(c.Request().Host)
		if err != nil {
			var errHTTP *echo.HTTPError
			switch err {
			case instance.ErrNotFound:
				errHTTP = echo.NewHTTPError(http.StatusNotFound, err)
			case instance.ErrIllegalDomain:
				errHTTP = echo.NewHTTPError(http.StatusBadRequest, err)
			default:
				errHTTP = echo.NewHTTPError(http.StatusInternalServerError, err)
			}
			errHTTP.Inner = err
			return errHTTP
		}
		c.Set("instance", i.WithContextualDomain(c.Request().Host))
		return next(c)
	}
}

// CheckInstanceBlocked is a middleware that blocks the routing access (for
// instance if the term- of-services have not been signed and have reach its
// deadline)
func CheckInstanceBlocked(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		pdoc, err := GetPermission(c)
		if err == nil && pdoc.Type == permissions.TypeCLI {
			return next(c)
		}
		if i.CheckInstanceBlocked() {
			if url, _ := i.ManagerURL(instance.ManagerBlockedURL); url != "" {
				return c.Redirect(http.StatusFound, url)
			}

			var returnCode int
			reason := i.BlockingReason

			switch i.BlockingReason {
			case instance.BlockedLoginFailed.Code:
				returnCode = http.StatusUnavailableForLegalReasons
				reason = instance.BlockedLoginFailed.Message
			case instance.BlockedPaymentFailed.Code:
				returnCode = http.StatusPaymentRequired
				reason = instance.BlockedPaymentFailed.Message
			default:
				// Used for retro-compatibility reasons. Will be deprecated.
				returnCode = http.StatusPaymentRequired
				if reason == "" {
					reason = http.StatusText(http.StatusPaymentRequired)
				}
			}

			contentType := AcceptedContentType(c)
			switch contentType {
			case jsonapi.ContentType, echo.MIMEApplicationJSON:
				return c.JSON(returnCode, i.Warnings())
			default:
				return echo.NewHTTPError(returnCode, reason)
			}
		}
		return next(c)
	}
}

// CheckTOSDeadlineExpired checks if there is not signed ToS and the deadline is
// exceeded
func CheckTOSDeadlineExpired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		if len(i.RegisterToken) > 0 {
			return next(c)
		}
		notSigned, deadline := i.CheckTOSNotSignedAndDeadline()
		if notSigned && deadline == instance.TOSBlocked {
			redirect, _ := i.ManagerURL(instance.ManagerTOSURL)
			if redirect != "" {
				return c.Redirect(http.StatusFound, redirect)
			}
			return echo.NewHTTPError(http.StatusPaymentRequired) // Compatibility reasons
		}
		return next(c)
	}
}

// GetInstance will return the instance linked to the given echo
// context or panic if none exists
func GetInstance(c echo.Context) *instance.Instance {
	return c.Get("instance").(*instance.Instance)
}

// GetInstanceSafe will return the instance linked to the given echo
// context
func GetInstanceSafe(c echo.Context) (*instance.Instance, bool) {
	i := c.Get("instance")
	if i == nil {
		return nil, false
	}
	inst, ok := i.(*instance.Instance)
	return inst, ok
}
