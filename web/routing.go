//go:generate statik -f -src=../assets -dest=. -externals=../assets/.externals

package web

import (
	//"strconv"
	"time"

	//"github.com/cozy/cozy-stack/model/instance/lifecycle"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	//"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/metrics"
	//"github.com/cozy/cozy-stack/web/accounts"
	"github.com/cozy/cozy-stack/web/apps"
	//"github.com/cozy/cozy-stack/web/auth"
	//"github.com/cozy/cozy-stack/web/compat"
	//"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/dispers"
	"github.com/cozy/cozy-stack/web/errors"
	//"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/instances"
	//"github.com/cozy/cozy-stack/web/intents"
	//"github.com/cozy/cozy-stack/web/jobs"
	"github.com/cozy/cozy-stack/web/middlewares"
	//"github.com/cozy/cozy-stack/web/move"
	//"github.com/cozy/cozy-stack/web/notifications"
	//"github.com/cozy/cozy-stack/web/oidc"
	//"github.com/cozy/cozy-stack/web/permissions"
	//"github.com/cozy/cozy-stack/web/public"
	"github.com/cozy/cozy-stack/web/realtime"
	//"github.com/cozy/cozy-stack/web/registry"
	//"github.com/cozy/cozy-stack/web/remote"
	//"github.com/cozy/cozy-stack/web/settings"
	//"github.com/cozy/cozy-stack/web/sharings"
	"github.com/cozy/cozy-stack/web/statik"
	//"github.com/cozy/cozy-stack/web/status"
	"github.com/cozy/cozy-stack/web/version"
	"github.com/cozy/echo"
	"github.com/cozy/echo/middleware"
	//"github.com/prometheus/client_golang/prometheus"
)

const (
	// cspScriptSrcWhitelist is a whitelist for default allowed domains in CSP.
	cspScriptSrcWhitelist = "https://piwik.cozycloud.cc https://matomo.cozycloud.cc"

	// cspImgSrcWhitelist is a whitelist of images domains that are allowed in
	// CSP.
	cspImgSrcWhitelist = "https://piwik.cozycloud.cc https://matomo.cozycloud.cc " +
		"https://*.tile.openstreetmap.org https://*.tile.osm.org " +
		"https://*.tiles.mapbox.com https://api.mapbox.com"

	// cspFrameSrcWhiteList is a whitelist of custom protocols that are allowed
	// in the CSP. We are using iframes on these custom protocols to open
	// deeplinks to them and have a fallback if the mobile apps are not
	// available.
	cspFrameSrcWhiteList = "cozydrive: cozybanks:"
)

var hstsMaxAge = 365 * 24 * time.Hour // 1 year

/*
// SetupAppsHandler adds all the necessary middlewares for the application
// handler.
func SetupAppsHandler(appsHandler echo.HandlerFunc) echo.HandlerFunc {
	mws := []echo.MiddlewareFunc{
		middlewares.LoadAppSession,
		middlewares.CheckIE,
		middlewares.Accept(middlewares.AcceptOptions{
			DefaultContentTypeOffer: echo.MIMETextHTML,
		}),
		middlewares.CheckInstanceBlocked,
		middlewares.CheckTOSDeadlineExpired,
	}
	if !config.GetConfig().CSPDisabled {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			HSTSMaxAge:        hstsMaxAge,
			CSPDefaultSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcParent, middlewares.CSPSrcWS},
			CSPStyleSrc:       []middlewares.CSPSource{middlewares.CSPUnsafeInline},
			CSPFontSrc:        []middlewares.CSPSource{middlewares.CSPSrcData},
			CSPImgSrc:         []middlewares.CSPSource{middlewares.CSPSrcData, middlewares.CSPSrcBlob},
			CSPFrameSrc:       []middlewares.CSPSource{middlewares.CSPSrcSiblings},
			CSPFrameAncestors: []middlewares.CSPSource{middlewares.CSPSrcSelf},

			CSPDefaultSrcWhitelist: config.GetConfig().CSPWhitelist["default"],
			CSPImgSrcWhitelist:     config.GetConfig().CSPWhitelist["img"] + " " + cspImgSrcWhitelist,
			CSPScriptSrcWhitelist:  config.GetConfig().CSPWhitelist["script"] + " " + cspScriptSrcWhitelist,
			CSPConnectSrcWhitelist: config.GetConfig().CSPWhitelist["connect"] + " " + cspScriptSrcWhitelist,
			CSPStyleSrcWhitelist:   config.GetConfig().CSPWhitelist["style"],
			CSPFontSrcWhitelist:    config.GetConfig().CSPWhitelist["font"],
			CSPFrameSrcWhitelist:   config.GetConfig().CSPWhitelist["frame"] + " " + cspFrameSrcWhiteList,
		})
		mws = append([]echo.MiddlewareFunc{secure}, mws...)
	}

	return middlewares.Compose(appsHandler, mws...)
}
*/

