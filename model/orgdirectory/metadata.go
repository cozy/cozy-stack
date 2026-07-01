package orgdirectory

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

const (
	DirectoryMetadataKey = "twakeDirectory"
	metadataKindGroup    = "group"
	metadataKindContact  = "contact"
)

// IsManagedDirectoryDoctype reports whether a doctype can contain managed
// organization directory documents.
func IsManagedDirectoryDoctype(doctype string) bool {
	return doctype == consts.Contacts || doctype == consts.Groups
}

// IsManagedDirectoryDoc reports whether a contact or group document is managed
// by the B2B organization directory replication.
func IsManagedDirectoryDoc(doc *couchdb.JSONDoc) bool {
	meta := directoryMetadata(doc)
	managed, _ := meta["managed"].(bool)
	return managed
}

func directoryMetadata(doc *couchdb.JSONDoc) map[string]interface{} {
	if doc == nil || doc.M == nil {
		return nil
	}
	meta, _ := doc.M[DirectoryMetadataKey].(map[string]interface{})
	return meta
}

func setGroupDirectoryMetadata(doc *couchdb.JSONDoc, organizationID, externalID string) {
	if doc.M == nil {
		doc.M = make(map[string]interface{})
	}
	doc.M[DirectoryMetadataKey] = map[string]interface{}{
		"managed":        true,
		"kind":           metadataKindGroup,
		"organizationId": organizationID,
		"externalId":     externalID,
	}
}

func setContactDirectoryMetadata(doc *couchdb.JSONDoc, input contactFields, email string) {
	if doc.M == nil {
		doc.M = make(map[string]interface{})
	}
	doc.M[DirectoryMetadataKey] = map[string]interface{}{
		"managed":        true,
		"kind":           metadataKindContact,
		"organizationId": input.OrganizationID,
		"username":       input.Username,
		"email":          email,
		"workplaceFqdn":  input.WorkplaceFQDN,
	}
}
