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
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/pkg/workers/updates"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/echo"
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
	opts := &instance.Options{
		Domain:     c.QueryParam("Domain"),
		Locale:     c.QueryParam("Locale"),
		UUID:       c.QueryParam("UUID"),
		TOSSigned:  c.QueryParam("TOSSigned"),
		TOSLatest:  c.QueryParam("TOSLatest"),
		Timezone:   c.QueryParam("Timezone"),
		Email:      c.QueryParam("Email"),
		PublicName: c.QueryParam("PublicName"),
		Settings:   c.QueryParam("Settings"),
		AuthMode:   c.QueryParam("AuthMode"),
		Passphrase: c.QueryParam("Passphrase"),
		Apps:       utils.SplitTrimString(c.QueryParam("Apps"), ","),
		Dev:        (c.QueryParam("Dev") == "true"),
	}
	if domainAliases := c.QueryParam("DomainAliases"); domainAliases != "" {
		opts.DomainAliases = strings.Split(domainAliases, ",")
	}
	if autoUpdate := c.QueryParam("AutoUpdate"); autoUpdate != "" {
		var b bool
		b, err = strconv.ParseBool(autoUpdate)
		if err != nil {
			return wrapError(err)
		}
		opts.AutoUpdate = &b
	}
	if cluster := c.QueryParam("SwiftCluster"); cluster != "" {
		opts.SwiftCluster, err = strconv.Atoi(cluster)
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
	in, err := instance.Create(opts)
	if err != nil {
		return wrapError(err)
	}
	in.OAuthSecret = nil
	in.SessionSecret = nil
	in.PassphraseHash = nil
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
	opts := &instance.Options{
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
	if swiftCluster := c.QueryParam("SwiftCluster"); swiftCluster != "" {
		i, err := strconv.ParseInt(swiftCluster, 10, 64)
		if err != nil {
			return wrapError(err)
		}
		opts.SwiftCluster = int(i)
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
	if debug, err := strconv.ParseBool(c.QueryParam("Debug")); err == nil {
		opts.Debug = &debug
	}
	if blocked, err := strconv.ParseBool(c.QueryParam("Blocked")); err == nil {
		opts.Blocked = &blocked
	}
	i, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	if err = instance.Patch(i, opts); err != nil {
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
	logbook, err := fs.Fsck(vfs.FsckOptions{
		Prune:  prune,
		DryRun: dryRun,
	})
	if err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, logbook)
}

func rebuildRedis(c echo.Context) error {
	instances, err := instance.List()
	if err != nil {
		return wrapError(err)
	}
	if err = jobs.System().CleanRedis(); err != nil {
		return wrapError(err)
	}
	for _, i := range instances {
		err = jobs.System().RebuildRedis(i)
		if err != nil {
			return wrapError(err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

func cleanOrphanAccounts(c echo.Context) error {
	type result struct {
		Result  string             `json:"result"`
		Error   string             `json:"error,omitempty"`
		Trigger *jobs.TriggerInfos `json:"trigger,omitempty"`
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

	sched := jobs.System()
	ts, err := sched.GetAllTriggers(db)
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

		msg := struct {
			Account      string `json:"account"`
			Konnector    string `json:"konnector"`
			FolderToSave string `json:"folder_to_save"`
		}{
			Account:   account.ID(),
			Konnector: account.AccountType,
		}

		var args string
		{
			d := rng.Intn(7)
			h := rng.Intn(6) // during the night, between 0 and 5
			m := rng.Intn(60)
			args = fmt.Sprintf("0 %d %d * * %d", m, h, d)
		}

		var r result
		infos := jobs.TriggerInfos{
			WorkerType: "konnector",
			Type:       "@cron",
			Arguments:  args,
		}
		r.Trigger = &infos
		t, err := jobs.NewTrigger(db, infos, msg)
		if err != nil {
			r.Result = "failed"
			r.Error = err.Error()
			continue
		}
		if !dryRun {
			err = sched.AddTrigger(t)
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

func updatesHandler(c echo.Context) error {
	slugs := utils.SplitTrimString(c.QueryParam("Slugs"), ",")
	domain := c.QueryParam("Domain")
	domainsWithContext := c.QueryParam("DomainsWithContext")
	forceRegistry, _ := strconv.ParseBool(c.QueryParam("ForceRegistry"))
	onlyRegistry, _ := strconv.ParseBool(c.QueryParam("OnlyRegistry"))
	msg, err := jobs.NewMessage(&updates.Options{
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
	job, err := jobs.System().PushJob(prefixer.GlobalPrefixer, &jobs.JobRequest{
		WorkerType:  "updates",
		Message:     msg,
		Admin:       true,
		ForwardLogs: true,
	})
	if err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, job)
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
