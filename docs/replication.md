[Table of contents](README.md#table-of-contents)

# Replication

Replication is the ability of a cozy-stack to copy / move all or a subset of its
data to another support. It should cover 2 use cases

* Devices: The continuous act of syncing change to a subset of the cozy
  documents and files to and from an user cozy to the same user's Devices
  through cozy-desktop and cozy-mobile
* Sharing: The continuous act of syncing change to a subset of the cozy
  documents and files to and from another user's cozy.

Replication will not be used for Moving, nor Backup. See associated docs in this
folder.

CouchDB replication is a well-understood, safe and stable algorithm ensuring
replication between two couchdb databases. It is delta-based, and is stateful:
we sync changes since a given checkpoint.

## Files synchronization

Replication of too heavy database (with a lot of attachments) has been, in cozy
experience, the source of some bugs. To avoid that (and some other issues), in
cozy-stack, the attachments are stored outside of couchdb.

This means we need a way to synchronize files in parallel to couchdb
replication.

Rsync is a well-understood, safe and stable algorithm to replicate files
hierarchy from one hosts to the other by only transfering changes. It is
one-shot and stateless. It could be an inspiration if we have to focus on
syncing small changes to big files. The option to use rsync/zsync have been
discussed internally (https://github.com/cozy/cozy-stack/pull/57 & Sprint Start
2016-10-24), but for now we should focus on using the files/folders couchdb
document for synchronization purpose and upload/download the actual binaries
with existing files routes or considering webdav.

**Useful golang package:** http://rclone.org/ sync folder in FS / Swift (may be
we can add a driver for cozy)

## Couchdb replication & limitation

