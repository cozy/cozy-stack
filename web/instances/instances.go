package instances

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/worker/updates"
	"github.com/labstack/echo/v4"
)

type apiInstance struct {
	*instance.Instance
}

func (i *apiInstance) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.Instance)
}

// Links is used to generate a JSON-API link for the instance
func (i *apiInstance) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/instances/" + i.Instance.DocID}
}

// Relationships is used to generate the content relationship in JSON-API format
func (i *apiInstance) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (i *apiInstance) Included() []jsonapi.Object {
	return nil
}

func createHandler(c echo.Context) error {
	var err error
	opts := &lifecycle.Options{
		Domain:      c.QueryParam("Domain"),
		Locale:      c.QueryParam("Locale"),
		UUID:        c.QueryParam("UUID"),
		TOSSigned:   c.QueryParam("TOSSigned"),
		TOSLatest:   c.QueryParam("TOSLatest"),
		Timezone:    c.QueryParam("Timezone"),
		ContextName: c.QueryParam("ContextName"),
		Email:       c.QueryParam("Email"),
		PublicName:  c.QueryParam("PublicName"),
		Settings:    c.QueryParam("Settings"),
		AuthMode:    c.QueryParam("AuthMode"),
		Passphrase:  c.QueryParam("Passphrase"),
		Key:         c.QueryParam("Key"),
		Apps:        utils.SplitTrimString(c.QueryParam("Apps"), ","),
	}
	if domainAliases := c.QueryParam("DomainAliases"); domainAliases != "" {
		opts.DomainAliases = strings.Split(domainAliases, ",")
	}
	if autoUpdate := c.QueryParam("AutoUpdate"); autoUpdate != "" {
		b, err := strconv.ParseBool(autoUpdate)
		if err != nil {
			return wrapError(err)
		}
		opts.AutoUpdate = &b
	}
	if layout := c.QueryParam("SwiftLayout"); layout != "" {
		opts.SwiftLayout, err = strconv.Atoi(layout)
		if err != nil {
			return wrapError(err)
		}
	}
	if diskQuota := c.QueryParam("DiskQuota"); diskQuota != "" {
		opts.DiskQuota, err = strconv.ParseInt(diskQuota, 10, 64)
		if err != nil {
			return wrapError(err)
		}
	}
	if iterations := c.QueryParam("KdfIterations"); iterations != "" {
		iter, err := strconv.Atoi(iterations)
		if err != nil {
			return wrapError(err)
		}
		if iter < crypto.MinPBKDF2Iterations && iter != 0 {
			err := errors.New("The KdfIterations number is too low")
			return jsonapi.InvalidParameter("KdfIterations", err)
		}
		if iter > crypto.MaxPBKDF2Iterations {
			err := errors.New("The KdfIterations number is too high")
			return jsonapi.InvalidParameter("KdfIterations", err)
		}
		opts.KdfIterations = iter
	}
	in, err := lifecycle.Create(opts)
	if err != nil {
		return wrapError(err)
	}
	in.OAuthSecret = nil
	in.SessSecret = nil
	in.PassphraseHash = nil
	return jsonapi.Data(c, http.StatusCreated, &apiInstance{in}, nil)
}

func showHandler(c echo.Context) error {
	domain := c.Param("domain")
	i, err := lifecycle.GetInstance(domain)
	if err != nil {
		return wrapError(err)
	}
	return jsonapi.Data(c, http.StatusOK, &apiInstance{i}, nil)
}

func modifyHandler(c echo.Context) error {
	domain := c.Param("domain")
	opts := &lifecycle.Options{
		Domain:      domain,
		Locale:      c.QueryParam("Locale"),
		UUID:        c.QueryParam("UUID"),
		TOSSigned:   c.QueryParam("TOSSigned"),
		TOSLatest:   c.QueryParam("TOSLatest"),
		Timezone:    c.QueryParam("Timezone"),
		ContextName: c.QueryParam("ContextName"),
		Email:       c.QueryParam("Email"),
		PublicName:  c.QueryParam("PublicName"),
		Settings:    c.QueryParam("Settings"),
	}
	if domainAliases := c.QueryParam("DomainAliases"); domainAliases != "" {
		opts.DomainAliases = strings.Split(domainAliases, ",")
	}
	if quota := c.QueryParam("DiskQuota"); quota != "" {
		i, err := strconv.ParseInt(quota, 10, 64)
		if err != nil {
			return wrapError(err)
		}
		opts.DiskQuota = i
	}
	if onboardingFinished, err := strconv.ParseBool(c.QueryParam("OnboardingFinished")); err == nil {
		opts.OnboardingFinished = &onboardingFinished
	}
	// Deprecated: the Debug parameter should no longer be used, but is kept
	// for compatibility.
	if debug, err := strconv.ParseBool(c.QueryParam("Debug")); err == nil {
		opts.Debug = &debug
	}
	if blocked, err := strconv.ParseBool(c.QueryParam("Blocked")); err == nil {
		opts.Blocked = &blocked
	}
	if deleting, err := strconv.ParseBool(c.QueryParam("Deleting")); err == nil {
		opts.Deleting = &deleting
	}
	i, err := lifecycle.GetInstance(domain)
	if err != nil {
		return wrapError(err)
	}
	if err = lifecycle.Patch(i, opts); err != nil {
		return wrapError(err)
	}
	return jsonapi.Data(c, http.StatusOK, &apiInstance{i}, nil)
}

