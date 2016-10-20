Replication
=========

Replication is the ability of a cozy-stack to copy / move all or a subset of its data to another support. It should cover 4 use cases

- Moving : The one-time act of moving a cozy from one hosting provider to another.
- Devices : The continuous act of syncing change to a subset of the cozy documents and files to and from an user cozy to the same user's Devices through cozy-desktop and cozy-mobile
- Sharing : The continuous act of syncing change to a subset of the cozy documents and files to and from another user's cozy.
- Backup : The regular but not continuous act of syncing the cozy data to


CouchDB replication is a well-understood, safe and stable algorithm ensuring replication between two couchdb databases.
Rsync is a well-understood, safe and stable algorithm to synchronize files hierarchy between two hosts.

**Moving** the cozy is similar to a one time **Backup**

# Files synchronization

Replication of too heavy database (with a lot of attachments) has been, in cozy experience, the source of some bugs. To avoid that (and some other issues), in cozy-stack, the attachments are stored outside of couchdb.

This means we need a way to synchronize files in parallel to couchdb replication.

This could be done with a rsync-like mechanism: Rsync essentially works by computing a signature of the files on target, sending this signature to source. On source, the file signature is used to compute a diff and then send only the changed chunks of the file to target. The chunks are stored on a tmp file on disk, so subsequent rsync does not need to resend the whole file.

The difficulty will be in maintaining synchronization of the couchdb & vfs state while synchronizing between hosts with two different replication algorithm. The MD5 stored in couchdb should help to ensure correctness. The already-planed *update-couchdb-to-vfs-state* function too.

We might need a zsync route too.

Some useful packages:
- Pure go rsync implementation : https://godoc.org/github.com/smtc/rsync
- Go bindings for librsync : https://github.com/silvasur/golibrsync
- A package to sync folder in FS or Swift: http://rclone.org/ (not rsync based, always whole file transfer)



# Couchdb replication & limitation

