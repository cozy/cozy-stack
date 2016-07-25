Cozy Cloud Architecture
=======================

What is Cozy Cloud?
-------------------

Cozy Cloud is a personal platform as a service with a focus on data.
Cozy Cloud can be seen as 4 layers, from inside to outside:

1. A place to keep your personnal data
2. A core API to handle the data
3. Your web apps, and also the mobile & desktop clients
4. A coherent User Experience

It's also a set of values: Simple, Versatile, Yours. These values mean a lot
for Cozy Cloud in all aspects. From an architectural point, it declines to:

- Simple to deploy and understand, not built as a galaxy of optimized
  microservices managed by kubernetes that only experts can debug.
- Versatile, can be hosted on a Raspberry Pi for geeks to massive scale on
  multiple servers by specialized hosting.
- Yours, you own your data and you control them. If you want to take back your
  data to go elsewhere, you can.


Overview
--------

The architecture of Cozy Cloud is composed of:

- a reverse proxy
- the cozy stack
- a couchdb instance to persist the JSON documents
- a space for storing files.

All of this can run on a personal server, self-hosted at home, like a
Raspberry Pi:

![Architecture for a self-hosted](self-hosted.png)

But it's also possible to deploy a farm of servers for hosting thousands of
cozy instances. It will looks like this:

![Architecture for a big instance](big-instance.png)

This elasticity comes with some constraints:

- most applications are run in the browser, not in the server
- what must run on the server is mutualized inside the cozy stack
- the cozy stack is stateless
- the data are stored in couchdb and a space for files
- a couchdb database is specific to an instance (no mix of data from 2 users
  in the same database).

### Reverse proxy

The reverse proxy is here to accept HTTPS connexions and forward the request
to the cozy stack. It's here mainly to manage the TLS part.

### The Cozy Stack

The Cozy Stack is a single executable. It can do several things but it's most
important usage is starting an HTTP server to serve as an API for all the
services of Cozy, from authentication to real-time events. This API can be
used on several domains. Each domain is a cozy instance for a specific user
("multi-tenant").

### Databases

The JSON documents that reprensent the users data are stored in CouchDB, but
they are not mixed in a single database. We don't mix data from 2 users in the
same database. It's easier and safer to control the access to the data by
using different databases per user.

But we think to go even farther by spliting the data of a user in several
databases, one per document type. For example, we can have a database for the
emails of a user and one for her todo list. This can simplify the
implementation of permissions (this app has access to these document types)
and can improve performances. CouchDB queries work with views. A view is
defined ahead of its usage and is built by couchdb when it is requested and is
stale, ie there were writes in the database since the last time it was
updated. So, with a single database per user, it's possible to experience lag
when the todolist view is requested after fetching a long list of emails. By
spliting the databases per doctypes, we gain on two fronts:

1. The views are updated less frequently, only when documents of the matching
doctypes are writen.
2. Some views are no longer necessary: those to access documents of a specific
doctypes.

There are downsides, mostly:

1. It can be harder to manage more databases
2. We don't really know how well CouchDB will perform with so many databases
3. It's no longer possible to use a single view for documents from doctypes
that are no longer in the same database.

We think that we can work on that and the pros will outweight the cons.

### TODO

- List konnectors / jobs
- say a word on metrics
- explain auth for users + apps + context
- explain permissions
- how to add a cozy instance to a farm
- context for sharing a photos album
- migration from current
- import/export data ("you will stay because you can leave")


Services
--------

The cozy stack came with several services. They run on the server, inside the
golang processus and have an HTTP interface.

### Authentication `/auth`

The cozy stack can authenticate the owner of a cozy instance. This can happen
in the classical web style, with a form and a cookie, but also with OAuth2 for
remote interactions like cozy-mobile and cozy-desktop.

### Applications `/apps`

It's possible to manage serverless applications from the cozy stack and serve
them via cozy stack. The stack does the routing and serve the HTML and the
assets for the applications.

### Data System `/data`

CouchDB is used for persistence of JSON documents. The data service is a layer
on top of it for routing the requests to the corresponding CouchDB database
and checking the permissions.

In particular, a serverless application can declare some contexts and access
data in those contexts even if it's not the owner of the cozy instance that
access it. For example, the owner of a cozy can create a photo album with a
selection of photos. This album can then be associated to a context to be
shared with friends of the owner. These friends can access the album and see
the photos, but not anonymous people.

### Virtual File System `/files`

It's possible to store files on the cozy, including binary ones like photos
and movies, thanks to the virtual file system. It's a facade, with several
implementations, depending on where the files are effectively stored:

- in a directory of a local file system (easier for self-hosted users)
- Swift from Open Stack (convenient for massive hosting)

The range of operations possible with this endpoint goes from simple ones,
like uploading a file, to more complex ones, like renaming a folder.

### Jobs `/jobs`

The cozy stack has queues where job descriptions can be put. For example, a
job can be to fetch the latest bills from a specific provider. These queues
can be consumed by external workers to complete the associated jobs.

We can imagine having a media worker that extract thumbnails from photos and
videos. It will fetch jobs from a media queue and each job description will
contain the path to the photo or video from which the thumbnail will be
extracted.

There is also a scheduler that acts like a crontab. It can add jobs at
recurrent time. For example, it can add a job for fetching github commits
every hour.

Later, we can dream about having more ways to create jobs (webhooks, on
document creation) and make them communicate. With a web interface on that, it
can become a simplified [_Ifttt_](https://ifttt.com/).

### Sync `/sync`

This endpoint will be for synchronizing your contacts and calendars by using
standard methods like caldav and carddav.

### Settings `/settings`

Each cozy instance has some settings, like its domain name, its language, the
name of its owner, the background for the home, etc.

### Notifications `/notifications`

The applications can put some notifications for the user. That goes from a
reminder for a meeting in 10 minutes to a suggestion to update your app.

### Real-time `/real-time`

This endpoint can be use to subscribe for real-time events. An application
that shows items of a specific doctype can listen for this doctype to be
notified of all the changes for this doctype. For example, the calendar app
can listen for all the events and if a synchronization with the mobile adds a
new event, the app will be notified and can show this new event.

### Status `/status`

It's here just to say that the API is up and that it can access the CouchDB
databases.


Serverless apps
---------------

### Home

It's where you land on your cozy and launch your apps.

### App Center (was marketplace)

You can install new apps here.

### Activity Monitor (was My apps)

It's a list of your installed apps and devices.

### My Accounts (was konnectors)

You can configure new accounts, to fetch data from them, and see the already
configured accounts.

### Preferences

You can set the settings of your cozy, choose a new background for the home,
and select a theme.

### Devtools

Some tools for the developpers of applications only: an API console,
documentation, logs of the permission checks, etc.

### Contacts

Manage your contact books.

### Calendar

Manage your events and alarms.

### Emails

A webmail client to read, send and backup your emails.

### Files

A web interface to browse your files.

### Photos

Organize your photos and share them with friends.

### Todo list

A task manager to never forgot what you should do.

### Onboarding

Start your cozy and setup your accounts.


Guidelines
----------

### Golang

Why?

- used by a lot of people -> https://github.com/golang/go/wiki/GoUsers
- some known open source projects: docker, kubernetes, grafana, syncthing,
  influxdb, caddy, etc.


### Gin framework

### Rest best pratices (jsonapi)

### Others

- security, backup, performances, help for developers
- [The 12-factor app](http://12factor.net/)
- quota
- streaming
- doctype with Romain
- indexer (bleve ?)

