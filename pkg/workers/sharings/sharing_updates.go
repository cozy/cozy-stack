package sharings

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"

	"github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
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
	Event   *EventDoc       `json:"event"`
	Message *SharingMessage `json:"message"`
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

// Sharing describes the sharing document structure
type Sharing struct {
	SharingType      string             `json:"sharing_type"`
	Permissions      permissions.Set    `json:"permissions,omitempty"`
	RecipientsStatus []*RecipientStatus `json:"recipients,omitempty"`
	Sharer           Sharer             `json:"sharer,omitempty"`
}

// Sharer gives the share info, only on the recipient side
type Sharer struct {
	URL          string           `json:"url"`
	SharerStatus *RecipientStatus `json:"sharer_status"`
}

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status       string               `json:"status,omitempty"`
	RefRecipient couchdb.DocReference `json:"recipient,omitempty"`
	AccessToken  *auth.AccessToken
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
	var res []Sharing
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
func sendToRecipients(ins *instance.Instance, domain string, sharing *Sharing, rule *permissions.Rule, docID, eventType string) error {
	var recInfos []*RecipientInfo

	if isRecipientSide(sharing) {
		// We are on the recipient side
		recInfos = make([]*RecipientInfo, 1)
		sharerStatus := sharing.Sharer.SharerStatus
		info, err := extractRecipient(ins, sharerStatus)
		if err != nil {
			return err
		}
		recInfos[0] = info
	} else {
		// We are on the sharer side
		recInfos = make([]*RecipientInfo, len(sharing.RecipientsStatus))
		for i, rec := range sharing.RecipientsStatus {
			info, err := extractRecipient(ins, rec)
			if err != nil {
				return err
			}
			recInfos[i] = info
		}
	}

	opts := &SendOptions{
		DocID:      docID,
		DocType:    rule.Type,
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
			logrus.Debugf("[sharings] Sending file: %#v", fileDoc)
			return SendFile(ins, opts, fileDoc)
		}
		if opts.Type == consts.DirType {
			ins.Logger().Debugf("[sharings] Sending directory: %#v", dirDoc)
			return SendDir(ins, opts, dirDoc)
		}

		logrus.Debugf("[sharings] Sending JSON (%v): %v", opts.DocType,
			opts.DocID)
		return SendDoc(ins, opts)

	case realtime.EventUpdate:
		if opts.Type == consts.FileType {
			if fileDoc.Trashed {
				logrus.Debugf("[sharings] Sending trash: %#v", fileDoc)
				return DeleteDirOrFile(opts)
			}

			stillShared := isDocumentStillShared(opts, fileDoc.ReferencedBy)
			if !stillShared {
				logrus.Debugf("[sharings] Sending remove references from %#v",
					fileDoc)
				return RemoveDirOrFileFromSharing(opts)
			}

			return UpdateOrPatchFile(ins, opts, fileDoc)
		}

		if opts.Type == consts.DirType {
			if dirDoc.DirID == consts.TrashDirID {
				logrus.Debugf("[sharings] Sending trash: %#v", dirDoc)
				return DeleteDirOrFile(opts)
			}

			stillShared := isDocumentStillShared(opts, dirDoc.ReferencedBy)
			if !stillShared {
				logrus.Debugf("[sharings] Sending remove references from %#v",
					dirDoc)
				return RemoveDirOrFileFromSharing(opts)
			}

			logrus.Debugf("[sharings] Sending patch dir %#v", dirDoc)
			return PatchDir(opts, dirDoc)
		}

		logrus.Debugf("[sharings] Sending update JSON (%v): %v", opts.DocType,
			opts.DocID)
		return UpdateDoc(ins, opts)

	case realtime.EventDelete:
		if opts.DocType == consts.Files {
			// The event "delete" for a file or a directory means that it has
			// been permanently deleted from the filesystem. We don't propagate
			// this event as of now.
			// TODO Do we propagate this event?
			return nil
		}
		return DeleteDoc(opts)

	default:
		return ErrEventNotSupported
	}
}

func extractRecipient(db couchdb.Database, rec *RecipientStatus) (*RecipientInfo, error) {
	recDoc, err := GetRecipient(db, rec.RefRecipient.ID)
	if err != nil {
		return nil, err
	}
	u, scheme, err := ExtractHostAndScheme(recDoc.M["url"].(string))

	if err != nil {
		return nil, err
	}
	info := &RecipientInfo{
		URL:    u,
		Scheme: scheme,
		Token:  rec.AccessToken.AccessToken,
	}
	return info, nil
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(db, consts.Recipients, recID, doc)
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
func isRecipientSide(sharing *Sharing) bool {
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
