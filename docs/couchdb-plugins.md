[Table of contents](README.md#table-of-contents)

# Couchdb plugins analysis

(original discussion is at https://github.com/cozy/cozy-stack/issues/9)

## Existing packages

### https://github.com/timjacobi/go-couchdb

-   last edit 2016-08
-   Apparently one of the firsts
-   Includes a http module, trying to figure out why, might simply be history
    (started in 2014)
-   fork of the "original" https://github.com/fjl/go-couchdb
-   brother of https://github.com/pokstad/go-couchdb which is used in this cool
    article : http://pokstad.com/2015/04/18/couchdb-and-go.html
-   Have tools to run as couchdbapp / couchdbdaemon (we wont use these)
-   Allow to pass a net/http.RoundTripper for fine transport tunning.
-   Pr merged with some fix for couchdb2, not sure if there is more issue

### https://github.com/zemirco/couchdb

-   last edit 2016-08
-   No functions for changes API
-   Have a special interface for document {GetID(), GetRev()}

### https://github.com/rhinoman/couchdb-go

-   last edit 2016-04
-   No functions for changes API
-   Have functions for managing users and roles
-   Have functions for proxying requests (downloads / uploads)
-   support json byte[] as well as interface{} for documents
-   Is clean and clear, but more complex because of more functions. We will need
    most those (users, proxy)

### https://github.com/dustin/go-couch

-   last edit 2016-08
-   Simplest

## Plan B - Make our own from net/http

If we go this way, it will be a good idea to look at the code of the other
packages for each function, some pitfalls could be avoided :

ex:

-   https://github.com/rhinoman/couchdb-go/blob/94e6ab663d5789615eb061b52ed2e67310bac13f/connection.go#L81
-   Use custom Director & httputil.ReverseProxy for file download / upload

## Considerations on how we will use it

(see the [architecture](architecture.md) for more info)

-   One stack instance communicate with one couchdb 2.x server or cluster. The
    couchdb server/cluster has a lot of databases (nb_users x nb_data_types).
    The pair stack+couch should handle as many active users as possible. Number
    of unactive users must not impact perfs too much. Note : definition of
    active user TBD (mobile app and 3rd party software are syncing using \*dav
    and some background jobs like fetching banks accounts and threshold alerts )
-   HTTPS between stack and couchdb is not a priority. If it's an option, it
    will allow more flexibility for ops team, but we can always find alternative
    solution to secure channel between stack and couchdb and self-hosted users
    will probably have both on an single machine.
-   We wont keep changes feed open, as it was identified in couchdb workshop as
    the limiting factor on number of databases we can have and we want to have a
    bazillion databases. We will probably do some kind of polling, this will
    need to be investigated deeper.
-   Binaries will NOT be stored in couchdb. We will only store in couchdb
    reference to the file in another storage solution (FS / Swift)
-   The stack will use couchdb in 2 ways :
    1. Just proxying to/from the client, we will still need to parse the JSON,
       but will only read or changes a few special fields (`_id`, `_type`,
       `_tags` ....) for ACL and then pipe it to/from couchdb. We wont need to
       make sense of the whole document and can eventually get away without
       parsing it on some routes. This part should be totally flexible on the
       json content.
    2. Some APIs and Jobs will need to make sense of the given documents, so
       (un)marshal them from/into smarter struct (CalendarEvent) to be used in
       business logic.

## Analysis

-   They are all relatively similar in term of API. rhinoman has some more
    functions that we will need but lack changes feed.
-   They all have a struct for database which hold configuration, as most of our
    request will be on different databases, we will have a great churn for these
    structure. This is less than optimal for RAM & GC, but probably
    insignificant against JSON parsing.
-   They all use JSON Marshal et encode to accept any interface{} as couchdb
    doc.
-   They are all relatively inactive / stable, not sure if it is because they
    are "finished" or abandonware.
-   No library has a notion of pooling connection. This is handled at the
    net/http.Transport level in golang.
-   Only the first library has option for fine-tunning of the internal
    Client/Transport,
-   No library support couchdb2 mango

## Going further (thanks @tomquest)

Another advantage of a DIY solution: building exactly what `stack` needs. This
is orienting the CouchDB access code for Cozy usage. And this could be
implemented as a **DSL**.

For example: _(if I understand the architecture, the goal is for an User to have
many Databases, one for each DocumentType)_

```go
// Create an Email
couchdb
    .Doc(anEmail) // deduce the database from the type of the parameters
    .Create()
    // .Update()
    // .Delete()
```

```go
// List emails from 100 to 200
emails = couchdb
    .emails() // pick the appropriate database
    .Offset(100)
    .Limit(200)
    .Order(DATE, DESC)
    .select() // Create the adhoc CouchDb query (`Mango` style ?)
```

BTW, `couchdb` could be an already configured object with the appropriate user
credentials.

Pros:

-   Cozy-stack oriented (functionally)
-   Still able to access CouchDb natively (eg. `couchdb.Connection()`)
-   Can hide technical stuff: access to the pool, concurrency, caching, https,
    retries, timeouts...
-   Build and maintained by Cozy

Cons:

-   DSL to write (and testing is a bit harder)
-   To be maintained, but in fact, this is already the case for the other
    wrappers

## Current decision

We will make our own driver by cherry picking relevant codes in other libraries.
