package data

import (
	"net/http"
	"strconv"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func proxy(c echo.Context, path string) error {
	doctype := c.Get("doctype").(string)
	instance := middlewares.GetInstance(c)
	p := couchdb.Proxy(instance, doctype, path)
	p.ServeHTTP(c.Response(), c.Request())
	return nil
}

func getDesignDoc(c echo.Context) error {
	docid := c.Param("designdocid")
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if paramIsTrue(c, "revs") {
		return proxy(c, "_design/"+docid)
	}

	return c.JSON(http.StatusBadRequest, echo.Map{
		"error": "_design docs are only readable for replication",
	})
}

func getLocalDoc(c echo.Context) error {
	doctype := c.Get("doctype").(string)
	docid := c.Param("docid")

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if err := permission.CheckReadable(doctype); err != nil {
		return err
	}

	return proxy(c, "_local/"+docid)
}

func setLocalDoc(c echo.Context) error {
	doctype := c.Get("doctype").(string)
	docid := c.Param("docid")

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if err := permission.CheckReadable(doctype); err != nil {
		return err
	}

	return proxy(c, "_local/"+docid)
}

func bulkGet(c echo.Context) error {
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if err := permission.CheckReadable(doctype); err != nil {
		return err
	}

	return proxy(c, "_bulk_get")
}

func bulkDocs(c echo.Context) error {
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.POST, doctype); err != nil {
		return err
	}

	if err := permission.CheckWritable(doctype); err != nil {
		return err
	}

	instance := middlewares.GetInstance(c)
	if err := couchdb.EnsureDBExist(instance, doctype); err != nil {
		return err
	}
	p, req, err := couchdb.ProxyBulkDocs(instance, doctype, c.Request())
	if err != nil {
		var code int
		if errHTTP, ok := err.(*echo.HTTPError); ok {
			code = errHTTP.Code
		} else {
			code = http.StatusInternalServerError
		}
		return c.JSON(code, echo.Map{
			"error": err.Error(),
		})
	}

	p.ServeHTTP(c.Response(), req)
	return nil
}

func createDB(c echo.Context) error {
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.POST, doctype); err != nil {
		return err
	}

	if err := permission.CheckWritable(doctype); err != nil {
		return err
	}

	return proxy(c, "/")
}

func fullCommit(c echo.Context) error {
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if err := permission.CheckWritable(doctype); err != nil {
		return err
	}

	return proxy(c, "_ensure_full_commit")
}

func revsDiff(c echo.Context) error {
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if err := permission.CheckReadable(doctype); err != nil {
		return err
	}

	return proxy(c, "_revs_diff")
}

var allowedChangesParams = map[string]bool{
	"feed":         true,
	"style":        true,
	"since":        true,
	"limit":        true,
	"timeout":      true,
	"include_docs": true,
	"heartbeat":    true, // Pouchdb sends heartbeet even for non-continuous
	"_nonce":       true, // Pouchdb sends a request hash to avoid aggressive caching by some browsers
	"seq_interval": true,
}

func changesFeed(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var doctype = c.Get("doctype").(string)

	// Drop a clear error for parameters not supported by stack
	for key := range c.QueryParams() {
		if !allowedChangesParams[key] {
			return jsonapi.Errorf(http.StatusBadRequest, "Unsupported query parameter '%s'", key)
		}
	}

	feed, err := couchdb.ValidChangesMode(c.QueryParam("feed"))
	if err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	feedStyle, err := couchdb.ValidChangesStyle(c.QueryParam("style"))
	if err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	limitString := c.QueryParam("limit")
	limit := 0
	if limitString != "" {
		if limit, err = strconv.Atoi(limitString); err != nil {
			return jsonapi.Errorf(http.StatusBadRequest, "Invalid limit value '%s': %s", limitString, err.Error())
		}
	}

	seqIntervalString := c.QueryParam("seq_interval")
	seqInterval := 0
	if seqIntervalString != "" {
		if seqInterval, err = strconv.Atoi(seqIntervalString); err != nil {
			return jsonapi.Errorf(http.StatusBadRequest, "Invalid seq_interval value '%s': %s", seqIntervalString, err.Error())
		}
	}

	includeDocs := paramIsTrue(c, "include_docs")

	if err = permission.CheckReadable(doctype); err != nil {
		return err
	}

	if err = middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	// Use the VFS lock for the files to avoid sending the changed feed while
	// the VFS is moving a directory.
	if doctype == consts.Files {
		mu := lock.ReadWrite(instance, "vfs")
		if err := mu.Lock(); err != nil {
			return err
		}
		defer mu.Unlock()
	}

	results, err := couchdb.GetChanges(instance, &couchdb.ChangesRequest{
		DocType:     doctype,
		Feed:        feed,
		Style:       feedStyle,
		Since:       c.QueryParam("since"),
		Limit:       limit,
		IncludeDocs: includeDocs,
		SeqInterval: seqInterval,
	})

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, results)
}

func dbStatus(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)

	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
		return err
	}

	if err := permission.CheckReadable(doctype); err != nil {
		return err
	}

	status, err := couchdb.DBStatus(instance, doctype)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, status)
}

func replicationRoutes(group *echo.Group) {
	group.PUT("/", createDB)

	// Routes used only for replication
	group.GET("/", dbStatus)
	group.GET("/_design/:designdocid", getDesignDoc)
	group.GET("/_changes", changesFeed)
	// POST=GET see http://docs.couchdb.org/en/stable/api/database/changes.html#post--db-_changes)
	group.POST("/_changes", changesFeed)

	group.POST("/_ensure_full_commit", fullCommit)

	// useful for Pouchdb replication
	group.POST("/_bulk_get", bulkGet) // https://github.com/couchbase/sync_gateway/wiki/Bulk-GET
	group.POST("/_bulk_docs", bulkDocs)

	group.POST("/_revs_diff", revsDiff)

	// for storing checkpoints
	group.GET("/_local/:docid", getLocalDoc)
	group.PUT("/_local/:docid", setLocalDoc)
}
