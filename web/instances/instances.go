package instances

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
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
	var diskQuota int64
	if c.QueryParam("DiskQuota") != "" {
		var err error
		diskQuota, err = strconv.ParseInt(c.QueryParam("DiskQuota"), 10, 64)
		if err != nil {
			return wrapError(err)
		}
	}
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	for _, setting := range strings.Split(c.QueryParam("Settings"), ",") {
		if parts := strings.SplitN(setting, ":", 2); len(parts) == 2 {
			settings.M[parts[0]] = parts[1]
		}
	}
	if tz := c.QueryParam("Timezone"); tz != "" {
		settings.M["tz"] = tz
	}
	if email := c.QueryParam("Email"); email != "" {
		settings.M["email"] = email
	}
	if name := c.QueryParam("PublicName"); name != "" {
		settings.M["public_name"] = name
	}

	var swiftCluster int
	if cluster := c.QueryParam("SwiftCluster"); cluster != "" {
		var err error
		swiftCluster, err = strconv.Atoi(cluster)
		if err != nil {
			return wrapError(err)
		}
	}

	in, err := instance.Create(&instance.Options{
		Domain:       c.QueryParam("Domain"),
		Locale:       c.QueryParam("Locale"),
		DiskQuota:    diskQuota,
		Settings:     settings,
		SwiftCluster: swiftCluster,
		Apps:         utils.SplitTrimString(c.QueryParam("Apps"), ","),
		Dev:          (c.QueryParam("Dev") == "true"),
	})
	if err != nil {
		return wrapError(err)
	}
	in.OAuthSecret = nil
	in.SessionSecret = nil
	in.PassphraseHash = nil
	pass := c.QueryParam("Passphrase")
	if pass != "" {
		if err = in.RegisterPassphrase([]byte(pass), in.RegisterToken); err != nil {
			return err
		}
		// set the onboarding finished when specifying a passphrase. we totally
		// skip the onboarding in that case.
		in.OnboardingFinished = true
		if err = instance.Update(in); err != nil {
			return err
		}
	}
	return jsonapi.Data(c, http.StatusCreated, &apiInstance{in}, nil)
}

func showHandler(c echo.Context) error {
	domain := c.Param("domain")
	i, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	return jsonapi.Data(c, http.StatusOK, &apiInstance{i}, nil)
}

