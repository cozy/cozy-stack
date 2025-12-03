package sharing

import (
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/notification"
	"github.com/cozy/cozy-stack/model/notification/center"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
)

func sendFileChangeNotification(inst *instance.Instance, description, fileName, fileOrDirID, parentID string, isFolder bool) error {
	// Format for file: https://alice.example.com/#/folder/{parent_folder_id}/file/{file_id}
	itemURL := inst.SubDomain(consts.DriveSlug)
	if isFolder {
		itemURL.Fragment = "/folder/" + url.PathEscape(fileOrDirID)
	} else {
		itemURL.Fragment = "/folder/" + url.PathEscape(parentID) + "/file/" + url.PathEscape(fileOrDirID)
	}

	n := &notification.Notification{
		Title: inst.Translate("Mail Sharing File Changed Subject", description),
		Slug:  consts.DriveSlug,
		Data: map[string]interface{}{
			"SharingDescription": description,
			"FileName":           fileName,
			"FileURL":            itemURL.String(),
			"IsFolder":           isFolder,
		},
		PreferredChannels: []string{"mail"},
	}
	return center.PushStack(inst.DomainName(), center.NotificationSharingFileChanged, n)
}

// MaybeNotifyFileChange checks if a file change notification should be sent.
// Only sends notifications when a file or folder is CREATED, not updated or removed.
func MaybeNotifyFileChange(inst *instance.Instance, msg TrackMessage, evt TrackEvent) {
	// Check if notifications are enabled for this context
	cfg := config.GetSharingNotificationsConfig(inst.ContextName)
	if !cfg.Enabled {
		return
	}

	// Only notify for file changes
	if msg.DocType != consts.Files {
		return
	}

	// Only notify on CREATED events, not updates or deletions
	if evt.Verb != "CREATED" {
		return
	}

	// Get the sharing
	s, err := FindSharing(inst, msg.SharingID)
	if err != nil {
		inst.Logger().WithNamespace("sharing").
			Warnf("Cannot find sharing for notification: %s", err)
		return
	}
	// Only send notification if this is the owner's instance
	if !s.Owner {
		return
	}
	// Only send for file-based sharings
	if s.FirstFilesRule() == nil {
		return
	}

	// Check if this is a folder or a file
	docType, _ := evt.Doc.Get("type").(string)
	isFolder := docType == consts.DirType

	// Get file/folder name from the event
	fileName, _ := evt.Doc.Get("name").(string)
	if fileName == "" {
		fileName = "unknown"
	}

	// Get file/folder ID from the event
	fileID := evt.Doc.ID()
	if fileID == "" {
		return
	}

	// Get parent folder ID from the event
	dirID, _ := evt.Doc.Get("dir_id").(string)
	if dirID == "" {
		return
	}

	// Send the notification (async, don't block on errors)
	go func() {
		if err := sendFileChangeNotification(inst, s.Description, fileName, fileID, dirID, isFolder); err != nil {
			inst.Logger().WithNamespace("sharing").
				Warnf("Failed to send file change notification: %s", err)
		}
	}()
}

// MaybeNotifyShareByLinkUpload checks if a notification should be sent when
// a file or folder is uploaded via a share-by-link (public link).
func MaybeNotifyShareByLinkUpload(inst *instance.Instance, fileName, fileID, parentID string, isFolder bool) {
	// Check if notifications are enabled for this context
	cfg := config.GetSharingNotificationsConfig(inst.ContextName)
	if !cfg.Enabled {
		return
	}

	if fileName == "" {
		fileName = "unknown"
	}

	if fileID == "" || parentID == "" {
		return
	}

	// For share-by-link, we use the parent folder name as description
	description := ""
	if parentDir, err := inst.VFS().DirByID(parentID); err == nil {
		description = parentDir.DocName
	}

	// Send the notification (async, don't block on errors)
	go func() {
		if err := sendFileChangeNotification(inst, description, fileName, fileID, parentID, isFolder); err != nil {
			inst.Logger().WithNamespace("sharing").
				Warnf("Failed to send share-by-link file change notification: %s", err)
		}
	}()
}
