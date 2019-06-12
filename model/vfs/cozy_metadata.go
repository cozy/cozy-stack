package vfs

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/metadata"
)

// DocTypeVersion represents the doctype version. Each time this document
// structure is modified, update this value
const DocTypeVersion = "1"

// FilesCozyMetadata is an extended version of cozyMetadata with some specific fields.
type FilesCozyMetadata struct {
	metadata.CozyMetadata
	// Instance URL where the file has been created
	CreatedOn string `json:"createdOn,omitempty"`
}

// NewCozyMetadata initializes a new FilesCozyMetadata struct
func NewCozyMetadata(instance string) *FilesCozyMetadata {
	now := time.Now()
	return &FilesCozyMetadata{
		CozyMetadata: metadata.CozyMetadata{
			DocTypeVersion:  DocTypeVersion,
			MetadataVersion: metadata.MetadataVersion,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		CreatedOn: instance,
	}
}

// Clone clones a FileCozyMetadata struct
func (fcm *FilesCozyMetadata) Clone() *FilesCozyMetadata {
	cloned := *fcm
	cloned.UpdatedByApps = make([]*metadata.UpdatedByAppEntry, len(fcm.UpdatedByApps))
	copy(cloned.UpdatedByApps, fcm.UpdatedByApps)
	return &cloned
}

// UpdatedByApp updates the list of UpdatedByApps entries with the new entry.
// It ensures that each entry has a unique slug+instance, and the new entry
// will be in the last position.
func (fcm *FilesCozyMetadata) UpdatedByApp(entry *metadata.UpdatedByAppEntry) {
	if entry.Slug == "" {
		return
	}

	updated := fcm.UpdatedByApps
	for _, app := range fcm.UpdatedByApps {
		if app.Slug == entry.Slug && app.Instance == entry.Instance {
			continue
		}
		updated = append(updated, app)
	}

	fcm.UpdatedByApps = append(updated, entry)
}

// ToJSONDoc returns a map with the cozyMetadata to be used inside a JSONDoc
// (typically for sendings sharing updates).
func (fcm *FilesCozyMetadata) ToJSONDoc() map[string]interface{} {
	doc := make(map[string]interface{})
	doc["doctypeVersion"] = fcm.DocTypeVersion
	doc["metadataVersion"] = fcm.MetadataVersion
	doc["createdAt"] = fcm.CreatedAt
	if fcm.CreatedByApp != "" {
		doc["createdByApp"] = fcm.CreatedByApp
	}
	if fcm.CreatedByAppVersion != "" {
		doc["createdByAppVersion"] = fcm.CreatedByAppVersion
	}
	if fcm.CreatedOn != "" {
		doc["createdOn"] = fcm.CreatedOn
	}

	doc["updatedAt"] = fcm.UpdatedAt
	if len(fcm.UpdatedByApps) > 0 {
		entries := make([]map[string]interface{}, len(fcm.UpdatedByApps))
		for i, entry := range fcm.UpdatedByApps {
			subdoc := map[string]interface{}{
				"slug": entry.Slug,
				"date": entry.Date,
			}
			if entry.Version != "" {
				subdoc["version"] = entry.Version
			}
			if entry.Instance != "" {
				subdoc["instance"] = entry.Instance
			}
			entries[i] = subdoc
		}
		doc["uploadedByApp"] = entries
	}

	return doc
}