Couchdb 2 replication protocol is described
[in details here](http://docs.couchdb.org/en/stable/replication/protocol.html).

### Quick summary

1. The replicator figures out what was the last sequence number _lastseqno_
   which was correctly synced from the source database, using a
   `_local/:replicationid` document.
2. The replicator `GET :source/changes?since=lastseqno&limit=batchsize` and
   identify which docs have changed (and their _open_revs_).
3. The replicator `POST :target/_revs_diff` to get a list of all revisions of
   the changed documents the target does not know.
4. The replicator `GET :source/:docid?open_revs=['rev1', ...]` several times to
   retrieve the missing documents revisions.
5. The replicator `POST :target/_bulk_docs` with these documents revisions.
6. Store the new last sequence number

Repeat 2-5 until there is no more changes.

### Details

* In step 5, the replicator can also attempt to `PUT :target/:docid` if doc are
  too heavy, but this should not happens in cozy-stack considering there wont be
  attachment in couchdb.
* In step 4, the replicator can optimize by calling `GET`
* The main difference from couchdb 1.X is in the replication history and the
  manner to determine and store the last sequence number. **TODO:** actually
  understand this and how it might increase disk usage if we have a lot of
  replications.
* Couchdb `_changes` and by extension replication can be either by polling or
  continuous (SSE / COMET)
* In couchdb benchmarking, we understood that the number of couch databases only
  use a bit of disk space but no RAM or CPU **as long as the database is not
  used**. Having a continuous replication active force us to keep the database
  file open and will starve RAM & FD usage. Moreover, continuous replication
  costs a lot (cf. ScienceTeam benchmark). To permit an unlimited number of
  inactive user on a single cozy-stack process, **the stack should avoid
  continuous replication from couchdb**.
* Two way replication is simply two one way replications

### Routes used by replication

To be a source of replication, the stack only need to support the following
route (and query parameters):

* `GET :source/:docid` get revisions of a document. The query parameters
  `open_revs, revs, latest` is necessary for replication.
* `POST :source/_all_docs` is used by current version of cozy-mobile and pouchdb
  as an optimization to fetch several document's revision at once.
* `GET :source/_changes` get a list of ID -> New Revision since a given sequence
  number. The query parameters `since, limit` are necessary for replication.

To be a target of replication, the stack need to support the following routes:

* `POST :target/_revs_diff` takes a list of ID -> Rev and returns which one are
  missing.
* `POST :target/_bulk_docs` create several documents at once
* `POST :target/_ensure_full_commit` ensure the documents are written to disk.
  This is useless if couchdb is configured without delayed write (default), but
  remote couchdb will call, so the stack should return the expected 201

In both case, we need to support

* `PUT :both/_local/:revdocid` to store the current sequence number.

## Stack Sync API exploration

### Easy part: 1 db/doctype on stack AND remote, no binaries

We _just_ need to implement the routes described above by proxying to the
underlying couchdb database.

```javascript
var db = new PouchDB("my-local-contacts");
db.replicate.from("https://bob.cozycloud.cc/data/contacts");
db.replicate.to("https://bob.cozycloud.cc/data/contacts");
```

To suport this we need to:

* Proxy `/data/:doctype/_changes` route with since, limit, feed=normal. Refuse
  all filter parameters with a clear error message.
  [(Doc)](http://docs.couchdb.org/en/stable/api/database/changes.html)
* Add support of `open_revs`, `revs`, `latest` query parameter to `GET /data/:doctype/:docid`
  [(Doc) ](http://docs.couchdb.org/en/stable/api/document/common.html?highlight=open_revs#get--db-docid)
* Proxy the `/data/:doctype/_revs_diff`
  [(Doc)](http://docs.couchdb.org/en/stable/api/database/misc.html#db-revs-diff)
  and `/data/:doctype/_bulk_docs` routes
  [(Doc)](http://docs.couchdb.org/en/stable/api/database/bulk-api.html) routes
* Have `/data/:doctype/_ensure_full_commit`
  [(Doc)](http://docs.couchdb.org/en/stable/api/database/compact.html#db-ensure-full-,
  revs, latestcommit) returns 201

This will cover documents part of the Devices use case.

### Continuous replication

It is impossible to implement it by simply proxying to couchdb (see unlimited
inactive users).

The current version of cozy-desktop uses it. **TODISCUSS** It could be replaced
by 3-minutes polling without big losses in functionality, eventually with some
more triggers based on user activity.

The big use case for which we might want it is Sharing. But, because Sharing
will (first) be stack to stack, we might imagine having the source of event
"ping" the remote through a separate route.

**Conclusion:** Start with polling replication. Consider alternative
notifications mechanism when the usecase appears and eventually use time-limited
continuous replication for a few special case (collaborative edit).

### Realtime

One of the cool feature of cozy apps was how changes are sent to the client in
realtime through websocket. However this have a cost: every cozy and every apps
open in a browser, even while not used keep one socket open on the server, this
is not scalable to thousands of users.

Several technology can be used for realtime: Websocket, SSE or COMET like
long-polling. Goroutines makes all solution similar performances wise. COMET is
a hack, websocket seems more popular and tested (x/net/websocket vs
html5-sse-example). SSE is not widely available and has some limitations
(headers...)

Depending on benchmarking, we can do some optimization on the feed:

* close feeds when the user is not on screen
* multiplex different applications' feed, so each open cozy will only use one
  socket to the server. This is hard, as all apps live on separate domain, an
  (hackish) option might be a iframe/SharedWorker bridge.

To have some form of couchdb-to-stack continuous changes monitoring, we can
monitor `_db_udpates`
[(Doc)](http://docs.couchdb.org/en/stable/api/server/common.html#db-updates)
which gives us an update when a couchdb has changed. We can then perform a
`_changes` query on this database to get the changed docs and proxy that to the
stack-to-client change feed.

**Conclusion:** We will use Websocket from the client to the stack. We will try
to avoid using continuous changes feed from couchdb to the stack. We will
optimize if proven needed by benchmarks, starting with "useless" changes and
eventually some multiplexing.

### Sharing

To be completed by discussing with Science team.

Current ideas (as understood by Romain)

* any filtered replication is unscalable
* 1 db for all shared docs `sharing_db`.
* Cozy-stack is responsible for saving documents that should be shared in both
  their dg implemented by 2-way replication between the 2 users `sharing_db`,
  filtering is done by computing a list of IDs and then `doc_ids` (sharing with
  filters/views are not efficient)
* Sharing is performed by batches regularly.
* Continuous replication can be considered for collaborative editing.

Proposal by Romain, if we find `_selector` filter replication performances to be
acceptable on very large / very old databases.

* No sharing database
* A permission, for anything is a mango-style selector.
* on every query, the Mango selector is checked at the stack or couchdb level
  (`$and`-ing for queries, testing output document, input document)
* Sharing is a filtered replication between user's 1 doctypedb et user's 2
  samedoctypedb
* No continuous replication
* Upon update, the stack trigger a PUSH replication to its remote or "ping" the
  remote, and the remote perform a normal PULL replication.

**TODO** experiment with performance of `_selector` filtered replication in
couchdb2
