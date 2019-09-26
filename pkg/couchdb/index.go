package couchdb

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// IndexViewsVersion is the version of current definition of views & indexes.
// This number should be incremented when this file changes.
const IndexViewsVersion int = 24

// Indexes is the index list required by an instance to run properly.
var Indexes = []*mango.Index{
	// Permissions
	mango.IndexOnFields(consts.Permissions, "by-source-and-type", []string{"source_id", "type"}),

	// Used to lookup over the children of a directory
	mango.IndexOnFields(consts.Files, "dir-children", []string{"dir_id", "_id"}),
	// Used to lookup a directory given its path
	mango.IndexOnFields(consts.Files, "dir-by-path", []string{"path"}),

	// Used to lookup a queued and running jobs
	mango.IndexOnFields(consts.Jobs, "by-worker-and-state", []string{"worker", "state"}),
	mango.IndexOnFields(consts.Jobs, "by-trigger-id", []string{"trigger_id", "queued_at"}),
	mango.IndexOnFields(consts.Jobs, "by-queued-at", []string{"queued_at"}),

	// Used to lookup oauth clients by name
	mango.IndexOnFields(consts.OAuthClients, "by-client-name", []string{"client_name"}),
	mango.IndexOnFields(consts.OAuthClients, "by-notification-platform", []string{"notification_platform"}),

	// Used to lookup login history by OS, browser, and IP
	mango.IndexOnFields(consts.SessionsLogins, "by-os-browser-ip", []string{"os", "browser", "ip"}),

	// Used to lookup notifications by their source, ordered by their creation
	// date
	mango.IndexOnFields(consts.Notifications, "by-source-id", []string{"source_id", "created_at"}),

	// Used to lookup the bitwarden ciphers in a folder
	mango.IndexOnFields(consts.BitwardenCiphers, "by-folder-id", []string{"folder_id"}),
}

// DiskUsageView is the view used for computing the disk usage for files
var DiskUsageView = &View{
	Name:    "disk-usage",
	Doctype: consts.Files,
	Map: `
function(doc) {
  if (doc.type === 'file') {
    emit(doc._id, +doc.size);
  }
}
`,
	Reduce: "_sum",
}

// OldVersionsDiskUsageView is the view used for computing the disk usage for
// the old versions of file contents.
var OldVersionsDiskUsageView = &View{
	Name:    "old-versions-disk-usage",
	Doctype: consts.FilesVersions,
	Map: `
function(doc) {
  emit(doc._id, +doc.size);
}
`,
	Reduce: "_sum",
}

// FilesReferencedByView is the view used for fetching files referenced by a
// given document
var FilesReferencedByView = &View{
	Name:    "referenced-by",
	Doctype: consts.Files,
	Reduce:  "_count",
	Map: `
function(doc) {
  if (isArray(doc.referenced_by)) {
    for (var i = 0; i < doc.referenced_by.length; i++) {
      emit([doc.referenced_by[i].type, doc.referenced_by[i].id]);
    }
  }
}`,
}

// ReferencedBySortedByDatetimeView is the view used for fetching files referenced by a
// given document, sorted by the datetime
var ReferencedBySortedByDatetimeView = &View{
	Name:    "referenced-by-sorted-by-datetime",
	Doctype: consts.Files,
	Reduce:  "_count",
	Map: `
function(doc) {
  if (isArray(doc.referenced_by)) {
    for (var i = 0; i < doc.referenced_by.length; i++) {
      var datetime = (doc.metadata && doc.metadata.datetime) || '';
      emit([doc.referenced_by[i].type, doc.referenced_by[i].id, datetime]);
    }
  }
}`,
}

// FilesByParentView is the view used for fetching files referenced by a
// given document
var FilesByParentView = &View{
	Name:    "by-parent-type-name",
	Doctype: consts.Files,
	Map: `
function(doc) {
  emit([doc.dir_id, doc.type, doc.name])
}`,
	Reduce: "_count",
}

// PermissionsShareByCView is the view for fetching the permissions associated
// to a document via a token code.
var PermissionsShareByCView = &View{
	Name:    "byToken",
	Doctype: consts.Permissions,
	Map: `
function(doc) {
  if (doc.type && doc.type.slice(0, 5) === "share" && doc.codes) {
    Object.keys(doc.codes).forEach(function(k) {
      emit(doc.codes[k]);
    })
  }
}`,
}

// PermissionsShareByShortcodeView is the view for fetching the permissions associated
// to a document via a token code.
var PermissionsShareByShortcodeView = &View{
	Name:    "by-short-code",
	Doctype: consts.Permissions,
	Map: `
function(doc) {
	if(doc.shortcodes) {
		for(var idx in doc.shortcodes) {
			emit(doc.shortcodes[idx], idx);
		}
	}
}`,
}

