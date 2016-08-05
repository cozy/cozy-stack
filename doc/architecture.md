Cozy Architecture
=================

What is Cozy?
-------------

Cozy is a personal platform as a service with a focus on data.
Cozy can be seen as 4 layers, from inside to outside:

1. A place to keep your personal data.
2. A core API to handle the data.
3. Your web apps, and also the mobile & desktop clients.
4. A coherent User Experience.

It's also a set of values: Simple, Versatile, Yours. These values mean a lot
for Cozy in all aspects. From an architectural point of view, it declines to:

- Simple to deploy and understand, not built as a galaxy of optimized
  microservices managed by kubernetes that only experts can debug.
- Versatile, can be hosted on a Raspberry Pi for geeks to massive scale on
  multiple servers by specialized hosting. Users can install apps.
- Yours, you own your data and you control it. If you want to take back your
  data to go elsewhere, you can.


Overview
--------

The architecture of Cozy is composed of:

- A reverse proxy.
- The cozy stack.
- A CouchDB instance to persist the JSON documents.
- A space for storing files.

All of this can run on a personal server, self-hosted at home, like a
Raspberry Pi:

![Architecture for a self-hosted](self-hosted.png)

But it's also possible to deploy a cozy on a more powerful server in order to
host dozens of cozy instances (an association for example). It will looks
like this:

![Architecture for a medium instance](middle-instance.png)

And even to scale to thousands of cozy instances on a server farm, with high
availability:

![Architecture for a big instance](big-instance.png)

This elasticity comes with some constraints:

- Most applications are run in the browser, not in the server.
- What must run on the server is mutualized inside the cozy stack.
- The cozy stack is stateless.
- The data is stored in couchdb and a space for files.
- A couchdb database is specific to an instance (no mix of data from 2 users
  in the same database).

### Reverse proxy

The reverse proxy is here to accept HTTPS connexions and forward the request
to the cozy stack. It's here mainly to manage the TLS part and binding a port
< 1024 without needing to launch the cozy stack as root.

### The Cozy Stack

The Cozy Stack is a single executable. It can do several things but its most
important usage is starting an HTTP server to serve as an API for all the
services of Cozy, from authentication to real-time events. This API can be
used on several domains. Each domain is a cozy instance for a specific user
("multi-tenant").

### Redis

Redis is optional when there is a single cozy stack running. When available,
it is used to synchronize the Cozy Stacks: distributed locks for special
operations like installing an application, queues for recurrent jobs, etc. As
a bonus, it can also be used to cache some frequently used documents.

### Databases

The JSON documents that represent the users data are stored in CouchDB, but
they are not mixed in a single database. We don't mix data from 2 users in the
same database. It's easier and safer to control the access to the data by
using different databases per user.

But we think to go even further by splitting the data of a user in several
databases, one per document type. For example, we can have a database for the
emails of a user and one for her todo list. This can simplify the
implementation of permissions (this app has access to these document types)
and can improve performance. CouchDB queries work with views. A view is
defined ahead of its usage and is built by CouchDB when it is requested and is
stale, i.e. there were writes in the database since the last time it was
updated. So, with a single database per user, it's possible to experience lag
when the todolist view is requested after fetching a long list of emails. By
splitting the databases per doctypes, we gain on two fronts:

1. The views are updated less frequently, only when documents of the matching
doctypes are written.
2. Some views are no longer necessary: those to access documents of a specific
doctypes.

There are downsides, mostly:

1. It can be harder to manage more databases.
2. We don't really know how well CouchDB will perform with so many databases.
3. It's no longer possible to use a single view for documents from doctypes.
that are no longer in the same database.

We think that we can work on that and the pros will outweight the cons.

### Metrics

The Cozy Stack can generate some metrics about its usage (the size of the
files transfered, the number of opened connexions, the number of requests to
redis, etc.) and export them to a metrics backend. It will help identify the
bottlenecks when scaling to add more users.


Services
--------

The cozy stack came with several services. They run on the server, inside the
golang process and have an HTTP interface.

### Authentication `/auth`

The cozy stack can authenticate the owner of a cozy instance. This can happen
in the classical web style, with a form and a cookie, but also with OAuth2 for
remote interactions like cozy-mobile and cozy-desktop.

### Applications `/apps`

It's possible to manage serverless applications from the cozy stack and serve
them via cozy stack. The stack does the routing and serve the HTML and the
assets for the applications.

The assets of the applications are installed in the virtual file system. On
the big instances, it means that even if it is the frontal 1 that installs the
application, frontal 2 will still be able to serve the application by getting
its assets from Swift.

