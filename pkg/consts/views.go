package consts

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

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

	// Used to lookup a file given its parent, and the children of a directory
	mango.IndexOnFields(Files, "dir-file-child", []string{"dir_id", "name", "type"}),
	// Used to lookup over the children of a directory
	mango.IndexOnFields(Files, "dir-children", []string{"dir_id", "_id"}),
	// Used to lookup a directory given its path
	mango.IndexOnFields(Files, "dir-by-path", []string{"path"}),
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
	Map: `
function(doc) {
  if (doc.type === 'file' && isArray(doc.referenced_by)) {
    for (var i = 0; i < doc.referenced_by.length; i++) {
      emit([doc.referenced_by[i].type, doc.referenced_by[i].id]);
    }
  }
}`,
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
function(doc){
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

// Views is the list of all views that are created by the stack.
var Views = []*couchdb.View{
	DiskUsageView,
	FilesReferencedByView,
	PermissionsShareByCView,
	PermissionsShareByDocView,
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
