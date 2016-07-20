Cozy Cloud
==========

What is Cozy Cloud?
-------------------

1. A place to keep your personnal data
2. A core API to handle the data
3. Your web apps, and also the mobile & desktop clients
4. A coherent User Experience


Overview
--------

![Architecture for a self-hosted](self-hosted.png)

![Architecture for a big instance](big-instance.png)


**TODO**

- explain the many couchdb databases
- List konnectors / jobs
- say a word on metrics
- explain auth for users + apps + context
- explain permissions
- how to add a cozy instance to a farm
- context for sharing a photos album
- security, performances, help for developers
- migration from current
- import/export data ("you will stay because you can leave")
- Rest best pratices (jsonapi)
- ifttt / webhooks

----

Golang, with Gin framework
A single executable
Stateless
Elastic
- can be self-hosted
- farms of 1000 - 10000 cozys
CouchDB for data
Open Stack for files
A jobs service (scheduler + queues)
Apps that run in the browser


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

### Data `/data`

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

### Settings `/settings`

Each cozy instance has some settings, like its domain name, its language, the
name of its owner, the background for the home, etc.

### Notifications `/notifications`

The applications can put some notifications for the user. That goes from a
reminder for a meeting in 10 minutes to a suggestion to update your app.


Serverless apps
---------------

### Home
### App Center
### Activity Monitor
### My Accounts
### Preferences (+ theme)
### Devtools
### Contacts
### Calendar
### Emails
### Files
### Photos
### Todo list
### Onboarding

