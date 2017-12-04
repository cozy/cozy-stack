package sharings

import (
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

const (
	// WorkerTypeSharingUpdates is the string representation of the type of
	// workers that deals with updating sharings.
	WorkerTypeSharingUpdates = "sharingupdates"
)

// WorkerData describes the basic data the workers need to process the events
// they will receive.
type WorkerData struct {
	DocID      string
	SharingID  string
	Selector   string
	Values     []string
	DocType    string
	Recipients []*RecipientInfo
}

// SharingMessage describes the message that will be transmitted to the workers
// "sharing_update" and "share_data".
type SharingMessage struct {
	SharingID string           `json:"sharing_id"`
	Rule      permissions.Rule `json:"rule"`
}

// AddTrigger creates a new trigger on the updates of the shared documents
// The delTrigger flag is when the trigger must only listen deletions, i.e.
// an one-way on the recipient side, for the revocation
func AddTrigger(instance *instance.Instance, rule permissions.Rule, sharingID string, delTrigger bool) error {
	sched := globals.GetScheduler()

	var eventArgs string
	if rule.Selector != "" {
		// TODO to be confirmed, but it looks like we shouldn't add a trigger
		// when delTrigger is true when there is a selector for the rule
		eventArgs = rule.Type + ":CREATED,UPDATED,DELETED:" +
			strings.Join(rule.Values, ",") + ":" + rule.Selector
	} else {
		if delTrigger {
			eventArgs = rule.Type + ":DELETED:" +
				strings.Join(rule.Values, ",")
		} else {
			eventArgs = rule.Type + ":CREATED,UPDATED,DELETED:" +
				strings.Join(rule.Values, ",")
		}

	}

	msg := SharingMessage{
		SharingID: sharingID,
		Rule:      rule,
	}

	workerArgs, err := jobs.NewMessage(msg)
	if err != nil {
		return err
	}
	t, err := scheduler.NewTrigger(&scheduler.TriggerInfos{
		Type:       "@event",
		WorkerType: WorkerTypeSharingUpdates,
		Domain:     instance.Domain,
		Arguments:  eventArgs,
		Message:    workerArgs,
	})
	if err != nil {
		return err
	}
	instance.Logger().Infof("[sharings] AddTrigger: trigger created for "+
		"sharing %s", sharingID)

	return sched.Add(t)
}

func removeSharingTriggers(ins *instance.Instance, sharingID string) error {
	sched := globals.GetScheduler()
	ts, err := sched.GetAll(ins.Domain)
	if err != nil {
		ins.Logger().Errorf("[sharings] removeSharingTriggers: Could not get "+
			"the list of triggers: %s", err)
		return err
	}

	for _, trigger := range ts {
		infos := trigger.Infos()
		if infos.WorkerType == WorkerTypeSharingUpdates {
			msg := SharingMessage{}
			errm := infos.Message.Unmarshal(&msg)
			if errm != nil {
				ins.Logger().Errorf("[sharings] removeSharingTriggers: An "+
					"error occurred while trying to unmarshal trigger "+
					"message: %s", errm)
				continue
			}

			if msg.SharingID == sharingID {
				errd := sched.Delete(ins.Domain, trigger.ID())
				if errd != nil {
					ins.Logger().Errorf("[sharings] removeSharingTriggers: "+
						"Could not delete trigger %s: %s", trigger.ID(), errd)
				}

				ins.Logger().Infof("[sharings] Trigger %s deleted for "+
					"sharing %s", trigger.ID(), sharingID)
			}
		}
	}

	return nil
}

// ShareDoc shares the documents specified in the Sharing structure to the
// specified recipient
func ShareDoc(instance *instance.Instance, sharing *Sharing, recStatus *Member) error {
	perms, err := sharing.Permissions(instance)
	if err != nil {
		return err
	}
	for _, rule := range perms.Permissions {
		if len(rule.Values) == 0 {
			return nil
		}
		// Trigger the updates if the sharing is not one-shot
		if sharing.SharingType != consts.OneShotSharing {
			err := AddTrigger(instance, rule, sharing.SID, false)
			if err != nil {
				return err
			}
		}

		var values []string
		var err error
		if rule.Selector != "" {
			// Selector-based sharing
			values, err = sharingBySelector(instance, rule)
		} else {
			// Value-based sharing
			values, err = sharingByValues(instance, rule)
		}
		if err != nil {
			return err
		}
		err = sendData(instance, sharing, recStatus, values, rule)
		if err != nil {
			return err
		}
	}
	return nil
}

// sharingBySelector returns the ids to share based on the Rule selector
func sharingBySelector(instance *instance.Instance, rule permissions.Rule) ([]string, error) {
	var values []string

	// Particular case for referenced_by: use the existing view
	if rule.Selector == consts.SelectorReferencedBy {
		for _, val := range rule.Values {
			// A referenced_by selector implies Values in the form
			// ["refDocType/refId"]
			parts := strings.Split(val, permissions.RefSep)
			if len(parts) != 2 {
				return nil, ErrBadPermission
			}
			refType := parts[0]
			refID := parts[1]
			req := &couchdb.ViewRequest{
				Key:    []string{refType, refID},
				Reduce: false,
			}
			var res couchdb.ViewResponse
			err := couchdb.ExecView(instance,
				consts.FilesReferencedByView, req, &res)
			if err != nil {
				return nil, err
			}
			for _, row := range res.Rows {
				values = append(values, row.ID)
			}

		}
	} else {
		// Create index based on selector to retrieve documents to share
		indexName := "by-" + rule.Selector
		index := mango.IndexOnFields(rule.Type, indexName,
			[]string{rule.Selector})
		err := couchdb.DefineIndex(instance, index)
		if err != nil {
			return nil, err
		}

		var docs []couchdb.JSONDoc

		// Request the index for all values
		// NOTE: this is not efficient in case of many Values
		// We might consider a map-reduce approach in case of bottleneck
		for _, val := range rule.Values {
			err = couchdb.FindDocs(instance, rule.Type,
				&couchdb.FindRequest{
					UseIndex: indexName,
					Selector: mango.Equal(rule.Selector, val),
				}, &docs)
			if err != nil {
				return nil, err
			}
			// Save returned doc ids
			for _, d := range docs {
				values = append(values, d.ID())
			}
		}
	}
	return values, nil
}

// sharingByValues returns the ids to share based on the Rule values
func sharingByValues(instance *instance.Instance, rule permissions.Rule) ([]string, error) {
	var values []string

	// Iterate on values to detect directory sharing
	for _, val := range rule.Values {
		if rule.Type == consts.Files {
			fs := instance.VFS()
			dirDoc, _, err := fs.DirOrFileByID(val)
			if err != nil {
				return nil, err
			}
			// Directory sharing: get all hierarchy
			if dirDoc != nil {
				rootPath, err := dirDoc.Path(fs)
				if err != nil {
					return nil, err
				}
				err = vfs.Walk(fs, rootPath, func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
					if err != nil {
						return err
					}
					var id string
					if dir != nil {
						id = dir.ID()
					} else if file != nil {
						id = file.ID()
					}
					values = append(values, id)

					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				// The value is a file: no particular treatment
				values = append(values, val)
			}
		} else {
			// Not a file nor directory: no particular treatment
			values = append(values, val)
		}
	}
	return values, nil
}

func sendData(instance *instance.Instance, sharing *Sharing, recStatus *Member, values []string, rule permissions.Rule) error {
	// Create a sharedata worker for each doc to send
	for _, val := range values {
		if recStatus.URL == "" {
			return ErrRecipientHasNoURL
		}
		u, err := url.Parse(recStatus.URL)
		if err != nil {
			return err
		}
		rec := &RecipientInfo{
			Domain:      u.Host,
			Scheme:      u.Scheme,
			AccessToken: recStatus.AccessToken,
			Client:      recStatus.Client,
		}

		workerMsg, err := jobs.NewMessage(WorkerData{
			DocID:      val,
			SharingID:  sharing.SID,
			Selector:   rule.Selector,
			Values:     rule.Values,
			DocType:    rule.Type,
			Recipients: []*RecipientInfo{rec},
		})
		if err != nil {
			return err
		}
		_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
			Domain:     instance.Domain,
			WorkerType: "sharedata",
			Options:    nil,
			Message:    workerMsg,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// RemoveDocumentIfNotShared checks if the given document is still shared and
// removes it if not.
//
// To check if a document is still shared all the permissions associated with
// sharings that apply to its doctype are fetched. If at least one permission
// "matches" then the document is kept.
func RemoveDocumentIfNotShared(ins *instance.Instance, doctype, docID string) error {
	fs := ins.VFS()

	doc := couchdb.JSONDoc{}
	err := couchdb.GetDoc(ins, doctype, docID, &doc)
	if err != nil {
		return err
	}

	// The doctype is not always set, at least in the tests, and is required in
	// order to delete the document.
	if doc.DocType() == "" {
		doc.Type = doctype
	}

	cursor := couchdb.NewSkipCursor(10000, 0)
	for {
		perms, errg := permissions.GetSharedWithMePermissionsByDoctype(ins, doctype, cursor)
		if errg != nil {
			return errg
		}

		for _, perm := range perms {
			if perm.Permissions.Allow(permissions.GET, doc) ||
				perm.Permissions.Allow(permissions.POST, doc) ||
				perm.Permissions.Allow(permissions.PATCH, doc) ||
				perm.Permissions.Allow(permissions.PUT, doc) ||
				perm.Permissions.Allow(permissions.DELETE, doc) {
				return nil
			}
		}

		if !cursor.HasMore() {
			break
		}
	}

	ins.Logger().Debugf("[sharings] Document %s is no longer shared, "+
		"removing it", docID)

	switch doctype {
	case consts.Files:
		dirDoc, fileDoc, errd := fs.DirOrFileByID(docID)
		if errd != nil {
			return errd
		}

		if dirDoc != nil {
			_, errt := vfs.TrashDir(fs, dirDoc)
			return errt
		}

		_, errt := vfs.TrashFile(fs, fileDoc)
		return errt

	default:
		return couchdb.DeleteDoc(ins, doc)
	}
}