func listHandler(c echo.Context) error {
	is, err := instance.List()
	if err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return jsonapi.DataList(c, http.StatusOK, nil, nil)
		}
		return wrapError(err)
	}

	objs := make([]jsonapi.Object, len(is))
	for i, in := range is {
		in.OAuthSecret = nil
		in.SessSecret = nil
		in.PassphraseHash = nil
		objs[i] = &apiInstance{in}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func deleteHandler(c echo.Context) error {
	domain := c.Param("domain")
	err := lifecycle.Destroy(domain)
	if err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func fsckHandler(c echo.Context) (err error) {
	domain := c.Param("domain")
	i, err := lifecycle.GetInstance(domain)
	if err != nil {
		return wrapError(err)
	}

	indexIntegrityCheck, _ := strconv.ParseBool(c.QueryParam("IndexIntegrity"))
	filesConsistencyCheck, _ := strconv.ParseBool(c.QueryParam("FilesConsistency"))
	failFast, _ := strconv.ParseBool(c.QueryParam("FailFast"))

	logCh := make(chan *vfs.FsckLog)
	go func() {
		fs := i.VFS()
		if indexIntegrityCheck {
			err = fs.CheckIndexIntegrity(func(log *vfs.FsckLog) { logCh <- log }, failFast)
		} else if filesConsistencyCheck {
			err = fs.CheckFilesConsistency(func(log *vfs.FsckLog) { logCh <- log }, failFast)
		} else {
			err = fs.Fsck(func(log *vfs.FsckLog) { logCh <- log }, failFast)
		}
		close(logCh)
	}()

	w := c.Response().Writer
	w.WriteHeader(200)
	encoder := json.NewEncoder(w)
	for log := range logCh {
		// XXX do not serialize to JSON the children and the cozyMetadata, as
		// it can take more than 64ko and scanner will ignore such lines.
		if log.FileDoc != nil {
			log.FileDoc.DirsChildren = nil  // It can be filled on type mismatch
			log.FileDoc.FilesChildren = nil // Idem
			log.FileDoc.Metadata = nil
		}
		if log.DirDoc != nil {
			log.DirDoc.DirsChildren = nil
			log.DirDoc.FilesChildren = nil
			log.DirDoc.Metadata = nil
		}
		if errenc := encoder.Encode(log); errenc != nil {
			i.Logger().WithField("nspace", "fsck").
				Warnf("Cannot encode to JSON: %s (%v)", err, log)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	if err != nil {
		log := map[string]string{"error": err.Error()}
		if errenc := encoder.Encode(log); errenc != nil {
			i.Logger().WithField("nspace", "fsck").
				Warnf("Cannot encode to JSON: %s (%v)", err, log)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	return nil
}

func updatesHandler(c echo.Context) error {
	slugs := utils.SplitTrimString(c.QueryParam("Slugs"), ",")
	domain := c.QueryParam("Domain")
	domainsWithContext := c.QueryParam("DomainsWithContext")
	forceRegistry, _ := strconv.ParseBool(c.QueryParam("ForceRegistry"))
	onlyRegistry, _ := strconv.ParseBool(c.QueryParam("OnlyRegistry"))
	msg, err := job.NewMessage(&updates.Options{
		Slugs:              slugs,
		Force:              true,
		ForceRegistry:      forceRegistry,
		OnlyRegistry:       onlyRegistry,
		Domain:             domain,
		DomainsWithContext: domainsWithContext,
		AllDomains:         domain == "",
	})
	if err != nil {
		return err
	}
	j, err := job.System().PushJob(prefixer.GlobalPrefixer, &job.JobRequest{
		WorkerType:  "updates",
		Message:     msg,
		ForwardLogs: true,
	})
	if err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, j)
}

func setAuthMode(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	m := echo.Map{}
	if err := c.Bind(&m); err != nil {
		return err
	}

	authModeString, ok := m["auth_mode"]
	if !ok {
		return jsonapi.BadRequest(errors.New("Missing auth_mode key"))
	}

	authMode, err := instance.StringToAuthMode(authModeString.(string))
	if err != nil {
		return jsonapi.BadRequest(err)
	}

	if !inst.HasAuthMode(authMode) {
		inst.AuthMode = authMode
		if err = couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
			return err
		}
	} else {
		alreadyAuthMode := fmt.Sprintf("Instance has already %s auth mode", authModeString)
		return c.JSON(http.StatusOK, alreadyAuthMode)
	}
	// Return success
	return c.JSON(http.StatusNoContent, nil)
}

type diskUsageResult struct {
	Used          int64 `json:"used,string"`
	Quota         int64 `json:"quota,string,omitempty"`
	Count         int   `json:"doc_count,omitempty"`
	Files         int64 `json:"files,string,omitempty"`
	Versions      int64 `json:"versions,string,omitempty"`
	VersionsCount int   `json:"versions_count,string,omitempty"`
}

func diskUsage(c echo.Context) error {
	domain := c.Param("domain")
	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	fs := instance.VFS()

	files, err := fs.FilesUsage()
	if err != nil {
		return err
	}

	versions, err := fs.VersionsUsage()
	if err != nil {
		return err
	}

	result := &diskUsageResult{}
	result.Used = files + versions
	result.Files = files
	result.Versions = versions
	result.Quota = fs.DiskQuota()
	if stats, err := couchdb.DBStatus(instance, consts.Files); err == nil {
		result.Count = stats.DocCount
	}
	if stats, err := couchdb.DBStatus(instance, consts.FilesVersions); err == nil {
		result.VersionsCount = stats.DocCount
	}
	return c.JSON(http.StatusOK, result)
}

func showPrefix(c echo.Context) error {
	domain := c.Param("domain")

	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, instance.DBPrefix())
}

func getSwiftBucketName(c echo.Context) error {
	domain := c.Param("domain")

	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	var containerNames map[string]string
	type swifter interface {
		ContainerNames() map[string]string
	}
	if obj, ok := instance.VFS().(swifter); ok {
		containerNames = obj.ContainerNames()
	}

	return c.JSON(http.StatusOK, containerNames)
}

func appVersion(c echo.Context) error {
	instances, err := instance.List()
	if err != nil {
		return nil
	}
	appSlug := c.Param("slug")
	version := c.Param("version")

	var instancesAppVersion []string
	var doc app.WebappManifest

	for _, instance := range instances {
		err := couchdb.GetDoc(instance, consts.Apps, consts.Apps+"/"+appSlug, &doc)
		if err == nil {
			if doc.Version() == version {
				instancesAppVersion = append(instancesAppVersion, instance.Domain)
			}
		}
	}

	i := struct {
		Instances []string `json:"instances"`
	}{
		instancesAppVersion,
	}

	return c.JSON(http.StatusOK, i)
}

func wrapError(err error) error {
	switch err {
	case instance.ErrNotFound:
		return jsonapi.NotFound(err)
	case instance.ErrExists:
		return jsonapi.Conflict(err)
	case instance.ErrIllegalDomain:
		return jsonapi.InvalidParameter("domain", err)
	case instance.ErrMissingToken:
		return jsonapi.BadRequest(err)
	case instance.ErrInvalidToken:
		return jsonapi.BadRequest(err)
	case instance.ErrMissingPassphrase:
		return jsonapi.BadRequest(err)
	case instance.ErrInvalidPassphrase:
		return jsonapi.BadRequest(err)
	case instance.ErrBadTOSVersion:
		return jsonapi.BadRequest(err)
	}
	return err
}

// Routes sets the routing for the instances service
func Routes(router *echo.Group) {
	// CRUD for instances
	router.GET("", listHandler)
	router.POST("", createHandler)
	router.GET("/:domain", showHandler)
	router.PATCH("/:domain", modifyHandler)
	router.DELETE("/:domain", deleteHandler)

	// Debug mode
	router.GET("/:domain/debug", getDebug)
	router.POST("/:domain/debug", enableDebug)
	router.DELETE("/:domain/debug", disableDebug)

	// Feature flags
	router.GET("/:domain/feature/flags", getFeatureFlags)
	router.PATCH("/:domain/feature/flags", patchFeatureFlags)
	router.GET("/:domain/feature/sets", getFeatureSets)
	router.PUT("/:domain/feature/sets", putFeatureSets)
	router.GET("/feature/config/:context", getFeatureConfig)
	router.GET("/feature/contexts/:context", getFeatureContext)
	router.PATCH("/feature/contexts/:context", patchFeatureContext)
	router.GET("/feature/defaults", getFeatureDefaults)
	router.PATCH("/feature/defaults", patchFeatureDefaults)

	// Advanced features for instances
	router.GET("/:domain/fsck", fsckHandler)
	router.POST("/updates", updatesHandler)
	router.POST("/token", createToken)
	router.GET("/oauth_client", findClientBySoftwareID)
	router.POST("/oauth_client", registerClient)
	router.POST("/:domain/export", exporter)
	router.POST("/:domain/import", importer)
	router.GET("/:domain/disk-usage", diskUsage)
	router.GET("/:domain/prefix", showPrefix)
	router.GET("/:domain/swift-prefix", getSwiftBucketName)
	router.POST("/:domain/auth-mode", setAuthMode)

	// Config
	router.POST("/redis", rebuildRedis)
	router.GET("/assets", assetsInfos)
	router.POST("/assets", addAssets)
	router.DELETE("/assets/:context/*", deleteAssets)
	router.GET("/contexts", lsContexts)
	router.GET("/with-app-version/:slug/:version", appVersion)

	// Fixers
	router.POST("/:domain/fixers/content-mismatch", contentMismatchFixer)
	router.POST("/:domain/fixers/orphan-account", orphanAccountFixer)
	router.POST("/:domain/fixers/indexes", indexesFixer)
}
