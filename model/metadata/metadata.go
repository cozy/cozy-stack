package metadata

import (
	"errors"
	"time"
)

const MetadataVersion = 1

// ErrSlugEmpty is returned when an UpdatedByApp entry is created with and empty
// slug
var ErrSlugEmpty = errors.New("Slug cannot be empty")

// UpdatedByAppEntry represents a modification made by an application to the
// document
type UpdatedByAppEntry struct {
	Slug    string    `json:"slug"`              // Slug of the app
	Date    time.Time `json:"date"`              // Date of the update
	Version string    `json:"version,omitempty"` // Version identifier of the app
}

// CozyMetaData holds all the metadata of a document
type CozyMetaData struct {
	// Name or identifier for the version of the schema used by this document
	DocTypeVersion int `json:"doctypeVersion"`
	// Version of the cozyMetadata
	MetadataVersion int `json:"metadataVersion"`
	// Creation date of the cozy document
	CreatedAt *time.Time `json:"createdAt"`
	// Slug of the app or konnector which created the document
	CreatedByApp string `json:"createdByApp"`
	// Version identifier of the app
	CreatedByAppVersion string `json:"createdByAppVersion,omitempty"`
	// Last modification date of the cozy document
	UpdatedAt *time.Time `json:"updatedAt"`
	// List of objects representing the applications which modified the cozy document
	UpdatedByApps []*UpdatedByAppEntry `json:"updatedByApps"`
	// Identifier of the account in io.cozy.accounts (for konnectors)
	SourceAccount string `json:"sourceAccount,omitempty"`
	// Import date (if any)
	ImportedAt *time.Time `json:"importedAt"`
	// Import source (if any)
	ImportedFrom string `json:"importedFrom"`
	// List of app identifier which use the doctype
	UsedByApps []string `json:"usedByApps"`
}

// New initializes a new CozyMetaData structure
func New() *CozyMetaData {
	now := time.Now()
	return &CozyMetaData{
		MetadataVersion: MetadataVersion,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
}

func (cm *CozyMetaData) Clone() CozyMetaData {
	cloned := *cm
	if cm.UpdatedAt != nil {
		tmp := *cm.UpdatedAt
		cloned.UpdatedAt = &tmp
	}
	if cm.CreatedAt != nil {
		tmp := *cm.CreatedAt
		cloned.CreatedAt = &tmp
	}
	if cm.ImportedAt != nil {
		tmp := *cm.ImportedAt
		cloned.ImportedAt = &tmp
	}
	cloned.UpdatedByApps = make([]*UpdatedByAppEntry, len(cm.UpdatedByApps))
	for idx, app := range cm.UpdatedByApps {
		cloned.UpdatedByApps[idx] = app
	}
	return cloned
}

// EnsureCreatedFields ensures that empty fields are filled, otherwise use
// the default metadata values during the creation process
func (cm *CozyMetaData) EnsureCreatedFields(defaultMetadata *CozyMetaData) {
	if cm.CreatedAt == nil {
		cm.CreatedAt = defaultMetadata.CreatedAt
	}
	if cm.UpdatedAt == nil {
		cm.UpdatedAt = defaultMetadata.UpdatedAt
	}
	if cm.CreatedByApp == "" {
		cm.CreatedByApp = defaultMetadata.CreatedByApp
	}
	if cm.DocTypeVersion == 0 {
		cm.DocTypeVersion = defaultMetadata.DocTypeVersion
	}
	if cm.MetadataVersion == 0 {
		cm.MetadataVersion = defaultMetadata.MetadataVersion
	}
	if cm.UpdatedByApps == nil {
		cm.UpdatedByApps = defaultMetadata.UpdatedByApps
	}
}

// Update updates the UpdatedAt timestamp
func (cm *CozyMetaData) Update() {
	now := time.Now()
	cm.UpdatedAt = &now
}

// UpdatedByApp updates an entry either by updating the struct if the
// slug/version already exists or by appending a new entry to the list
func (cm *CozyMetaData) UpdatedByApp(slug, version string) error {
	if slug == "" {
		return ErrSlugEmpty
	}

	date := time.Now()
	updated := &UpdatedByAppEntry{Slug: slug, Date: date, Version: version}
	for idx, entry := range cm.UpdatedByApps {
		if entry.Slug == slug {
			cm.UpdatedByApps[idx] = updated
			return nil
		}
	}

	// The entry has not been found, adding it
	cm.UpdatedByApps = append(cm.UpdatedByApps, updated)
	cm.UsedByApps = append(cm.UsedByApps, slug)
	cm.UpdatedAt = &date
	return nil
}
