package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/revision"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "index",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		Timeout:      5 * time.Minute,
		WorkerFunc:   Worker,
	})
}

// BatchSize is the maximal number of documents manipulated at once by the
// worker.
const BatchSize = 100

type IndexMessage struct {
	Doctype string `json:"doctype"`
}

func Worker(ctx *job.TaskContext) error {
	logger := ctx.Logger()
	inst := ctx.Instance
	var msg IndexMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	logger.Debugf("Index %s", msg.Doctype)
	if msg.Doctype != consts.Files {
		return errors.New("Only file can be indexed for the moment")
	}

	mu := config.Lock().ReadWrite(inst, "index/"+msg.Doctype)
	if err := mu.Lock(); err != nil {
		return err
	}
	defer mu.Unlock()

	lastSeq, err := getLastSeqNumber(inst, msg.Doctype)
	if err != nil {
		return err
	}
	feed, err := callChangesFeed(inst, msg.Doctype, lastSeq)
	if err != nil {
		return err
	}
	if feed.LastSeq == lastSeq {
		return nil
	}

	for _, change := range feed.Results {
		if err := callExternalIndexers(inst, msg.Doctype, change); err != nil {
			logger.Warnf("Index error: %s", err)
			return err
		}
	}
	_ = updateLastSequenceNumber(inst, msg.Doctype, feed.LastSeq)

	if feed.Pending > 0 {
		_ = pushJob(inst, msg.Doctype)
	}

	return nil
}

func callExternalIndexers(inst *instance.Instance, doctype string, change couchdb.Change) error {
	if strings.HasPrefix(change.DocID, "_design/") {
		return nil
	}
	if change.Doc.Get("type") == consts.DirType {
		return nil
	}

	indexers := inst.ExternalIndexers()
	for _, indexer := range indexers {
		u, err := url.Parse(indexer)
		if err != nil {
			return err
		}
		u.Path = fmt.Sprintf("/docs/%s/%s/%s", inst.Domain, doctype, change.DocID)
		if change.Deleted {
			req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
			if err != nil {
				return err
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode >= 500 {
				return fmt.Errorf("DELETE status code: %d", res.StatusCode)
			}
		} else {
			md5sum := fmt.Sprintf("%x", change.Doc.Get("md5sum"))
			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			if err != nil {
				return err
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()

			// When the content has not changed, there is no need to regenerate
			// an embedding.
			needIndexation := false
			switch res.StatusCode {
			case 200:
				var metadata map[string]interface{}
				if err = json.NewDecoder(res.Body).Decode(&metadata); err != nil {
					return err
				}
				needIndexation = metadata["md5sum"] != md5sum
			case 404:
				needIndexation = true
			default:
				return fmt.Errorf("GET status code: %d", res.StatusCode)
			}
			if !needIndexation {
				// TODO we should patch the metadata in the vector db when a
				// file has been moved/renamed.
				continue
			}

			dirID, _ := change.Doc.Get("dir_id").(string)
			name, _ := change.Doc.Get("name").(string)
			mime, _ := change.Doc.Get("mime").(string)
			internalID, _ := change.Doc.Get("internal_vfs_id").(string)
			u.RawQuery = url.Values{
				"dir_id": []string{dirID},
				"name":   []string{name},
				"md5sum": []string{md5sum},
			}.Encode()
			fs := inst.VFS()
			fileDoc := &vfs.FileDoc{
				Type:       consts.FileType,
				DocID:      change.DocID,
				DirID:      dirID,
				DocName:    name,
				InternalID: internalID,
			}
			// TODO notes with images
			content, err := fs.OpenFile(fileDoc)
			if err != nil {
				return err
			}
			defer content.Close()
			req, err = http.NewRequest(http.MethodPut, u.String(), content)
			if err != nil {
				return err
			}
			req.Header.Add("Content-Type", mime)
			res, err = http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode >= 500 {
				return fmt.Errorf("PUT status code: %d", res.StatusCode)
			}
		}
	}
	return nil
}

// getLastSeqNumber returns the last sequence number of the previous
// indexation for this doctype.
func getLastSeqNumber(inst *instance.Instance, doctype string) (string, error) {
	result, err := couchdb.GetLocal(inst, doctype, "index")
	if couchdb.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	seq, _ := result["last_seq"].(string)
	return seq, nil
}

// updateLastSequenceNumber updates the last sequence number for this
// indexation if it's superior to the number in CouchDB.
func updateLastSequenceNumber(inst *instance.Instance, doctype, seq string) error {
	result, err := couchdb.GetLocal(inst, doctype, "index")
	if err != nil {
		if !couchdb.IsNotFoundError(err) {
			return err
		}
		result = make(map[string]interface{})
	} else {
		if prev, ok := result["last_seq"].(string); ok {
			if revision.Generation(seq) <= revision.Generation(prev) {
				return nil
			}
		}
	}
	result["last_seq"] = seq
	return couchdb.PutLocal(inst, doctype, "index", result)
}

// callChangesFeed fetches the last changes from the changes feed
// http://docs.couchdb.org/en/stable/api/database/changes.html
func callChangesFeed(inst *instance.Instance, doctype, since string) (*couchdb.ChangesResponse, error) {
	return couchdb.GetChanges(inst, &couchdb.ChangesRequest{
		DocType:     doctype,
		IncludeDocs: true,
		Since:       since,
		Limit:       BatchSize,
	})
}

// pushJob adds a new job to continue on the pending documents in the changes
// feed.
func pushJob(inst *instance.Instance, doctype string) error {
	msg, err := job.NewMessage(&IndexMessage{
		Doctype: doctype,
	})
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "index",
		Message:    msg,
	})
	return err
}
