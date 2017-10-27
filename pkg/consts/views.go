package consts

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// IndexViewsVersion is the version of current definition of views & indexes.
// This number should be incremented when this file changes.
const IndexViewsVersion int = 12

// GlobalIndexes is the index list required on the global databases to run
// properly.
var GlobalIndexes = []*mango.Index{
	mango.IndexOnFields(Instances, "by-domain", []string{"domain"}),
}

// Indexes is the index list required by an instance to run properly.
var Indexes = []*mango.Index{
	// Permissions
	mango.IndexOnFields(Permissions, "by-source-and-type", []string{"source_id", "type"}),
	// Sharings
	mango.IndexOnFields(Sharings, "by-sharing-id", []string{"sharing_id"}),

	// Used to lookup over the children of a directory
	mango.IndexOnFields(Files, "dir-children", []string{"dir_id", "_id"}),
	// Used to lookup a directory given its path
	mango.IndexOnFields(Files, "dir-by-path", []string{"path"}),

	// Used to lookup a queued and running jobs
	mango.IndexOnFields(Jobs, "by-worker-and-state", []string{"worker", "state"}),
	mango.IndexOnFields(Jobs, "by-trigger-id", []string{"trigger_id"}),

	// Used to lookup oauth clients by name
	mango.IndexOnFields(OAuthClients, "by-client-name", []string{"client_name"}),

	// Used to looked login history by OS, browser, and IP
	mango.IndexOnFields(SessionsLogins, "by-os-browser-ip", []string{"os", "browser", "ip"}),
}

// DiskUsageView is the view used for computing the disk usage
var DiskUsageView = &couchdb.View{
	Name:    "disk-usage",
	Doctype: Files,
	Map: `
function(doc) {
  if (doc.type === 'file') {
    emit(doc._id, +doc.size);
  }
}
`,
	Reduce: "_sum",
}

// FilesReferencedByView is the view used for fetching files referenced by a
// given document
var FilesReferencedByView = &couchdb.View{
	Name:    "referenced-by",
	Doctype: Files,
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
var ReferencedBySortedByDatetimeView = &couchdb.View{
	Name:    "referenced-by-sorted-by-datetime",
	Doctype: Files,
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
var FilesByParentView = &couchdb.View{
	Name:    "by-parent-type-name",
	Doctype: Files,
	Map: `
function(doc) {
  emit([doc.dir_id, doc.type, doc.name])
}`,
	Reduce: "_count",
}

// PermissionsShareByCView is the view for fetching the permissions associated
// to a document via a token code.
var PermissionsShareByCView = &couchdb.View{
	Name:    "byToken",
	Doctype: Permissions,
	Map: `
function(doc) {
  if (doc.type === "share" && doc.codes) {
    Object.keys(doc.codes).forEach(function(k) {
      emit(doc.codes[k]);
    })
  }
}`,
}

// PermissionsShareByDocView is the view for fetching a list of permissions
// associated to a list of IDs.
var PermissionsShareByDocView = &couchdb.View{
	Name:    "byDoc",
	Doctype: Permissions,
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

// SharedWithPermissionsView returns the list of permissions associated with
// sharings.
var SharedWithPermissionsView = &couchdb.View{
	Name:    "sharedWithPermissions",
	Doctype: Sharings,
	Map: `
function(doc) {
  Object.keys(doc.permissions).forEach(function(k) {
    var rule = doc.permissions[k];
    for (var i=0; i<rule.values.length; i++) {
      emit([rule.type, doc.owner, doc.sharing_id], rule);
    }
  });
}`,
}

// SharingRecipientView is used to find a contact that is a sharing recipient,
// by its email or its cozy instance.
var SharingRecipientView = &couchdb.View{
	Name:    "sharingRecipient",
	Doctype: Contacts,
	Map: `
function(doc) {
  if (isArray(doc.email)) {
    for (var i = 0; i < doc.email.length; i++) {
      emit([doc.email[i].address, 'email']);
    }
  }
  if (isArray(doc.cozy)) {
    for (var i = 0; i < doc.cozy.length; i++) {
      emit([doc.cozy[i].url, 'cozy']);
    }
  }
}
`,
}

// TriggerLastJob indexes the last job launched by a specific trigger.
var TriggerLastJob = &couchdb.View{
	Name:    "triggerLastJob",
	Doctype: Jobs,
	Map: `
function(doc) {
  if (doc.trigger_id) {
    var state = doc.state;
    if (state == "done" || state == "errored") {
      emit([doc.worker, doc.trigger_id], state);
    }
  }
}
`,
}

// Views is the list of all views that are created by the stack.
var Views = []*couchdb.View{
	DiskUsageView,
	FilesReferencedByView,
	ReferencedBySortedByDatetimeView,
	FilesByParentView,
	PermissionsShareByCView,
	PermissionsShareByDocView,
	SharedWithPermissionsView,
	SharingRecipientView,
	TriggerLastJob,
}

// ViewsByDoctype returns the list of views for a specified doc type.
func ViewsByDoctype(doctype string) []*couchdb.View {
	var views []*couchdb.View
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
