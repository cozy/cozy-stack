package sharings

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

func init() {
	jobs.AddWorker("sharingupdates", &jobs.WorkerConfig{
		Concurrency: runtime.NumCPU(),
		WorkerFunc:  SharingUpdates,
	})
}

var (
	// ErrSharingIDNotUnique is used when several occurences of the same sharing id are found
	ErrSharingIDNotUnique = errors.New("Several sharings with this id found")
	// ErrSharingDoesNotExist is used when the given sharing does not exist.
	ErrSharingDoesNotExist = errors.New("Sharing does not exist")
	// ErrDocumentNotLegitimate is used when a shared document is triggered but
	// not legitimate for this sharing
	ErrDocumentNotLegitimate = errors.New("Triggered illegitimate shared document")
	//ErrRecipientDoesNotExist is used when the given recipient does not exist
	ErrRecipientDoesNotExist = errors.New("Recipient with given ID does not exist")
	// ErrRecipientHasNoURL is used to signal that a recipient has no URL.
	ErrRecipientHasNoURL = errors.New("Recipient has no URL")
	// ErrEventNotSupported is used to signal that the event propagated by the
	// trigger is not supported by this worker.
	ErrEventNotSupported = errors.New("Event not supported")
)

// TriggerEvent describes the fields retrieved after a triggered event
type TriggerEvent struct {
	Event   *EventDoc                `json:"event"`
	Message *sharings.SharingMessage `json:"message"`
}

// EventDoc describes the event returned by the trigger
type EventDoc struct {
	Type string `json:"type"`
	Doc  *couchdb.JSONDoc
}

// SharingMessage describes a sharing message
type SharingMessage struct {
	SharingID string           `json:"sharing_id"`
	Rule      permissions.Rule `json:"rule"`
}

// SharingUpdates handles shared document updates
func SharingUpdates(ctx context.Context, m *jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)

	event := &TriggerEvent{}
	err := m.Unmarshal(&event)
	if err != nil {
		return err
	}
	sharingID := event.Message.SharingID
	rule := event.Message.Rule
	docID := event.Event.Doc.M["_id"].(string)

	// Get the sharing document
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	var res []sharings.Sharing
	err = couchdb.FindDocs(i, consts.Sharings, &couchdb.FindRequest{
		UseIndex: "by-sharing-id",
		Selector: mango.Equal("sharing_id", sharingID),
	}, &res)
	if err != nil {
		return err
	}
	if len(res) < 1 {
		return ErrSharingDoesNotExist
	} else if len(res) > 1 {
		return ErrSharingIDNotUnique
	}
	sharing := &res[0]

	// One-Shot sharing do not propagate updates.
	if sharing.SharingType == consts.OneShotSharing {
		return ErrDocumentNotLegitimate
	}

	return sendToRecipients(i, domain, sharing, &rule, docID, event.Event.Type)
}

