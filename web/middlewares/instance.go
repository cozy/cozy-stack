package middlewares

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/echo"
)

// NeedInstance is an echo middleware which will display an error
// if there is no instance.
func NeedInstance(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Get("instance") != nil {
			return next(c)
		}
		i, err := lifecycle.GetInstance(c.Request().Host)
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
// instance if the term-of-services have not been signed and have reach its
// deadline)
func CheckInstanceBlocked(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		pdoc, err := GetPermission(c)
		if err == nil && pdoc.Type == permission.TypeCLI {
			return next(c)
		}
		if i.CheckInstanceBlocked() {
			returnCode := http.StatusServiceUnavailable
			// Standard checks
			if i.BlockingReason == instance.BlockedLoginFailed.Code {
				return c.Render(returnCode, "instance_blocked.html", echo.Map{
					"Title":       i.TemplateTitle(),
					"Domain":      i.ContextualDomain(),
					"ContextName": i.ContextName,
					"Locale":      i.Locale,
					"ThemeCSS":    ThemeCSS(i),
					"Reason":      i.Translate(instance.BlockedLoginFailed.Message),
				})
			}

			if url, _ := i.ManagerURL(instance.ManagerBlockedURL); url != "" && IsLoggedIn(c) {
				return c.Redirect(http.StatusFound, url)
			}

			// Fallback by trying to determine the blocking reason
			reason := i.BlockingReason

			if reason == "" {
				reason = i.Translate(instance.BlockedUnknown.Message)
			} else if reason == instance.BlockedPaymentFailed.Code {
				returnCode = http.StatusPaymentRequired
				reason = i.Translate(instance.BlockedPaymentFailed.Message)
			}

			contentType := AcceptedContentType(c)
			switch contentType {
			case jsonapi.ContentType, echo.MIMEApplicationJSON:
				return c.JSON(returnCode, i.Warnings())
			default:
				return c.Render(returnCode, "instance_blocked.html", echo.Map{
					"Title":       i.TemplateTitle(),
					"Domain":      i.ContextualDomain(),
					"ContextName": i.ContextName,
					"Locale":      i.Locale,
					"ThemeCSS":    ThemeCSS(i),
					"Reason":      reason,
				})
			}
		}
		return next(c)
	}
}

// CheckOnboardingNotFinished checks if there is the instance needs to complete
// its onboarding
func CheckOnboardingNotFinished(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		if !i.OnboardingFinished {
			return c.Render(http.StatusOK, "need_onboarding.html", echo.Map{
				"Title":       i.TemplateTitle(),
				"ThemeCSS":    ThemeCSS(i),
				"Domain":      i.ContextualDomain(),
				"ContextName": i.ContextName,
				"Locale":      i.Locale,
			})
		}
		return next(c)
	}
}

// CheckTOSDeadlineExpired checks if there is not signed ToS and the deadline is
// exceeded
func CheckTOSDeadlineExpired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		pdoc, err := GetPermission(c)
		if err == nil && pdoc.Type == permission.TypeCLI {
			return next(c)
		}

		redirect, _ := i.ManagerURL(instance.ManagerTOSURL)

		// Skip check if the instance does not have a ManagerURL or a
		// registerToken
		if len(i.RegisterToken) > 0 || redirect == "" {
			return next(c)
		}

		notSigned, deadline := i.CheckTOSNotSignedAndDeadline()
		if notSigned && deadline == instance.TOSBlocked {
			return c.Redirect(http.StatusFound, redirect)
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