func modifyHandler(c echo.Context) error {
	domain := c.Param("domain")
	i, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	var shouldUpdate bool
	if quota := c.QueryParam("DiskQuota"); quota != "" {
		var diskQuota int64
		diskQuota, err = strconv.ParseInt(quota, 10, 64)
		if err != nil {
			return wrapError(err)
		}
		i.BytesDiskQuota = diskQuota
		shouldUpdate = true
	}
	if locale := c.QueryParam("Locale"); locale != "" {
		i.Locale = locale
		shouldUpdate = true
	}
	if onboardingFinished := c.QueryParam("OnboardingFinished"); onboardingFinished != "" {
		i.OnboardingFinished, err = strconv.ParseBool(onboardingFinished)
		if err != nil {
			return wrapError(err)
		}
		shouldUpdate = true
	}
	if shouldUpdate {
		if err = instance.Update(i); err != nil {
			return wrapError(err)
		}
	}
	if debug, err := strconv.ParseBool(c.QueryParam("Debug")); err == nil {
		if debug {
			err = logger.AddDebugDomain(domain)
		} else {
			err = logger.RemoveDebugDomain(domain)
		}
		if err != nil {
			return wrapError(err)
		}
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
		in.SessionSecret = nil
		in.PassphraseHash = nil
		objs[i] = &apiInstance{in}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func deleteHandler(c echo.Context) error {
	domain := c.Param("domain")
	err := instance.Destroy(domain)
	if err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func fsckHandler(c echo.Context) error {
	domain := c.Param("domain")
	i, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	prune, _ := strconv.ParseBool(c.QueryParam("Prune"))
	dryRun, _ := strconv.ParseBool(c.QueryParam("DryRun"))
	fs := i.VFS()
	logbook, err := fs.Fsck()
	if err != nil {
		return wrapError(err)
	}
	if prune {
		fs.FsckPrune(logbook, dryRun)
	}
	return c.JSON(http.StatusOK, logbook)
}

func rebuildRedis(c echo.Context) error {
	instances, err := instance.List()
	if err != nil {
		return wrapError(err)
	}
	for _, i := range instances {
		err = globals.GetScheduler().RebuildRedis(i.Domain)
		if err != nil {
			return wrapError(err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

func cleanOrphanAccounts(c echo.Context) error {
	type result struct {
		Result  string                  `json:"result"`
		Error   string                  `json:"error,omitempty"`
		Trigger *scheduler.TriggerInfos `json:"trigger,omitempty"`
	}

	dryRun, _ := strconv.ParseBool(c.QueryParam("DryRun"))
	results := make([]*result, 0)
	domain := c.Param("domain")

	db, err := instance.Get(domain)
	if err != nil {
		return err
	}

	var as []*accounts.Account
	err = couchdb.GetAllDocs(db, consts.Accounts, nil, &as)
	if couchdb.IsNoDatabaseError(err) {
		return c.JSON(http.StatusOK, results)
	}
	if err != nil {
		return err
	}

	sched := globals.GetScheduler()
	ts, err := sched.GetAll(domain)
	if couchdb.IsNoDatabaseError(err) {
		return c.JSON(http.StatusOK, results)
	}
	if err != nil {
		return err
	}

	konnectors, err := apps.ListKonnectors(db)
	if couchdb.IsNoDatabaseError(err) {
		return c.JSON(http.StatusOK, results)
	}
	if err != nil {
		return err
	}

	triggersAccounts := make(map[string]struct{})

	for _, trigger := range ts {
		if trigger.Infos().WorkerType == "konnector" {
			var v struct {
				Account string `json:"account"`
			}
			if err = json.Unmarshal(trigger.Infos().Message, &v); err != nil {
				continue
			}
			if v.Account != "" {
				triggersAccounts[v.Account] = struct{}{}
			}
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, account := range as {
		_, ok := triggersAccounts[account.ID()]
		if ok {
			continue
		}

		var konnectorFound bool
		for _, k := range konnectors {
			if k.Slug() == account.AccountType {
				konnectorFound = true
				break
			}
		}

		if !konnectorFound {
			continue
		}

		msg, _ := json.Marshal(struct {
			Account      string `json:"account"`
			Konnector    string `json:"konnector"`
			FolderToSave string `json:"folder_to_save"`
		}{
			Account:   account.ID(),
			Konnector: account.AccountType,
		})

		var args string
		{
			d := rng.Intn(7)
			h := rng.Intn(6) // during the night, between 0 and 5
			m := rng.Intn(60)
			args = fmt.Sprintf("0 %d %d * * %d", m, h, d)
		}

		var r result
		infos := &scheduler.TriggerInfos{
			WorkerType: "konnector",
			Domain:     domain,
			Type:       "@cron",
			Arguments:  args,
			Message:    jobs.Message(msg),
		}
		r.Trigger = infos
		t, err := scheduler.NewTrigger(infos)
		if err != nil {
			r.Result = "failed"
			r.Error = err.Error()
			continue
		}
		if !dryRun {
			err = sched.Add(t)
		} else {
			err = nil
		}
		if err != nil {
			r.Result = "failed"
			r.Error = err.Error()
		} else {
			r.Result = "created"
		}
		results = append(results, &r)
	}

	return c.JSON(http.StatusOK, results)
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
	}
	return err
}

// Routes sets the routing for the instances service
func Routes(router *echo.Group) {
	router.GET("", listHandler)
	router.POST("", createHandler)
	router.GET("/:domain", showHandler)
	router.PATCH("/:domain", modifyHandler)
	router.DELETE("/:domain", deleteHandler)
	router.GET("/:domain/fsck", fsckHandler)
	router.POST("/updates", updatesHandler)
	router.POST("/token", createToken)
	router.POST("/oauth_client", registerClient)
	router.POST("/:domain/export", exporter)
	router.POST("/:domain/import", importer)
	router.POST("/:domain/orphan_accounts", cleanOrphanAccounts)
	router.POST("/redis", rebuildRedis)
}