// SetupAssets add assets routing and handling to the given router. It also
// adds a Renderer to render templates.
func SetupAssets(router *echo.Echo, assetsPath string) (err error) {
	var r statik.AssetRenderer
	if assetsPath != "" {
		r, err = statik.NewDirRenderer(assetsPath)
	} else {
		r, err = statik.NewRenderer()
	}
	if err != nil {
		return err
	}
	middlewares.BuildTemplates()
	apps.BuildTemplates()

	cacheControl := middlewares.CacheControl(middlewares.CacheOptions{
		MaxAge: 24 * time.Hour,
	})

	router.Renderer = r
	router.HEAD("/assets/*", echo.WrapHandler(r))
	router.GET("/assets/*", echo.WrapHandler(r))
	router.GET("/favicon.ico", echo.WrapHandler(r), cacheControl)
	router.GET("/robots.txt", echo.WrapHandler(r), cacheControl)
	router.GET("/security.txt", echo.WrapHandler(r), cacheControl)
	return nil
}

/*
// SetupRoutes sets the routing for HTTP endpoints
func SetupRoutes(router *echo.Echo) error {
	router.Use(timersMiddleware)

	if !config.GetConfig().CSPDisabled {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			HSTSMaxAge:        hstsMaxAge,
			CSPDefaultSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf},
			CSPImgSrc:         []middlewares.CSPSource{middlewares.CSPSrcData, middlewares.CSPSrcBlob},
			CSPFrameAncestors: []middlewares.CSPSource{middlewares.CSPSrcNone},
		})
		router.Use(secure)
	}

	router.Use(middlewares.CORS(middlewares.CORSOptions{
		BlackList: []string{"/auth/"},
	}))

	// non-authentified HTML routes for authentication (login, OAuth, ...)
	{
		mws := []echo.MiddlewareFunc{
			middlewares.NeedInstance,
			middlewares.LoadSession,
			middlewares.Accept(middlewares.AcceptOptions{
				DefaultContentTypeOffer: echo.MIMETextHTML,
			}),
			middlewares.CheckIE,
			middlewares.CheckInstanceBlocked,
		}
		router.GET("/", auth.Home, mws...)
		auth.Routes(router.Group("/auth", mws...))
	}

	// authentified JSON API routes
	{
		mwsNotBlocked := []echo.MiddlewareFunc{
			middlewares.NeedInstance,
			middlewares.LoadSession,
			middlewares.Accept(middlewares.AcceptOptions{
				DefaultContentTypeOffer: jsonapi.ContentType,
			}),
		}
		mws := append(mwsNotBlocked, middlewares.CheckInstanceBlocked, middlewares.CheckTOSDeadlineExpired)
		registry.Routes(router.Group("/registry", mws...))
		data.Routes(router.Group("/data", mws...))
		files.Routes(router.Group("/files", mws...))
		intents.Routes(router.Group("/intents", mws...))
		jobs.Routes(router.Group("/jobs", mws...))
		notifications.Routes(router.Group("/notifications", mws...))
		move.Routes(router.Group("/move", mws...))
		permissions.Routes(router.Group("/permissions", mws...))
		realtime.Routes(router.Group("/realtime", mws...))
		remote.Routes(router.Group("/remote", mws...))
		sharings.Routes(router.Group("/sharings", mws...))

		// The settings routes needs not to be blocked
		apps.WebappsRoutes(router.Group("/apps", mwsNotBlocked...))
		apps.KonnectorRoutes(router.Group("/konnectors", mwsNotBlocked...))
		settings.Routes(router.Group("/settings", mwsNotBlocked...))
		compat.Routes(router.Group("/compat", mwsNotBlocked...))

		// Careful, the normal middlewares NeedInstance and LoadSession are not
		// applied to these groups since they should not be used for oauth
		// redirection.
		accounts.Routes(router.Group("/accounts"))
		oidc.Routes(router.Group("/oidc"))
	}

	// other non-authentified routes
	{
		public.Routes(router.Group("/public"))
		status.Routes(router.Group("/status"))
		version.Routes(router.Group("/version"))
	}

	// dev routes
	if build.IsDevRelease() {
		router.GET("/dev/mails/:name", devMailsHandler, middlewares.NeedInstance)
		router.GET("/dev/templates/:name", devTemplatesHandler)
	}

	setupRecover(router)
	router.HTTPErrorHandler = errors.ErrorHandler
	return nil
}
*/

