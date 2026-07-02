package orgdirectory

import (
	"errors"
	"fmt"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const managedDocsPageSize = 1000

func findExternalContactByEmail(db prefixer.Prefixer, email string) (*contact.Contact, error) {
	matches, err := contact.FindAllByEmail(db, email)
	if errors.Is(err, contact.ErrNotFound) {
		return nil, contact.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return singleExternalContact(matches, "email "+email)
}

func findExternalContactByCozyURL(db prefixer.Prefixer, cozyURL string) (*contact.Contact, error) {
	var docs []*contact.Contact
	req := &couchdb.FindRequest{
		Selector: mango.Map{
			"cozy": map[string]interface{}{
				"$elemMatch": map[string]interface{}{
					"url": cozyURL,
				},
			},
		},
		Limit: 10,
	}
	err := couchdb.FindDocsUnoptimized(db, consts.Contacts, req, &docs)
	if couchdb.IsNoDatabaseError(err) || couchdb.IsNotFoundError(err) {
		return nil, contact.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, contact.ErrNotFound
	}
	return singleExternalContact(docs, "cozy URL "+cozyURL)
}

func singleExternalContact(matches []*contact.Contact, label string) (*contact.Contact, error) {
	var external []*contact.Contact
	for _, doc := range matches {
		if doc.IsExternal() || IsManagedDirectoryDoc(&doc.JSONDoc) {
			external = append(external, doc)
		}
	}
	if len(external) == 0 {
		return nil, contact.ErrNotFound
	}
	if len(external) > 1 {
		return nil, fmt.Errorf("multiple external contacts found for %s", label)
	}
	return external[0], nil
}

func listManagedJSONDocs(inst *instance.Instance, doctype, organizationID string) ([]*couchdb.JSONDoc, error) {
	docs, err := listManagedDocs[couchdb.JSONDoc](inst, doctype, organizationID)
	for _, doc := range docs {
		doc.Type = doctype
	}
	return docs, err
}

func listManagedDocs[T any](inst *instance.Instance, doctype, organizationID string) ([]*T, error) {
	var docs []*T
	var bookmark string
	for {
		var page []*T
		req := managedDocsRequest(organizationID, bookmark)
		res, err := couchdb.FindDocsUnoptimizedRaw(inst, doctype, req, &page)
		if couchdb.IsNoDatabaseError(err) || couchdb.IsNotFoundError(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, page...)

		nextBookmark := ""
		if res != nil {
			nextBookmark = res.Bookmark
		}
		if len(page) < managedDocsPageSize || nextBookmark == "" || nextBookmark == bookmark {
			return docs, nil
		}
		bookmark = nextBookmark
	}
}

func managedDocsRequest(organizationID, bookmark string) *couchdb.FindRequest {
	return &couchdb.FindRequest{
		Selector: mango.And(
			mango.Equal(DirectoryMetadataKey+".managed", true),
			mango.Equal(DirectoryMetadataKey+".organizationId", organizationID),
		),
		Limit:    managedDocsPageSize,
		Bookmark: bookmark,
	}
}
