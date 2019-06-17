package vfs

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/metadata"
)

// DocTypeVersion represents the doctype version. Each time this document
// structure is modified, update this value
const DocTypeVersion = "1"

// UploadedByEntry is a struct with information on the app that has done the
// last upload of a file.
type UploadedByEntry struct {
	Slug    string            `json:"slug,omitempty"`
	Version string            `json:"version,omitempty"`
	Client  map[string]string `json:"oauthClient,omitempty"`
}

// FilesCozyMetadata is an extended version of cozyMetadata with some specific fields.
type FilesCozyMetadata struct {
	metadata.CozyMetadata
	// Instance URL where the file has been created
	CreatedOn string `json:"createdOn,omitempty"`
	// Date of the last upload of a new content
	UploadedAt *time.Time `json:"uploadedAt,omitempty"`
	// Information about the last time the content was uploaded
	UploadedBy *UploadedByEntry `json:"uploadedBy,omitempty"`
	// Instance URL where the content has been changed the last time
	UploadedOn string `json:"uploadedOn,omitempty"`
	// Identifier of the account in io.cozy.accounts (for konnectors)
	SourceAccount string `json:"sourceAccount,omitempty"`
	// Identifier unique to the account targeted by the connector (login most of the time)
	SourceIdentifier string `json:"sourceAccountIdentifier,omitempty"`
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
	if fcm.UploadedBy != nil {
		client := make(map[string]string)
		for k, v := range fcm.UploadedBy.Client {
			client[k] = v
		}
		cloned.UploadedBy = &UploadedByEntry{
			Slug:    fcm.UploadedBy.Slug,
			Version: fcm.UploadedBy.Version,
			Client:  client,
		}
	}
	if fcm.UploadedAt != nil {
		at := *fcm.UploadedAt
		cloned.UploadedAt = &at
	}
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

	if fcm.UploadedAt != nil {
		doc["uploadedAt"] = *fcm.UploadedAt
	}
	if fcm.UploadedBy != nil {
		uploaded := make(map[string]interface{})
		if fcm.UploadedBy.Slug != "" {
			uploaded["slug"] = fcm.UploadedBy.Slug
		}
		if fcm.UploadedBy.Version != "" {
			uploaded["slug"] = fcm.UploadedBy.Version
		}
		if len(fcm.UploadedBy.Client) > 0 {
			uploaded["oauthClient"] = fcm.UploadedBy.Client
		}
		doc["uploadedBy"] = uploaded
	}
	if fcm.UploadedOn != "" {
		doc["uploadedOn"] = fcm.UploadedOn
	}
	if fcm.SourceAccount != "" {
		doc["sourceAccount"] = fcm.SourceAccount
	}
	if fcm.SourceIdentifier != "" {
		doc["sourceAccountIdentifier"] = fcm.SourceIdentifier
	}
	return doc
}
