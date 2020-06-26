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

### Comparison of strings

Comparison of strings is done using ICU which implements the Unicode Collation
Algorithm, giving a dictionary sorting of keys. This can give surprising
results if you were expecting ASCII ordering. Note that:

- All symbols sort before numbers and letters (even the “high” symbols like tilde, `0x7e`)
- Differing sequences of letters are compared without regard to case, so `a < aa` but also `A < aa` and `a < AA`
- Identical sequences of letters are compared with regard to case, with lowercase before uppercase, so `a < A`.

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
by using `new_edits: false`, but it is not well documented to say the least. The
more accurate description was in the old wiki, that [no longer
exists](https://wiki.apache.org/couchdb/HTTP_Bulk_Document_API#Posting_Existing_Revisions).
Here is a copy of what it said:

> The replicator uses a special mode of \_bulk_docs. The documents it writes to
> the destination database already have revision IDs that need to be preserved for
> the two databases to be in sync (otherwise it would not be possible to tell that
> the two represent the same revision.) To prevent the database from assigning
> them new revision IDs, a "new_edits":false property is added to the JSON request
> body.

> Note that this changes the interpretation of the \_rev parameter in each
> document: rather than being the parent revision ID to be matched against, it's
> the existing revision ID that will be saved as-is into the database. And since
> it's important to retain revision history when adding to the database, each
> document body in this mode should have a \_revisions property that lists its
> revision history; the format of this property is described on the HTTP document
> API. For example:

> `curl -X POST -d '{"new_edits":false,"docs":[{"_id":"person","_rev":"2-3595405","_revisions":{"start":2,"ids":["3595405","877727288"]},"name":"jim"}]}' "$OTHER_DB/_bulk_docs"`

> This command will replicate one of the revisions created above, into a
> separate database `OTHER_DB`. It will have the same revision ID as in `DB`,
> `2-3595405`, and it will be known to have a parent revision with ID
> `1-877727288`. (Even though `OTHER_DB` will not have the body of that revision,
> the history will help it detect conflicts in future replications.)

> As with \_all_or_nothing, this mode can create conflicts; in fact, this is
> where the conflicts created by replication come from.
> In short, it's a `PUT /doc/{id}?new_edits=false` with `_rev` the new revision of
> the document, and `_revisions` the parents of this revision in the revisions
> tree of this document.

### Conflict example

Here is an example of a CouchDB conflict.

Let's assume the following document with the revision history `[1-abc, 2-def]`
saved in database:

```
{
  "_id": foo,
  "_rev": 2-def,
  "bar": "tender",
  "_revisions": {
    "ids": [
      "def",
      "abc"
    ]
  }
}
```

The `_revisions` block is returned when passing `revs=true` to the query and
gives all the revision ids, which the revision part after the dash.
For instance, in `2-def`, `2` is called the "generation" and `def` the "id".

We update the document with a `POST /bulk_docs` query, with the following
content:

```
{
	"docs": [
		{
			"_id": "foo",
			"_rev": "3-ghi",
			"_revisions": { "start": 3, "ids": ["ghi", "xyz", "abc"] }
			,
			"bar": "racuda"
		}
	],
	"new_edits": false
}
```

This produces a conflict bewteen `2-def` and `2-xyz`: the former was first saved
in database, but we forced the latter to be a new child of `1-abc`. Hence, this
document will have two revisions branches: `1-abc, 2-def` and `1-abc, 2-xyz, 3-ghi`.

### Sharing

In the [sharing protocol](https://docs.cozy.io/en/cozy-stack/sharing-design/),
we implement this behaviour as we follow the CouchDB replication model. However,
we prevent CouchDB conflicts for files and directories: see [this
explanation](https://docs.cozy.io/en/cozy-stack/sharing-design/#couchdb-conflicts)

## Design docs in \_all_docs

When querying `GET /{db}/_all_docs`, the response include the design docs. It's
quite difficult to filter them, particulary when pagination is involved. We have
added an endpoint `GET /data/:doctype/_normal_docs` to the stack to help client
side applications to deal with this.