**TODO** git or npm to install apps?
**TODO** 2 channels (stable / unstable)

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

- In a directory of a local file system (easier for self-hosted users).
- Swift from Open Stack (convenient for massive hosting).

The range of possible operations with this endpoint goes from simple ones,
like uploading a file, to more complex ones, like renaming a folder. It also
ensure that an instance is not exceeding its quota, and keeps a trash to
recover files recently deleted.

### Sharing `/sharing`

Users will want to share things like calendars. This service is there for
sharing JSON documents between cozy instances, with respect to the access
control.

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
name of its owner, the background for the home, etc. Also, the owner of the
cozy instance can choose a theme (a set of colors and a font) and this theme
will be available as a set of CSS variables in a stylesheet that can be
imported by the applications.

### Notifications `/notifications`

The applications can put some notifications for the user. That goes from a
reminder for a meeting in 10 minutes to a suggestion to update your app.

### Real-time `/real-time`

This endpoint can be used to subscribe for real-time events. An application
that shows items of a specific doctype can listen for this doctype to be
notified of all the changes for this doctype. For example, the calendar app
can listen for all the events and if a synchronization with the mobile adds a
new event, the app will be notified and can show this new event.

### Status `/status`

It's here just to say that the API is up and that it can access the CouchDB
databases, for debugging and monitoring purposes.


Workers
-------

The workers take jobs from the queues and process them.

### Fetch emails `/jobs/mailbox`

It fetches a mailbox to synchronize it and see if there are some new emails.

Payload: the mailbox

### Send email `/jobs/sendmail`

It connects to the SMTP server to send an email.

Payload: mail account, recipient, body, attachments

### Extract metadata `/jobs/metadata`

When a file is added or updated, this worker will extract its metadata (EXIF
for an image, id3 for a music, etc.)

Payload: the filepath

### Konnectors `/jobs/konnector`

It synchronizes an account on a remote service (fetch bills for example).

Payload: the kind of konnector, the credentials of the account and some
optional parameters (like the folder where to put the files)

### Registry `/jobs/registry`

It updates the list of available applications.

Payload: none

### Indexer `/jobs/indexer`

