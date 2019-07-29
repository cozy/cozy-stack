// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strconv"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	apps "github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// JSMimeType is the content-type for javascript
const JSMimeType = "application/javascript"

const typeTextEventStream = "text/event-stream"

type apiApp struct {
	apps.Manifest
}

func (man *apiApp) MarshalJSON() ([]byte, error) {
	return json.Marshal(man.Manifest)
}

// Links is part of the Manifest interface
func (man *apiApp) Links() *jsonapi.LinksList {
	var route string
	links := jsonapi.LinksList{}
	switch app := man.Manifest.(type) {
	case (*apps.WebappManifest):
		route = "/apps/"
		if app.Icon != "" {
			links.Icon = "/apps/" + app.Slug() + "/icon/" + app.Version()
		}
		if (app.State() == apps.Ready || app.State() == apps.Installed) &&
			app.Instance != nil {
			links.Related = app.Instance.SubDomain(app.Slug()).String()
		}
	case (*apps.KonnManifest):
		route = "/konnectors/"
		if app.Icon != "" {
			links.Icon = "/konnectors/" + app.Slug() + "/icon/" + app.Version()
		}
		links.Perms = "/permissions/konnectors/" + app.Slug()
	}
	if route != "" {
		links.Self = route + man.Manifest.Slug()
	}
	return &links
}

// Relationships is part of the Manifest interface
func (man *apiApp) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the Manifest interface
func (man *apiApp) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

// apiApp is a jsonapi.Object
var _ jsonapi.Object = (*apiApp)(nil)

func getHandler(appType consts.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		man, err := apps.GetBySlug(instance, slug, appType)
		if err != nil {
			return wrapAppsError(err)
		}
		if err := middlewares.Allow(c, permission.GET, man); err != nil {
			return err
		}
		if webapp, ok := man.(*apps.WebappManifest); ok {
			webapp.Instance = instance
		}
		return jsonapi.Data(c, http.StatusOK, &apiApp{man}, nil)
	}
}

// installHandler handles all POST /:slug request and tries to install
// or update the application with the given Source.
func installHandler(installerType consts.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		source := c.QueryParam("Source")
		if source == "" {
			source = "registry://" + slug
		}
		if err := middlewares.AllowInstallApp(c, installerType, source, permission.POST); err != nil {
			return err
		}

		var overridenParameters *json.RawMessage
		if p := c.QueryParam("Parameters"); p != "" {
			var v json.RawMessage
			if err := json.Unmarshal([]byte(p), &v); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest)
			}
			overridenParameters = &v
		}

		var w http.ResponseWriter
		isEventStream := c.Request().Header.Get("Accept") == typeTextEventStream
		if isEventStream {
			w = c.Response().Writer
			w.Header().Set("Content-Type", typeTextEventStream)
			w.WriteHeader(200)
		}

		inst, err := apps.NewInstaller(instance, instance.AppsCopier(installerType),
			&apps.InstallerOptions{
				Operation:   apps.Install,
				Type:        installerType,
				SourceURL:   source,
				Slug:        slug,
				Deactivated: c.QueryParam("Deactivated") == "true",
				Registries:  instance.Registries(),

				OverridenParameters: overridenParameters,
			},
		)
		if err != nil {
			if isEventStream {
				var b []byte
				if b, err = json.Marshal(err.Error()); err == nil {
					writeStream(w, "error", string(b))
				}
			}
			return wrapAppsError(err)
		}

		go inst.Run()
		return pollInstaller(c, instance, isEventStream, w, slug, inst)
	}
}

