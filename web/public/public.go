// Package public adds some public routes that can be used to give information
// to anonymous users, or to the not yet authentified cozy owner on its login
// page.
package public

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	csettings "github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/pkg/avatar"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo/v4"
)

func errorNotFound() error {
	return echo.NewHTTPError(http.StatusNotFound, "Page not found")
}

func errorInvalidParam(name string) error {
	return echo.NewHTTPError(http.StatusBadRequest, "Invalid `"+name+"`")
}

// Avatar returns the default avatar currently.
//
//  1. If an avatar has been uploaded through `PUT /settings/avatar`, this
//     image will be returned.
//
//  2. Otherwise it depends on the `fallback` query param:
//
//     2.1. `fallback=404`: just respond a 404 if no avatar file was set
//
//     2.2. `fallback=default` (or empty): get the `default-avatar.png` asset (for retro-compatibility)
//
//     2.3. `fallback=anonymous`: get a generic user avatar without initials visible (respects `format`)
//
//     2.4. `fallback=initials`, initials are calculated:
//
//     2.4.1. Attempt with [../../model/settings/service.go] PublicName(), which gets the
//     instance's `public_name`, or the `DomainName`
//
//  4. Additional query params when `fallback` isn't set or is `anonymous`:
//
//     4.1. `fx=translucent`: if SVG, make the output partially transparent
//     4.2. `as=unconfirmed`: if SVG, make the output grayscale
//     4.3. `format=png`: request a PNG response, otherwise defaults to SVG
func Avatar(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := inst.AvatarFS().ServeAvatarContent(c.Response(), c.Request()); err != os.ErrNotExist {
		return err
	}

	role := c.QueryParam("as")
	grayscale := role == "unconfirmed"
	if role != "" && !grayscale {
		return errorInvalidParam("as")
	}

	fx := c.QueryParam("fx")
	translucent := fx == "translucent"
	if fx != "" && !translucent {
		return errorInvalidParam("fx")
	}

	format := strings.ToLower(c.QueryParam("format"))
	wantPNG := format == "png"
	if format != "" && !wantPNG && format != "svg" {
		return errorInvalidParam("format")
	}

	fallback := c.QueryParam("fallback")
	fallbackIsInitials := fallback == "initials"
	fallbackIs404 := fallback == "404"
	fallbackIsDefault := fallback == "" || fallback == "default"
	fallbackIsAnonymous := fallback == "anonymous"
	if !(fallbackIsInitials || fallbackIs404 || fallbackIsDefault || fallbackIsAnonymous) {
		return errorInvalidParam("fallback")
	}

	switch {
	case fallbackIs404:
		return errorNotFound()

	case fallbackIsDefault:
		f, ok := assets.Get("/images/default-avatar.png", inst.ContextName)
		if ok {
			handler := statik.NewHandler()
			handler.ServeFile(c.Response(), c.Request(), f, true)
			return nil
		}
		return errorNotFound()

	case fallbackIsInitials || fallbackIsAnonymous:
		publicName, err := csettings.PublicName(inst)
		if err != nil {
			publicName = strings.Split(inst.Domain, ".")[0]
		}
		if fallbackIsAnonymous {
			publicName = ""
		}
		options := make([]avatar.Options, 0, 1)
		if wantPNG {
			options = append(options, avatar.FormatPNG)
		}
		if grayscale {
			options = append(options, avatar.GreyBackground)
		}
		if translucent {
			options = append(options, avatar.Translucent)
		}

		img, mime, err := config.Avatars().GenerateInitials(publicName, options...)
		if err == nil {
			return c.Blob(http.StatusOK, mime, img)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Error generating avatar")
	}

	// shouldn't be reachable
	return echo.NewHTTPError(http.StatusInternalServerError, "Well, this is unexpected !")
}

// Prelogin returns information that could be useful to show a login page (like
// in the flagship app).
func Prelogin(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !inst.OnboardingFinished {
		return c.JSON(http.StatusPreconditionFailed, echo.Map{
			"error": "the instance has not been onboarded",
		})
	}

	publicName, err := csettings.PublicName(inst)
	if err != nil {
		publicName = ""
	}
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	_, oidc := config.GetOIDC(inst.ContextName)
	franceConnect := inst.FranceConnectID != ""
	return c.JSON(http.StatusOK, echo.Map{
		"Kdf":           setting.PassphraseKdf,
		"KdfIterations": setting.PassphraseKdfIterations,
		"OIDC":          oidc,
		"FranceConnect": franceConnect,
		"magic_link":    inst.MagicLink,
		"locale":        inst.Locale,
		"name":          publicName,
	})
}

// Routes sets the routing for the public service
func Routes(router *echo.Group) {
	cacheControl := middlewares.CacheControl(middlewares.CacheOptions{
		MaxAge: 24 * time.Hour,
	})
	router.GET("/avatar", Avatar, cacheControl)
	router.GET("/prelogin", Prelogin)
}
