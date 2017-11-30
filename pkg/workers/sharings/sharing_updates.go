package sharings

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
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
	// ErrRecipientHasNoURL is used to signal that a recipient has no URL.
	ErrRecipientHasNoURL = errors.New("Recipient has no URL")
	// ErrEventNotSupported is used to signal that the event propagated by the
	// trigger is not supported by this worker.
	ErrEventNotSupported = errors.New("Event not supported")
)

// SharingMessage describes a sharing message
type SharingMessage struct {
	SharingID string           `json:"sharing_id"`
	Rule      permissions.Rule `json:"rule"`
}

// SharingUpdates handles shared document updates
func SharingUpdates(ctx *jobs.WorkerContext) error {
	domain := ctx.Domain()
	var event struct {
		Verb string `json:"verb"`
		Doc  *couchdb.JSONDoc
	}
	if err := ctx.UnmarshalEvent(&event); err != nil {
		return err
	}

	var msg *sharings.SharingMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	sharingID := msg.SharingID
	rule := msg.Rule
	docID := event.Doc.ID()

	// Get the sharing document
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	sharing, err := sharings.FindSharing(i, sharingID)
	if err != nil {
		return ErrSharingDoesNotExist
	}

	// One-Shot sharing do not propagate updates.
	if sharing.SharingType == consts.OneShotSharing {
		return ErrDocumentNotLegitimate
	}

	return sendToRecipients(i, domain, sharing, &rule, docID, event.Verb)
}

// sendToRecipients sends the document to the recipient, or sharer.
//
// Several scenario are to be distinguished:
// TODO explanation
func sendToRecipients(ins *instance.Instance, domain string, sharing *sharings.Sharing, rule *permissions.Rule, docID, eventType string) error {
	var recInfos []*sharings.RecipientInfo

	// sharing revoked: drop it
	// NOTE: this should never happen as a revoked sharing removes the triggers
	if sharing.Revoked {
		return nil
	}
	sendToSharer := !sharing.Owner

	if sendToSharer {
		// We are on the recipient side
		recInfos = make([]*sharings.RecipientInfo, 1)
		sharerStatus := sharing.Sharer
		info, err := sharings.ExtractRecipientInfo(&sharerStatus)
		if err != nil {
			return err
		}
		recInfos[0] = info

	} else {
		// We are on the sharer side
		for _, rec := range sharing.Recipients {
			// Ignore the revoked recipients
			if rec.Status != consts.SharingStatusRevoked {
				info, err := sharings.ExtractRecipientInfo(&rec)
				if err != nil {
					return err
				}
				recInfos = append(recInfos, info)
			}
		}
	}

	if len(recInfos) == 0 {
		return nil
	}

	opts := &SendOptions{
		DocID:      docID,
		DocType:    rule.Type,
		SharingID:  sharing.SID,
		Recipients: recInfos,
		Selector:   rule.Selector,
		Values:     rule.Values,
		Path:       fmt.Sprintf("/sharings/doc/%s/%s", rule.Type, docID),
	}

	var fileDoc *vfs.FileDoc
	var dirDoc *vfs.DirDoc
	var err error
	fs := ins.VFS()

	if opts.DocType == consts.Files && eventType != realtime.EventDelete {
		dirDoc, fileDoc, err = fs.DirOrFileByID(docID)
		if err != nil {
			// If deleted: propagate event
			errVfs := err.(*couchdb.Error)
			if errVfs.StatusCode == http.StatusNotFound {
				return DeleteDirOrFile(ins, opts, true)
			}
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
				return DeleteDirOrFile(ins, opts, false)
			}

			stillShared := isDocumentStillShared(fs, opts, fileDoc.ReferencedBy)
			if !stillShared {
				ins.Logger().Debugf("[sharings] sharing_update: Sending "+
					"remove references from %#v", fileDoc)
				return RemoveDirOrFileFromSharing(ins, opts, sendToSharer)
			}
			return UpdateOrPatchFile(ins, opts, fileDoc, sendToSharer)
		}

		if opts.Type == consts.DirType {
			if dirDoc.DirID == consts.TrashDirID {
				// Particular case: the recipient removes the target of the sharing,
				// e.g. the shared directory
				if sendToSharer && isSharedContainer(rule, docID) {
					err := sharings.RevokeSharing(ins, sharing, true)
					if err != nil {
						return err
					}
				}
				ins.Logger().Debugf("[sharings] sharing_update: Sending "+
					"trash: %v", dirDoc)
				return DeleteDirOrFile(ins, opts, false)
			}

			stillShared := isDocumentStillShared(fs, opts, dirDoc.ReferencedBy)
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
		// Particular case: the recipient removes the target of the sharing,
		// e.g. a photo album
		if sendToSharer && isSharedContainer(rule, docID) {
			err := sharings.RevokeSharing(ins, sharing, true)
			if err != nil {
				return err
			}
		}
		return DeleteDoc(ins, opts)

	default:
		return ErrEventNotSupported
	}
}

// This function checks if the document with the given ID still belong in the
// sharing that triggered the worker.
//
// As the trigger is started by either the current revision of said document or
// by the previous one, we don't have another way of knowing if the document is
// still shared.
//
// TODO Handle sharing of directories
func isDocumentStillShared(fs vfs.VFS, opts *SendOptions, docRefs []couchdb.DocReference) bool {
	switch opts.Selector {
	case consts.SelectorReferencedBy:
		relevantRefs := opts.extractRelevantReferences(docRefs)

		return len(relevantRefs) > 0

	default:
		return isShared(fs, opts.DocID, opts.Values)
	}
}

// isSharedContainer returns true if the given doc is the target of the sharing
func isSharedContainer(rule *permissions.Rule, docID string) bool {
	if rule.Selector == "" {
		// Single file or directory sharing
		if len(rule.Values) == 1 {
			if rule.Values[0] == docID {
				return true
			}
		}
	}
	return false
}