// PermissionsShareByDocView is the view for fetching a list of permissions
// associated to a list of IDs.
var PermissionsShareByDocView = &View{
	Name:    "byDoc",
	Doctype: consts.Permissions,
	Map: `
function(doc) {
  if (doc.type === "share" && doc.permissions) {
    Object.keys(doc.permissions).forEach(function(k) {
      var p = doc.permissions[k];
      var selector = p.selector || "_id";
      for (var i=0; i<p.values.length; i++) {
        emit([p.type, selector, p.values[i]], p.verbs);
      }
    });
  }
}`,
}

// PermissionsByDoctype returns a list of permissions that have at least one
// rule for the given doctype.
var PermissionsByDoctype = &View{
	Name:    "permissions-by-doctype",
	Doctype: consts.Permissions,
	Map: `
function(doc) {
  if (doc.permissions) {
    Object.keys(doc.permissions).forEach(function(k) {
	  emit([doc.permissions[k].type, doc.type]);
	});
  }
}
`,
}

// SharedDocsBySharingID is the view for fetching a list of shared doctype/id
// associated with a sharingid
var SharedDocsBySharingID = &View{
	Name:    "shared-docs-by-sharingid",
	Doctype: consts.Shared,
	Map: `
function(doc) {
  if (doc.infos) {
    Object.keys(doc.infos).forEach(function(k) {
      emit(k, doc._id);
    });
  }
}`,
}

// SharingsByDocTypeView is the view for fetching a list of sharings
// associated with a doctype
var SharingsByDocTypeView = &View{
	Name:    "sharings-by-doctype",
	Doctype: consts.Sharings,
	Map: `
function(doc) {
	if (isArray(doc.rules)) {
		for (var i = 0; i < doc.rules.length; i++) {
			if (!doc.rules[i].local) {
				emit(doc.rules[i].doctype, doc._id);
			}
		}
	}
}`,
}

// ContactByEmail is used to find a contact by its email address
var ContactByEmail = &View{
	Name:    "contacts-by-email",
	Doctype: consts.Contacts,
	Map: `
function(doc) {
	if (isArray(doc.email)) {
		for (var i = 0; i < doc.email.length; i++) {
			emit(doc.email[i].address, doc._id);
		}
	}
}
`,
}

// Views is the list of all views that are created by the stack.
var Views = []*View{
	DiskUsageView,
	OldVersionsDiskUsageView,
	FilesReferencedByView,
	ReferencedBySortedByDatetimeView,
	FilesByParentView,
	PermissionsShareByCView,
	PermissionsShareByDocView,
	PermissionsByDoctype,
	PermissionsShareByShortcodeView,
	SharedDocsBySharingID,
	SharingsByDocTypeView,
	ContactByEmail,
}

// ViewsByDoctype returns the list of views for a specified doc type.
func ViewsByDoctype(doctype string) []*View {
	var views []*View
	for _, view := range Views {
		if view.Doctype == doctype {
			views = append(views, view)
		}
	}
	return views
}

// IndexesByDoctype returns the list of indexes for a specified doc type.
func IndexesByDoctype(doctype string) []*mango.Index {
	var indexes []*mango.Index
	for _, index := range Indexes {
		if index.Doctype == doctype {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// globalIndexes is the index list required on the global databases to run
// properly.
var globalIndexes = []*mango.Index{
	mango.IndexOnFields(consts.Exports, "by-domain", []string{"domain", "created_at"}),
}

// secretIndexes is the index list required on the secret databases to run
// properly
var secretIndexes = []*mango.Index{
	mango.IndexOnFields(consts.AccountTypes, "by-slug", []string{"slug"}),
}

// DomainAndAliasesView defines a view to fetch instances by domain and domain
// aliases.
var DomainAndAliasesView = &View{
	Name:    "domain-and-aliases",
	Doctype: consts.Instances,
	Map: `
function(doc) {
  emit(doc.domain);
  if (isArray(doc.domain_aliases)) {
    for (var i = 0; i < doc.domain_aliases.length; i++) {
      emit(doc.domain_aliases[i]);
    }
  }
}
`,
}

// globalViews is the list of all views that are created by the stack on the
// global databases.
var globalViews = []*View{
	DomainAndAliasesView,
}

// InitGlobalDB defines views and indexes on the global databases. It is called
// on every startup of the stack.
func InitGlobalDB() error {
	if err := DefineIndexes(GlobalSecretsDB, secretIndexes); err != nil {
		return err
	}
	if err := DefineIndexes(GlobalDB, globalIndexes); err != nil {
		return err
	}
	return DefineViews(GlobalDB, globalViews)
}
