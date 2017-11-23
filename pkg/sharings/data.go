package sharings

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

// ShareDoc shares the documents specified in the Sharing structure to the
// specified recipient
func ShareDoc(instance *instance.Instance, sharing *Sharing, recStatus *RecipientStatus) error {
	for _, rule := range sharing.Permissions {
		if len(rule.Values) == 0 {
			return nil
		}
		// Trigger the updates if the sharing is not one-shot
		if sharing.SharingType != consts.OneShotSharing {
			err := AddTrigger(instance, rule, sharing.SharingID, false)
			if err != nil {
				return err
			}
		}

		var values []string
		var err error
		if rule.Selector != "" {
			// Selector-based sharing
			values, err = sharingBySelector(instance, rule)
			if err != nil {
				return err
			}
		} else {
			// Value-based sharing
			values, err = sharingByValues(instance, rule)
			if err != nil {
				return err
			}
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

func sendData(instance *instance.Instance, sharing *Sharing, recStatus *RecipientStatus, values []string, rule permissions.Rule) error {
	// Create a sharedata worker for each doc to send
	for _, val := range values {
		domain, scheme, err := ExtractDomainAndScheme(recStatus.recipient)
		if err != nil {
			return err
		}
		rec := &RecipientInfo{
			URL:         domain,
			Scheme:      scheme,
			AccessToken: recStatus.AccessToken,
			Client:      recStatus.Client,
		}

		workerMsg, err := jobs.NewMessage(WorkerData{
			DocID:      val,
			SharingID:  sharing.SharingID,
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

	// TODO Using a cursor might lead to inconsistency. Change it if the need
	// arises.
	cursor := couchdb.NewSkipCursor(10000, 0)

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

	for {
		perms, errg := permissions.GetSharedWithMePermissionsByDoctype(ins,
			doctype, cursor)
		if errg != nil {
			return errg
		}

		for _, perm := range perms {
			if perm.Permissions.Allow(permissions.GET, doc) ||
				perm.Permissions.Allow(permissions.POST, doc) ||
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
