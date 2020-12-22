package instances

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

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
			log.FileDoc.FilesChildrenSize = 0
			log.FileDoc.Metadata = nil
		}
		if log.DirDoc != nil {
			log.DirDoc.DirsChildren = nil
			log.DirDoc.FilesChildren = nil
			log.DirDoc.FilesChildrenSize = 0
			log.DirDoc.Metadata = nil
		}
		if log.VersionDoc != nil {
			log.VersionDoc.Metadata = nil
		}
		if errenc := encoder.Encode(log); errenc != nil {
			i.Logger().WithField("nspace", "fsck").
				Warnf("Cannot encode to JSON: %s (%v)", errenc, log)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	if err != nil {
		log := map[string]string{"error": err.Error()}
		if errenc := encoder.Encode(log); errenc != nil {
			i.Logger().WithField("nspace", "fsck").
				Warnf("Cannot encode to JSON: %s (%v)", errenc, log)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	return nil
}

func checkTriggers(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return wrapError(err)
	}

	triggers, err := job.System().GetAllTriggers(inst)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusOK, []map[string]interface{}{
				{"error": err.Error()},
			})
		}
		return wrapError(err)
	}

	results := []map[string]interface{}{}
	for i, left := range triggers {
		for j, right := range triggers {
			if i >= j {
				continue
			}
			if left.Type() != right.Type() {
				continue
			}
			lInfos := left.Infos()
			rInfos := right.Infos()
			if lInfos.WorkerType != rInfos.WorkerType {
				continue
			}
			if lInfos.Arguments != rInfos.Arguments {
				continue
			}
			if lInfos.Debounce != rInfos.Debounce {
				continue
			}
			results = append(results, map[string]interface{}{
				"type":      "duplicate",
				"_id":       lInfos.ID(),
				"other_id":  rInfos.ID(),
				"trigger":   left.Type(),
				"worker":    lInfos.WorkerType,
				"arguments": lInfos.Arguments,
				"debounce":  lInfos.Debounce,
			})
		}
	}

	return c.JSON(http.StatusOK, results)
}

func checkShared(c echo.Context) error {
	domain := c.Param("domain")
	i, err := lifecycle.GetInstance(domain)
	if err != nil {
		return wrapError(err)
	}

	results, err := sharing.CheckShared(i)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusOK, []map[string]interface{}{
				{"error": err.Error()},
			})
		}
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, results)
}

func checkSharings(c echo.Context) error {
	domain := c.Param("domain")
	i, err := lifecycle.GetInstance(domain)
	if err != nil {
		return wrapError(err)
	}

	results, err := sharing.CheckSharings(i)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusOK, []map[string]interface{}{
				{"error": err.Error()},
			})
		}
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, results)
}