// updateHandler handles all POST /:slug request and tries to install
// or update the application with the given Source.
func updateHandler(installerType consts.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		source := c.QueryParam("Source")
		if err := middlewares.AllowInstallApp(c, installerType, source, permission.POST); err != nil {
			return err
		}

		var overridenParameters *json.RawMessage
		if p := c.QueryParam("Parameters"); p != "" {
			var v json.RawMessage
			if err := json.Unmarshal([]byte(p), &v); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest)
			}
			overridenParameters = &v
		}

		var w http.ResponseWriter
		isEventStream := c.Request().Header.Get("Accept") == typeTextEventStream
		if isEventStream {
			w = c.Response().Writer
			w.Header().Set("Content-Type", typeTextEventStream)
			w.WriteHeader(200)
		}

		permissionsAcked, _ := strconv.ParseBool(c.QueryParam("PermissionsAcked"))
		inst, err := apps.NewInstaller(instance, instance.AppsCopier(installerType),
			&apps.InstallerOptions{
				Operation:  apps.Update,
				Type:       installerType,
				SourceURL:  source,
				Slug:       slug,
				Registries: instance.Registries(),

				PermissionsAcked:    permissionsAcked,
				OverridenParameters: overridenParameters,
			},
		)
		if err != nil {
			if isEventStream {
				var b []byte
				if b, err = json.Marshal(err.Error()); err == nil {
					writeStream(w, "error", string(b))
				}
				return nil
			}
			return wrapAppsError(err)
		}

		go inst.Run()
		return pollInstaller(c, instance, isEventStream, w, slug, inst)
	}
}

// deleteHandler handles all DELETE /:slug used to delete an application with
// the specified slug.
func deleteHandler(installerType consts.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		source := "registry://" + slug
		if err := middlewares.AllowInstallApp(c, installerType, source, permission.DELETE); err != nil {
			return err
		}

		// Check if there is a mobile client attached to this app
		if installerType == consts.WebappType {
			oauthClient, err := oauth.FindClientBySoftwareID(instance, "registry://"+slug)
			if err == nil && oauthClient != nil {
				return wrapAppsError(apps.ErrLinkedAppExists)
			}
		}

		// Delete accounts locally and remotely for banking konnectors
		if installerType == consts.KonnectorType {
			toDelete, err := findAccountsToDelete(instance, slug)
			if err != nil {
				return wrapAppsError(err)
			}
			if len(toDelete) > 0 {
				man, err := app.GetKonnectorBySlug(instance, slug)
				if err != nil {
					return wrapAppsError(err)
				}
				deleteKonnectorWithAccounts(instance, slug, toDelete)
				return jsonapi.Data(c, http.StatusAccepted, &apiApp{man}, nil)
			}
		}

		inst, err := apps.NewInstaller(instance, instance.AppsCopier(installerType),
			&apps.InstallerOptions{
				Operation:  apps.Delete,
				Type:       installerType,
				Slug:       slug,
				Registries: instance.Registries(),
			},
		)
		if err != nil {
			return wrapAppsError(err)
		}
		man, err := inst.RunSync()
		if err != nil {
			return wrapAppsError(err)
		}
		return jsonapi.Data(c, http.StatusOK, &apiApp{man}, nil)
	}
}

type deletionEntry struct {
	account *account.Account
	trigger job.Trigger
}

func findAccountsToDelete(instance *instance.Instance, slug string) ([]deletionEntry, error) {
	jobsSystem := job.System()
	triggers, err := jobsSystem.GetAllTriggers(instance)
	if err != nil {
		return nil, err
	}

	var toDelete []deletionEntry
	for _, t := range triggers {
		if t.Infos().WorkerType != "konnector" {
			continue
		}

		var msg struct {
			Account string `json:"account"`
			Slug    string `json:"konnector"`
		}
		err := t.Infos().Message.Unmarshal(&msg)
		if err == nil && msg.Slug == slug && msg.Account != "" {
			acc := &account.Account{}
			if err := couchdb.GetDoc(instance, consts.Accounts, msg.Account, acc); err == nil {
				entry := deletionEntry{acc, t}
				toDelete = append(toDelete, entry)
			}
		}
	}
	return toDelete, nil
}