// sendToRecipients sends the document to the recipient, or sharer.
//
// Several scenario are to be distinguished:
// TODO explanation
func sendToRecipients(ins *instance.Instance, domain string, sharing *sharings.Sharing, rule *permissions.Rule, docID, eventType string) error {
	var recInfos []*sharings.RecipientInfo
	sendToSharer := isRecipientSide(sharing)

	if sendToSharer {
		// We are on the recipient side
		recInfos = make([]*sharings.RecipientInfo, 1)
		sharerStatus := sharing.Sharer.SharerStatus
		info, err := extractRecipient(ins, sharerStatus)
		if err != nil {
			return err
		}
		recInfos[0] = info
	} else {
		// We are on the sharer side
		for _, rec := range sharing.RecipientsStatus {
			// Ignore the revoked recipients
			if rec.Status != consts.SharingStatusRevoked {
				info, err := extractRecipient(ins, rec)
				if err != nil {
					return err
				}
				recInfos = append(recInfos, info)
			}
		}
	}

	opts := &SendOptions{
		DocID:      docID,
		DocType:    rule.Type,
		SharingID:  sharing.SharingID,
		Recipients: recInfos,
		Selector:   rule.Selector,
		Values:     rule.Values,

		Path: fmt.Sprintf("/sharings/doc/%s/%s", rule.Type, docID),
	}

	var fileDoc *vfs.FileDoc
	var dirDoc *vfs.DirDoc
	var err error
	if opts.DocType == consts.Files && eventType != realtime.EventDelete {
		fs := ins.VFS()
		dirDoc, fileDoc, err = fs.DirOrFileByID(docID)
		if err != nil {
			return err
		}

		if dirDoc != nil {
			opts.Type = consts.DirType
		} else {
			opts.Type = consts.FileType
		}
	}

	switch eventType {
	case realtime.EventCreate:
		if opts.Type == consts.FileType {
			ins.Logger().Debugf("[sharings] sharing_update: Sending file: %#v",
				fileDoc)
			return SendFile(ins, opts, fileDoc)
		}
		if opts.Type == consts.DirType {
			ins.Logger().Debugf("[sharings] sharing_update: Sending "+
				"directory: %#v", dirDoc)
			return SendDir(ins, opts, dirDoc)
		}

		ins.Logger().Debugf("[sharings] sharing_update: Sending %v: %v",
			opts.DocType, opts.DocID)
		return SendDoc(ins, opts)

	case realtime.EventUpdate:
		if opts.Type == consts.FileType {
			if fileDoc.Trashed {
				ins.Logger().Debugf("[sharings] sharing_update: Sending "+
					"trash: %#v", fileDoc)
				return DeleteDirOrFile(ins, opts)
			}

			stillShared := isDocumentStillShared(opts, fileDoc.ReferencedBy)
			if !stillShared {
				ins.Logger().Debugf("[sharings] sharing_update: Sending "+
					"remove references from %#v", fileDoc)
				return RemoveDirOrFileFromSharing(ins, opts, sendToSharer)
			}

			return UpdateOrPatchFile(ins, opts, fileDoc, sendToSharer)
		}

		if opts.Type == consts.DirType {
			if dirDoc.DirID == consts.TrashDirID {
				ins.Logger().Debugf("[sharings] sharing_update: Sending "+
					"trash: %v", dirDoc)
				return DeleteDirOrFile(ins, opts)
			}

			stillShared := isDocumentStillShared(opts, dirDoc.ReferencedBy)
			if !stillShared {
				ins.Logger().Debugf("[sharings] sharing_update: Sending "+
					"remove references from %v", dirDoc)
				return RemoveDirOrFileFromSharing(ins, opts, sendToSharer)
			}

			ins.Logger().Debugf("[sharings] sharing_update: Sending patch "+
				"dir: %v", dirDoc)
			return PatchDir(ins, opts, dirDoc)
		}

		ins.Logger().Debugf("[sharings] sharing_update: Sending update "+
			"%s: %s", opts.DocType, opts.DocID)
		return UpdateDoc(ins, opts)

	case realtime.EventDelete:
		if opts.DocType == consts.Files {
			// The event "delete" for a file or a directory means that it has
			// been permanently deleted from the filesystem. We don't propagate
			// this event as of now.
			// TODO Do we propagate this event?
			return nil
		}
		return DeleteDoc(ins, opts)

	default:
		return ErrEventNotSupported
	}
}

func extractRecipient(db couchdb.Database, rec *sharings.RecipientStatus) (*sharings.RecipientInfo, error) {
	recDoc, err := GetRecipient(db, rec.RefRecipient.ID)
	if err != nil {
		return nil, err
	}
	u, scheme, err := ExtractHostAndScheme(recDoc.M["url"].(string))

	if err != nil {
		return nil, err
	}
	info := &sharings.RecipientInfo{
		URL:         u,
		Scheme:      scheme,
		AccessToken: rec.AccessToken,
		Client:      rec.Client,
	}
	return info, nil
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(db, consts.Contacts, recID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrRecipientDoesNotExist
	}
	return doc, err
}

// ExtractHostAndScheme returns the recipient's host and the scheme
func ExtractHostAndScheme(fullURL string) (string, string, error) {
	if fullURL == "" {
		return "", "", ErrRecipientHasNoURL
	}
	u, err := url.Parse(fullURL)
	if err != nil {
		return "", "", err
	}
	host := u.Host
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return host, scheme, nil
}

// isRecipientSide is used to determine whether or not we are on the recipient side.
// A sharing is on the recipient side iff:
// - the sharing type is master-master
// - the SharerStatus structure is not nil
func isRecipientSide(sharing *sharings.Sharing) bool {
	if sharing.SharingType == consts.MasterMasterSharing {
		if sharing.Sharer.SharerStatus != nil {
			return true
		}
	}
	return false
}

// This function checks if the document with the given ID still belong in the
// sharing that triggered the worker.
//
// As the trigger is started by either the current revision of said document or
// by the previous one, we don't have another way of knowing if the document is
// still shared.
//
// TODO Handle sharing of directories
func isDocumentStillShared(opts *SendOptions, docRefs []couchdb.DocReference) bool {
	switch opts.Selector {
	case consts.SelectorReferencedBy:
		relevantRefs := opts.extractRelevantReferences(docRefs)

		return len(relevantRefs) > 0

	default:
		return isShared(opts.DocID, opts.Values)
	}
}