When a JSON document is added, updated or deleted, this worker will update the
index for full text search. [Bleve](http://www.blevesearch.com/) looks like a
good candidate for the indexing and full text search technology.

Payload: the doctype and the document to index


Serverless apps
---------------

### Home `/apps/home` (and aliased to `/` by default)

It's where you land on your cozy and launch your apps. Having widgets to
display informations would be nice too!

### App Center (was marketplace) `/apps/app-center`

You can install new apps here.

### Activity Monitor (was My apps) `/apps/activity-monitor`

It's a list of your installed apps and devices.

### My Accounts (was konnectors) `/apps/my-accounts`

You can configure new accounts, to fetch data from them, and see the already
configured accounts.

### Preferences `/apps/preferences`

You can set the settings of your cozy, choose a new background for the home,
and select a theme.

### Devtools `/apps/devtools`

Some tools for the developpers of applications only: an API console,
documentation, logs of the permission checks, etc.

### Contacts `/apps/contacts`

Manage your contact books.

### Calendar `/apps/calendar`

Manage your events and alarms.

### Emails `/apps/emails`

A webmail client to read, send and backup your emails.

### Files `/apps/files`

A web interface to browse your files.

### Photos `/apps/photos`

Organize your photos and share them with friends.

### Todo list `/apps/todo`

A task manager to never forgot what you should do.

### Onboarding `/apps/onboarding`

Start your cozy and setup your accounts.


Clients
-------

### Mobile

Cozy-mobile is an application for android and iOS for synchronizing files,
contacts and calendars between the phone and the cozy instance.


### Desktop

Cozy-desktop is a client for Linux, OSX and windows that allows to sync the
files in a cozy instance with a laptop or desktop.


Security
--------

### Access Control in the Cozy Stack

Authentication, authorizations and other things like that are simple for a
personal cloud, right? Well, not really. Let's see why.

First, authentication doesn't come only on the classical web flavour with a
login+password form and cookies. The login is not necessary, as the cozy
instance is already identified by its domain and has only one owner. But, more
than that, the authentication can also happen from a remote application, like
cozy-mobile or cozy-desktop. Oh, and 2 factor authentication (2FA) is
mandatory for something as valuable as personal data.

Then, authorizations are complicated. When the Cozy Stack receives a request
for accessing a JSON document, it has to check if it's authorized and this
control doesn't depend of only the user. The same document can be read in one
application but not in another. And even inside an application, there is a
notion of context. For example, in the photos application, the authenticated
owner of the cozy can see all the photos and share an album with some photos
to some of her friends. This album is a context and the cozy stack will allow
the access to the photos of this album, and only those.

**TODO** OAuth 2, permissions, intent, etc.

### Protection mechanisms for the client side applications

This is mostly applying the state of the art:

- Using HTTPS, with HSTS.
- Using secure, httpOnly,
  [sameSite](https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00)
  cookies to avoid cookies theft or misuse.
- Using a Content Security Policy (CSP).
- Using X-frame-options http header to protect against click-jacking.

But, it's more complicated than for a classical Single Page App. A cozy
instance on a domain have many SPAs, and these apps have different
permissions. Since they are in the same domain, separating them is not easy.
We have to forbid embeding in iframes and set a very strict Content Security
Policy. Even then, they share some ground, like localStorage, and we can't
block two applications to communicate between them.

That's why we want to have code review of the applications and a way to alert
of suspect behaviours via the marketplace.

**TODO** CSRF

### Encrypted data

Some data are encrypted before being saved in CouchDB (passwords for the
accounts for example). Encrypting everything has some downsides:

- It's not possible to index encryped documents or do computations on the
  encrypted fields in reasonable time
  ([homomorphic encryption](https://en.wikipedia.org/wiki/Homomorphic_encryption)
  is still an open subject).
- Having more encrypted data can globally weaken the encryption, if it's not handled properly.
- If the encryption key is lost or a bug happen, the data is lost with no way
  to recover them.

So, we are more confortable to encrypt only some fields. And later, when we
will have more experience and feedbacks from the user, extend the encryption
to more fields.

We are also working with [SMIS](https://project.inria.fr/smis/), a research lab, to find a way to securely store and backup the encryption keys.

### Be open to external contributors

Our code is Open Source, external contributors can review it. If they (you?)
find a weakness, please contact us by sending an email to security AT
cozycloud.cc. This is a mailing-list specially setup for responsible
disclosure of security weaknesses in Cozy. We will respond in less than
72 hours.

When a security flaw is found, the process is the following:

- Make a pull-request to fix (on our private git instance) and test it.
- Deploy the fix on cozycloud.cc
- Publish a new version, announce it on
  [the forum](https://forum.cozy.io/c/latest-information-about-cozy-security)
  as a security update and on the mailing-lists.
- 15 days later, add the details on the forum.


Guidelines
----------

### The Go Programming Language

Go (often referred as golang) is an open source programming language created
at Google in 2007. It has nice properties for our usage:

- Simplicity (the language can be learned in weeks, not years).
- A focus on productivity.
- Good performance.
- A good support of concurrency with channels and goroutines.

Moreover, Go is
[used by a lot of companies](https://github.com/golang/go/wiki/GoUsers),
is in [the Top 10 of the most used languages](http://spectrum.ieee.org/computing/software/the-2016-top-programming-languages)
and has some known open source projects: docker, kubernetes, grafana,
syncthing, influxdb, caddy, etc. And it works on
[the ARM platforms](https://github.com/golang/go/wiki/GoArm).

Go has some tools to help the developers to format its code (`go fmt`),
retrieve and install external packages (`go get`), display documentation
(`godoc`), check for potential errors with static analysis (`go vet`), etc.
Most of them can be used via
[gometalinter](https://github.com/alecthomas/gometalinter), which is nice for
continuous integration.

So, we think that writing the Cozy Stack in Go is the right choice.

### Repository organisation

```
├── assets          The assets for the front-end
│   ├── fonts       The webfonts
│   ├── images      The images
│   ├── scripts     The javascript files
│   └── styles      The CSS files
├── cmd             One .go file for each command of the cozy executable
├── doc             Documentation, including this file
└── web             One sub-directory for each of the services listed above
```

### Rest API

We follow the best practices about Rest API (using the right status codes,
HTTP verbs, organise code by resources, use content-negociation, etc.). When
known standards make sense (caldav & carddav for example), use them. Else,
[JSON API](http://jsonapi.org) is a good default.

The golang web framework used for the cozy stack is
[Gin](https://gin-gonic.github.io/gin/).

All the HTTP resources will be documented with
[swagger-ui](https://github.com/swagger-api/swagger-ui).

### DocTypes

Each JSON document saved in CouchDB has a field `docType` that identify the
kind of thing it is. For example, a contact will have the docType
`github.com/cozy/cozy-doctypes/contact`, and in the cozy-doctypes repository,
there will be a contact folder with a JSON inside it that describes this
doctype (a bit like the golang imports):

- What are the mandatory and optional fields?
- What is the type (string, integer, date) of the fields?
- Is there a validation rule for a field?
- How the fields can be indexed for full text search?
- What is the role of each field (documentation)?

This description can be used by any cozy client library (JS, Golang, etc.) to
generate some models to simplify the use of documents of this doctype.

### Import and export

> You will stay because you can leave.

An important promise of Cozy is to give back to the users the control of
their data. And this promise is not complete with a way to export the data to
use it somewhere else.

The Cozy Stack will offer an export button that gives a tarball to the user
with the full data. She can then import it on another instance for example. It
should also be possible to use the data outside of Cozy. So, the format for
the tarball should be as simple as possible and be documented.

### How to contribute?

Cozy's DNA is fundamentally Open Source and we want our community to thrive.
Having contributions (code, design, translations) is important for us and we
will try to create the favorable conditions to support it.

#### Adding a new konnector

Adding a konnector is easy for someone who knows JavaScript. The repository
has already a lot of pull requests by external contributors. The wiki has
documentation to explain the first steps of creating a new konnector. 3 active
contributors have been promoted to the maintainers team and can merge the pull
requests. We have done workshops to help new developers code their first
konnector and we will keep doing it.

#### Creating a new application

One of the goals of the new architecture is to make it easier for developers
to write new apps. It means having a good documentation, but also some
devtools to help:

- The `cozy` executable will have a command to setup a new project.
- The devtools on the cozy interface will give documentation about the
  doctypes, help explore the Rest API, and check if the permissions are OK.
- `cozy-ui` will make it easy to reuse some widgets and offer an application
  with a style coherent to the cozy identity.

#### Reporting a bug or suggesting a new feature

We are listening to our users. The forum is here to discuss on many subjects,
including how the applications are used. The issues on github are a good place
for bug tracking.

#### Translating to a new language

We will keep having internationalization for our applications and the
translations are maintained on transifex by the community. Translating to a
new language, or reviewing an existing one, is really appreciated.


FAQ
---

> Does the current konnectors in nodejs will be lost?

No, they won't. The business logic to scrap data from the many sources will be
kept and they will be adapted to fit in this new architecture. It won't be a
daemonized http server anymore, just some node scripts. The Cozy Stack will
listen for jobs for them and, then, will launch these nodejs scripts with the
right parameters.

> So, it's not possible to have a custom application with a server part, like
the lounge IRC client?

We want to support this use case, just not on the short term. It's not clear
how we can do that (while not weakening the security). One idea is to run the
applications in a different server, or maybe in docker.

> How to install and update cozy?

The Cozy Stack will have no auto-update mechanism. For installing and updating
it, you can use the classical ways:

- Using a package manager, like apt for debian & ubuntu.
- Using an official image for Raspberry Pi (and other embedded platforms).
- Using the image and services of an hosting company.
- Or compiling and installing it manually if you are really brave ;-)

> How to add a cozy instance to a farm?

1. Choose a (sub-)domain and configure the DNS for this (sub-)domain.
2. Configure the reverse-proxy to accept this (sub-)domain.
3. Use the `cozy` executable to configure the cozy stack.

> How to migrate from the nodejs cozy to this new architecture for cozy?

1. Export the data from the nodejs cozy (we need to add a button in the web
interface for that in the coming months).
2. Install the new cozy.
3. Import the data.

Please note that we don't support a continuous replication method that will
enable to use both the nodejs and the new architecture at the same time. It
looks too complicated for a temporary thing.

> How to backup the data?

There are 2 sensitive places with data:

- In CouchDB.
- on the place used for the Virtual File System (a directory on the local
  filesystem, or in Swift).

You can use the tools of your choice to backup these 2 locations. The good old
rsync works fine (CouchDB files are append-only, except when compaction
happens, so it's friendly to rsync).

It's highly recommended to have an automated backup, but sometimes it can be
useful to have a way to backup manually the data. The "export data" button in
the web interface give a tarball that can be used to transfer your data from
one instance to another, and so, it can be used as a backup.

> Aren't microservices better for scaling?

Yes, it's often easier to scale by separating concerns, and microservices is a
way to achieve that. But, it has some serious downsides:

- It takes more memory and it's probably a no-go for Raspberry Pi.
- It's more complicated for a developper to install the whole stack before
  coding its application.
- It's harder to deploy in production.

For the scalability, we can also deploy some specialized instances of the Cozy
Stack. For example, we can have some Cozy Stack processes dedicated for
real-time. Even, if they have all the code, we can route only the relevant
trafic from the load-balancer.