func deleteKonnectorWithAccounts(instance *instance.Instance, slug string, toDelete []deletionEntry) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch r := r.(type) {
				case error:
					err = r
				default:
					err = fmt.Errorf("%v", r)
				}
				stack := make([]byte, 4<<10) // 4 KB
				length := runtime.Stack(stack, false)
				log := instance.Logger().WithField("panic", true).WithField("nspace", "konnectors")
				log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
			}
		}()

		log := instance.Logger().WithField("nspace", "konnectors")
		jobsSystem := job.System()
		for _, entry := range toDelete {
			acc := entry.account
			acc.ManualCleaning = true
			if err := couchdb.DeleteDoc(instance, acc); err != nil {
				log.Errorf("Cannot delete account: %v", err)
				return
			}
			j, err := account.PushAccountDeletedJob(jobsSystem, instance, acc.ID(), acc.Rev(), slug)
			if err != nil {
				log.Errorf("Cannot push a job for account deletion: %v", err)
				return
			}
			err = j.WaitUntilDone(instance)
			if err != nil {
				log.Error(err)
				return
			}
			err = jobsSystem.DeleteTrigger(instance, entry.trigger.ID())
			if err != nil {
				log.Errorf("Cannot delete the trigger: %v", err)
			}
		}
		inst, err := apps.NewInstaller(instance, instance.AppsCopier(consts.KonnectorType),
			&apps.InstallerOptions{
				Operation:  apps.Delete,
				Type:       consts.KonnectorType,
				Slug:       slug,
				Registries: instance.Registries(),
			},
		)
		if err != nil {
			log.Errorf("Cannot uninstall the konnector: %v", err)
			return
		}
		_, err = inst.RunSync()
		if err != nil {
			log.Errorf("Cannot uninstall the konnector: %v", err)
		}
	}()
}

func pollInstaller(c echo.Context, instance *instance.Instance, isEventStream bool, w http.ResponseWriter, slug string, inst *apps.Installer) error {
	if !isEventStream {
		man, _, err := inst.Poll()
		if err != nil {
			return wrapAppsError(err)
		}
		go func() {
			for {
				_, done, err := inst.Poll()
				if done || err != nil {
					break
				}
			}
		}()
		return jsonapi.Data(c, http.StatusAccepted, &apiApp{man}, nil)
	}

	for {
		man, done, err := inst.Poll()
		if err != nil {
			var b []byte
			if b, err = json.Marshal(err.Error()); err == nil {
				writeStream(w, "error", string(b))
			}
			break
		}
		buf := new(bytes.Buffer)
		if err := jsonapi.WriteData(buf, &apiApp{man}, nil); err == nil {
			writeStream(w, "state", buf.String())
		}
		if done {
			break
		}
	}
	return nil
}

func writeStream(w http.ResponseWriter, event string, b string) {
	s := fmt.Sprintf("event: %s\r\ndata: %s\r\n\r\n", event, b)
	_, err := w.Write([]byte(s))
	if err != nil {
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// listWebappsHandler handles all GET / requests which can be used to list
// installed applications.
func listWebappsHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Apps); err != nil {
		return err
	}

	// Adding the startKey if it is given in the request
	startKey := c.QueryParam("start_key")

	// Same for the limit
	var limit int
	if l := c.QueryParam("limit"); l != "" {
		if converted, err := strconv.Atoi(l); err == nil {
			limit = converted
		}
	}

	docs, next, err := apps.ListWebappsWithPagination(instance, limit, startKey)
	if err != nil {
		return wrapAppsError(err)
	}
	objs := make([]jsonapi.Object, len(docs))
	for i, d := range docs {
		d.Instance = instance
		objs[i] = &apiApp{d}
	}

	// Generating links list for the next apps
	links := generateLinksList(c, next, limit, next)

	return jsonapi.DataList(c, http.StatusOK, objs, links)
}

// listKonnectorsHandler handles all GET / requests which can be used to list
// installed applications.
func listKonnectorsHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Konnectors); err != nil {
		return err
	}

	// Adding the startKey if it is given in the request
	var startKey string
	if sk := c.QueryParam("start_key"); sk != "" {
		startKey = sk
	}

	// Same for the limit
	var limit int
	if l := c.QueryParam("limit"); l != "" {
		if converted, err := strconv.Atoi(l); err == nil {
			limit = converted
		}
	}
	docs, next, err := apps.ListKonnectorsWithPagination(instance, limit, startKey)
	if err != nil {
		return wrapAppsError(err)
	}
	objs := make([]jsonapi.Object, len(docs))
	for i, d := range docs {
		objs[i] = &apiApp{d}
	}

	// Generating links list for the next konnectors
	links := generateLinksList(c, next, limit, next)

	return jsonapi.DataList(c, http.StatusOK, objs, links)
}