Couchdb 2 replication protocol is described [in details here ](http://docs.couchdb.org/en/2.0.0/replication/protocol.html)


## Quick summary

1. The replicator figures out what was the last sequence number *lastseqno* which was correctly synced from the source database, using a `_local/:replicationid` document.
2. The replicator `GET :source/changes?since=lastseqno&limit=batchsize` and identify which docs have changed (and their *open_revs*).
3. The replicator `POST :target/_revs_diff` to get a list of all revisions of the changed documents the target does not know.
4. The replicator `GET :source/:docid?open_revs=['rev1', ...]` several times to retrieve the missing documents revisions.
5. The replicator `POST :target/_bulk_docs` with these documents revisions.
6. Store the new last sequence number

Repeat 2-5 until there is no more changes.

## Details

- In step 5, the replicator can also attempt to `PUT :target/:docid` if doc are too heavy, but this should not happens in cozy-stack considering there wont be attachment in couchdb.
- The main difference from couchdb 1.X is in the replication history and the manner to determine and store the last sequence number. **TODO :** actually understand this and how it might increase disk usage if we have a lot of replications.
- Couchdb `_changes` and by extension replication can be either by polling `?since=xx&limit=xx` can return `[]`, or continuous (SSE / COMET)
- In couchdb benchmarking, we understood that the number of couch databases only use a bit of disk space but no RAM or CPU **as long as the database is not used**. Having a continuous replication active force us to keep the database file open and will starve RAM & FD usage. Moreover, continuous replication costs a lot (cf. ScienceTeam benchmark). To permit an unlimited number of inactive user on a single cozy-stack process, **the stack should avoid continuous replication from couchdb**.
- Two way replication is simply two one way replications


## Routes used by replication

To be a source of replication, the stack only need to support the following route:

- `GET :source/:docid?open_revs=xxxx` get revisions of a document
- `GET  :source/_changes` get a list of ID -> New Revision since a given sequence number

To be a target of replication, the stack need to support the following routes:

- `POST :target/_revs_diff` takes a list of ID -> Rev and returns which one are missing.
- `POST :target/_bulk_docs` create several documents at once
- `POST :target/_ensure_full_commit` ensure the documents are written to disk. This is useless if couchdb is configured without delayed write (default), but remote couchdb will call, so the stack should return the expected 201

In both case, we need to support

- `PUT    :both/_local/:revdocid` to store the current sequence number.

# Stack Sync API exploration


## Easy part: 1 db/doctype on stack AND remote, no binaries
We *just* need to implement the routes described above by proxying to the underlying couchdb database.

```javascript
var db = new PouchDB("my-local-contacts")
db.replicate.from('https://bob.cozycloud.cc/data/contacts')
db.replicate.to('https://bob.cozycloud.cc/data/contacts')
```

To suport this we need to :
- Proxy `_changes` route with since, limit, feed=normal. Refuse all filter parameters. [(Doc)](http://docs.couchdb.org/en/2.0.0/api/database/changes.html)
- Add support of `open_revs` query parameter to `GET /data/:doctype/:docid` [(Doc) ](http://docs.couchdb.org/en/2.0.0/api/document/common.html?highlight=open_revs#get--db-docid)
- Proxy the `_revs_diff` [(Doc)](http://docs.couchdb.org/en/2.0.0/api/database/misc.html#db-revs-diff) and `_bulk_docs` [(Doc)](http://docs.couchdb.org/en/2.0.0/api/database/bulk-api.html) routes
- Have `_ensure_full_commit` [(Doc)](http://docs.couchdb.org/en/2.0.0/api/database/compact.html#db-ensure-full-commit) returns 201

This will cover documents part of the Devices use case.

With a way to list doctype, this could also cover Backup and Moving.


## Continuous replication

It is impossible to implement it by simply proxying to couchdb (see unlimited inactive users). But neither the Device nor the Backup option need open continuous replication.

The only use case for which we might want it is sharing. Sharing has the advantage of being (at first) stack to stack. So we might imagine having the source of event "ping" the remote through a separate route.

**Conclusion** : no continuous replication except for a few special case (collaborative edit)


## Realtime

One of the cool feature of cozy apps was how changes are sent to the client in realtime through websocket. However this have a cost : every cozy and every apps open in a browser, even while not used keep one socket open on the server.

Several technology can be used for this : Websocket, SSE or COMET like long-polling. Goroutines makes all solution similar performances wise. COMET is a hack, websocket seems more popular and tested (x/net/websocket vs html5-sse-example). SSE has some limitations (can't set header...)

Depending on benchmarking, we can do some optimization on the feed :
- close feeds when the user is not on screen
- multiplex different applications' feed, so each open cozy will only use one socket to the server. And then use a [SharedWorker](https://developer.mozilla.org/en/docs/Web/API/SharedWorker) or postMessage to pass the events to each application javascript context (either iframe or browser tabs).


## Sharing

To be completed by discussing with Science team.

Current ideas (as understood by Romain)

- any filtered replication is unscalable
- 1 db for all shared docs `sharing_db`.
- Cozy-stack is responsible for saving documents that should be shared in both their doctype database and the sharing database
- Sharing implemented by 2-way replication between the 2 users `sharing_db`, filtering is done by computing a list of IDs and then `doc_ids` (sharing with filters/views are not efficient)
- Sharing is performed by batches regularly.
- Continuous replication can be considered for collaborative editing.

Proposal by Romain, if we find `_selector` filter replication performances to be acceptable on very large / very old databases.

- No sharing database
- A permission, for anything is a mango-style selector.
- on every query, the Mango selector is checked at the stack or couchdb level ($and-ing for queries, testing output document, input document)
- Sharing is a filtered replication between user's 1 doctypedb et user's 2 samedoctypedb
- No continuous replication
- Upon update, the stack trigger a PUSH replication to its remote



**TODO** experiment with performance of `_selector` filtered replication in couchdb2
