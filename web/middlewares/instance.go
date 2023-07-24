package middlewares

import (
	"io"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/labstack/echo/v4"
	"golang.org/x/net/idna"
)

// NeedInstance is an echo middleware which will display an error
// if there is no instance.
func NeedInstance(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Get("instance") != nil {
			return next(c)
		}
		host, err := idna.ToUnicode(c.Request().Host)
		if err != nil {
			return err
		}
		i, err := lifecycle.GetInstance(host)
		if err != nil {
			var errHTTP *echo.HTTPError
			switch err {
			case instance.ErrNotFound, instance.ErrIllegalDomain:
				err = instance.ErrNotFound
				errHTTP = echo.NewHTTPError(http.StatusNotFound, err)
			default:
				errHTTP = echo.NewHTTPError(http.StatusInternalServerError, err)
			}
			errHTTP.Internal = err
			return errHTTP
		}
		c.Set("instance", i.WithContextualDomain(host))
		return next(c)
	}
}

// CheckInstanceDeleting is a middleware that blocks the routing access for
// instances with the deleting flag set.
func CheckInstanceDeleting(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		if i.Deleting {
			err := instance.ErrNotFound
			errHTTP := echo.NewHTTPError(http.StatusNotFound, err)
			errHTTP.Internal = err
			return errHTTP
		}
		return next(c)
	}
}

// CheckInstanceBlocked is a middleware that blocks the routing access (for
// instance if the term-of-services have not been signed and have reach its
// deadline)
func CheckInstanceBlocked(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		if _, ok := GetCLIPermission(c); ok {
			return next(c)
		}
		if i.CheckInstanceBlocked() {
			return handleBlockedInstance(c, i, next)
		}
		return next(c)
	}
}

func handleBlockedInstance(c echo.Context, i *instance.Instance, next echo.HandlerFunc) error {
	returnCode := http.StatusServiceUnavailable
	contentType := AcceptedContentType(c)

	if c.Request().URL.Path == "/robots.txt" {
		if f, ok := assets.Get("/robots.txt", i.ContextName); ok {
			_, err := io.Copy(c.Response(), f.Reader())
			return err
		}
	}

	// Standard checks
	if i.BlockingReason == instance.BlockedLoginFailed.Code {
		return c.Render(returnCode, "instance_blocked.html", echo.Map{
			"Domain":       i.ContextualDomain(),
			"ContextName":  i.ContextName,
			"Locale":       i.Locale,
			"Title":        i.TemplateTitle(),
			"Favicon":      Favicon(i),
			"Reason":       i.Translate(instance.BlockedLoginFailed.Message),
			"SupportEmail": i.SupportEmailAddress(),
		})
	}

	// Allow konnectors to be run for the delete accounts hook just before
	// moving a Cozy.
	if move.GetStore().AllowDeleteAccounts(i) {
		perms, err := GetPermission(c)
		if err == nil && perms.Type == permission.TypeKonnector {
			return next(c)
		}
	}

	if i.BlockingReason == instance.BlockedImporting.Code ||
		i.BlockingReason == instance.BlockedMoving.Code {
		// Allow requests to the importing page
		if strings.HasPrefix(c.Request().URL.Path, "/move/") {
			return next(c)
		}
		switch contentType {
		case jsonapi.ContentType, echo.MIMEApplicationJSON:
			reason := i.Translate(instance.BlockedPaymentFailed.Message)
			return c.JSON(returnCode, echo.Map{"error": reason})
		default:
			return c.Redirect(http.StatusFound, i.PageURL("/move/importing", nil))
		}
	}

	if url, _ := i.ManagerURL(instance.ManagerBlockedURL); url != "" && IsLoggedIn(c) {
		switch contentType {
		case jsonapi.ContentType, echo.MIMEApplicationJSON:
			warnings := warningOrBlocked(i, returnCode)
			return c.JSON(returnCode, warnings)
		default:
			return c.Redirect(http.StatusFound, url)
		}
	}

	// Fallback by trying to determine the blocking reason
	reason := i.BlockingReason
	if reason == instance.BlockedPaymentFailed.Code {
		returnCode = http.StatusPaymentRequired
		reason = i.Translate(instance.BlockedPaymentFailed.Message)
	}

	switch contentType {
	case jsonapi.ContentType, echo.MIMEApplicationJSON:
		warnings := warningOrBlocked(i, returnCode)
		return c.JSON(returnCode, warnings)
	default:
		return c.Render(returnCode, "instance_blocked.html", echo.Map{
			"Domain":       i.ContextualDomain(),
			"ContextName":  i.ContextName,
			"Locale":       i.Locale,
			"Title":        i.TemplateTitle(),
			"Favicon":      Favicon(i),
			"Reason":       reason,
			"SupportEmail": i.SupportEmailAddress(),
		})
	}
}

func warningOrBlocked(i *instance.Instance, returnCode int) []*jsonapi.Error {
	warnings := i.Warnings()
	if len(warnings) == 0 {
		warnings = []*jsonapi.Error{
			{
				Status: returnCode,
				Title:  "Blocked",
				Code:   instance.BlockedUnknown.Code,
				Detail: i.Translate(instance.BlockedUnknown.Message),
			},
		}
	}
	return warnings
}

// CheckOnboardingNotFinished checks if there is the instance needs to complete
// its onboarding
func CheckOnboardingNotFinished(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		i := GetInstance(c)
		if !i.OnboardingFinished {
			return c.Render(http.StatusOK, "need_onboarding.html", echo.Map{
				"Domain":       i.ContextualDomain(),
				"ContextName":  i.ContextName,
				"Locale":       i.Locale,
				"Title":        i.TemplateTitle(),
				"Favicon":      Favicon(i),
				"SupportEmail": i.SupportEmailAddress(),
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
		if _, ok := GetCLIPermission(c); ok {
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
			switch AcceptedContentType(c) {
			case jsonapi.ContentType, echo.MIMEApplicationJSON:
				return c.JSON(http.StatusPaymentRequired, i.Warnings())
			default:
				return c.Redirect(http.StatusFound, redirect)
			}
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