func generateLinksList(c echo.Context, next string, limit int, nextID string) *jsonapi.LinksList {
	links := &jsonapi.LinksList{}
	if next != "" { // Do not generate the next URL if there are no next konnectors
		nextURL := &url.URL{
			Scheme: c.Scheme(),
			Host:   c.Request().Host,
			Path:   c.Path(),
		}
		values := nextURL.Query()
		values.Set("start_key", nextID)
		values.Set("limit", strconv.Itoa(limit))
		nextURL.RawQuery = values.Encode()

		links.Next = nextURL.String()
	}
	return links
}

// iconHandler gives the icon of an application
func iconHandler(appType consts.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		version := c.Param("version")
		app, err := apps.GetBySlug(instance, slug, appType)
		if err != nil {
			return err
		}

		if !middlewares.IsLoggedIn(c) && middlewares.Allow(c, permission.GET, app) != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "Not logged in")
		}

		if version != "" {
			// The maximum cache-control recommanded is one year :
			// https://www.ietf.org/rfc/rfc2616.txt
			c.Response().Header().Set("Cache-Control", "max-age=31536000, immutable")
		}

		var fs appfs.FileServer
		var filepath string
		switch appType {
		case consts.WebappType:
			filepath = path.Join("/", app.(*apps.WebappManifest).Icon)
			fs = instance.AppsFileServer()
		case consts.KonnectorType:
			filepath = path.Join("/", app.(*apps.KonnManifest).Icon)
			fs = instance.KonnectorsFileServer()
		}

		err = fs.ServeFileContent(c.Response(), c.Request(),
			app.Slug(), app.Version(), app.Checksum(), filepath)
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, err)
		}
		return err
	}
}

// WebappsRoutes sets the routing for the web apps service
func WebappsRoutes(router *echo.Group) {
	router.GET("/", listWebappsHandler)
	router.GET("/:slug", getHandler(consts.WebappType))
	router.POST("/:slug", installHandler(consts.WebappType))
	router.PUT("/:slug", updateHandler(consts.WebappType))
	router.DELETE("/:slug", deleteHandler(consts.WebappType))
	router.GET("/:slug/icon", iconHandler(consts.WebappType))
	router.GET("/:slug/icon/:version", iconHandler(consts.WebappType))
}

// KonnectorRoutes sets the routing for the konnectors service
func KonnectorRoutes(router *echo.Group) {
	router.GET("/", listKonnectorsHandler)
	router.GET("/:slug", getHandler(consts.KonnectorType))
	router.POST("/:slug", installHandler(consts.KonnectorType))
	router.PUT("/:slug", updateHandler(consts.KonnectorType))
	router.DELETE("/:slug", deleteHandler(consts.KonnectorType))
	router.GET("/:slug/icon", iconHandler(consts.KonnectorType))
	router.GET("/:slug/icon/:version", iconHandler(consts.KonnectorType))
}

func wrapAppsError(err error) error {
	switch err {
	case apps.ErrInvalidSlugName:
		return jsonapi.InvalidParameter("slug", err)
	case apps.ErrAlreadyExists:
		return jsonapi.Conflict(err)
	case apps.ErrNotFound:
		return jsonapi.NotFound(err)
	case apps.ErrNotSupportedSource:
		return jsonapi.InvalidParameter("Source", err)
	case apps.ErrManifestNotReachable:
		return jsonapi.NotFound(err)
	case apps.ErrSourceNotReachable:
		return jsonapi.BadRequest(err)
	case apps.ErrBadManifest:
		return jsonapi.BadRequest(err)
	case apps.ErrMissingSource:
		return jsonapi.BadRequest(err)
	case apps.ErrLinkedAppExists:
		return jsonapi.BadRequest(err)
	}
	if _, ok := err.(*url.Error); ok {
		return jsonapi.InvalidParameter("Source", err)
	}
	return err
}
