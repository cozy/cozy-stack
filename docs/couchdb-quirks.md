# CouchDB Quirks

## Mango indexes

### Exists operator

The
[`$exists` operator](http://docs.couchdb.org/en/stable/api/database/find.html#condition-operators)
can be used with a mango index for the `true` value, but not for the `false`
value. For `false`, a more heavy solution is required:
[a partial index](http://docs.couchdb.org/en/stable/api/database/find.html#find-partial-indexes).

### Index selection

CouchDB may accept or refuse to use a mango index for a query, with obsure
reasons. In general, you can follow these two rules of thumb:

1. An index on the fields `foo, bar, baz` can be used only to fetch documents
   where `foo`, `bar`, and `baz` exist. It means that a query that filters only
   on the value on `foo` won't use the mango index, because it can miss a
   document where `foo` has the expected value but without `bar` or `baz`. If
   you know that all the documents that you want have the `bar` and `baz`
   fields, you can just add two filters `$exists: true` (one for `bar`, the
   other for `baz`).

2. You should use exactly the same sequence of fields for creating the index and
   the `sort` operator of the query. If you have an index on `os, browser, ip`
   for the `io.cozy.sessions.logins`, and you want to have all the documents for
   a login from `windows`, sorted by `browser`, you can use the index, but you
   should use `os, browser, ip` for the sort (or at least `os, browser`, even if
   it is seems to weird to sort on `os` when all the sorted documents will have
   the same value, `windows`). Please note that using `use_index` on a request,
   the results will be sorted by default according to this rule. So, you can
   omit the `sort` operator on the query (except if you want the `descending`
   order).

## Old revisions

CouchDB keeps for each document a list of its revision (or more exactly a tree
with replication and conflicts).

It's possible to ask the list of the old revisions of a document with
[`GET /db/{docid}?revs_info=true`](http://docs.couchdb.org/en/stable/api/document/common.html#get--db-docid).
It works only if the document has not been deleted. For a deleted document,
[a trick](https://stackoverflow.com/questions/10854883/retrieve-just-deleted-document/10857330#10857330)
is to query the changes feed to know the last revision of the document, and to
recreate the document from this revision.

With an old revision, it's possible to get the content of the document at this
revision with `GET /db/{docid}?rev={rev}` if the database was not compacted. On
CouchDB 2.x, compacts happen automatically on all databases from times to times.

A `purge` operation consists to remove the tombstone for the deleted documents.
It is a manual operation, triggered by a
[`POST /db/_purge`](http://docs.couchdb.org/en/stable/api/database/misc.html).

## Conflicts

It is possible to create a conflict on CouchDB like it does for the replication
by using `new_edits`, but it is not well documented to say the least. The more
accurate description is on the old wiki:
https://wiki.apache.org/couchdb/HTTP_Bulk_Document_API#Posting_Existing_Revisions.

In short, it's a `PUT /doc/{id}?new_edits=false` with `_rev` the new revision of
the document, and `_revisions` the parents of this revision in the revisions
tree of this document.

## Design docs in \_all_docs

When querying `GET /{db}/_all_docs`, the response include the design docs. It's
quite difficult to filter them, particulary when pagination is involved. We have
added an endpoint `GET /data/:doctype/_normal_docs` to the stack to help client
side applications to deal with this.