/*
func timersMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			status := strconv.Itoa(c.Response().Status)
			metrics.HTTPTotalDurations.
				WithLabelValues(c.Request().Method, status).
				Observe(v)
		}))
		defer timer.ObserveDuration()
		return next(c)
	}
}
*/

// SetupAdminRoutes sets the routing for the administration HTTP endpoints
func SetupAdminRoutes(router *echo.Echo) error {
	var mws []echo.MiddlewareFunc
	if build.IsDevRelease() {
		mws = append(mws, middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}\tstatus=${status}\tmethod=${method}\thost=${host}\turi=${uri}\tbytes_out=${bytes_out}\n",
		}))
	} else {
		mws = append(mws, middlewares.BasicAuth(config.GetConfig().AdminSecretFileName))
	}

	instances.Routes(router.Group("/instances", mws...))
	version.Routes(router.Group("/version", mws...))
	metrics.Routes(router.Group("/metrics", mws...))
	realtime.Routes(router.Group("/realtime", mws...))

	setupRecover(router)

	router.HTTPErrorHandler = errors.ErrorHandler
	return nil
}

// SetupMajorRoutes (previous CreateSubdomainProxy) returns a new web server that will handle that apps
// proxy routing if the host of the request match an application, and route to
// the given router otherwise.
func SetupMajorRoutes(router *echo.Echo /*, appsHandler echo.HandlerFunc*/) (*echo.Echo, error) {
	if err := SetupAssets(router, config.GetConfig().Assets); err != nil {
		return nil, err
	}

	/*
	if err := SetupRoutes(router); err != nil {
		return nil, err
	}
	*/

	//appsHandler = SetupAppsHandler(appsHandler)

	main := echo.New()
	main.HideBanner = true
	main.HidePort = true
	main.Renderer = router.Renderer

	/*
	main.Any("/*", func(c echo.Context) error {
		if parent, slug, _ := middlewares.SplitHost(c.Request().Host); slug != "" {
			if i, err := lifecycle.GetInstance(parent); err == nil {
				c.Set("instance", i.WithContextualDomain(parent))
				c.Set("slug", slug)
				return appsHandler(c)
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	*/

	dispers.Routes(main.Group("/dispers"))
	setupRecover(router)

	main.HTTPErrorHandler = errors.HTMLErrorHandler
	return main, nil
}

// setupRecover sets a recovering strategy of panics happening in handlers
func setupRecover(router *echo.Echo) {
	if !build.IsDevRelease() {
		recoverMiddleware := middlewares.RecoverWithConfig(middlewares.RecoverConfig{
			StackSize: 10 << 10, // 10KB
		})
		router.Use(recoverMiddleware)
	}
}
