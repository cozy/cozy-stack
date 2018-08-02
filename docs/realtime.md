# Realtime docs

## Context

### Definitions

* **Event:** Something happening in the stack. Most of them will come from
  couchdb, some jobs and user actions might also trigger them.
* **Events feed:** the feed of occurring events. There is two types of feeds:
  * **continuous** allows to follow events as they occurs
  * **interval** allow the see the history of the feed from any given time
* **Realtime:** user experienced updates of the interface from change happening
  from another source. _Ie, I have a folder opened in cozy-files on the browser,
  I take some pictures from my smartphone, the pictures appears in the folder
  without me needing to refresh the browser tab._

### What couchdb offers

Couchdb supports with its `_changes` API both events feeds types:

* using `since=now&continuous=true` we get all events **continuous**ly as they
  happen (SSE)
* using `since=(last known seq_number)` we get all changes in the **interval**
  between last known `seq_number` and now.

Couchdb also offers a `_db_updates` route, which give us **continuous** changes
at the database level. This routes does not support a since parameter, as there
is no global `seq_number`.

Other events will be generated from the stack itself, such as session close or
jobs activity.

### Performance limitation

**We cannot have a continuous `_changes` feed open to every databases**

## Use cases for interval events feeds

### Replication

Couchdb replication algorithm can work in one-shot mode, where it replicates
changes since last sync up until now, or in continuous mode where it replicates
changes as they happens.

* The stack will not allow continuous mode for replication.
* This is already supported with the `_changes` route

### Sharing

Our sharing is based on couchdb replication rules, so also depends on `_changes`
feed to ensure all changes have been applied.

**Considering these use cases, there is no need for non-couchdb event to be part
of the interval events feed.**

## Use cases for continuous events feeds

### Realtime

Some events should be send to the client to update views as data change.

### `@event` jobs trigger

Some event will trigger the activation of a job (ie. When a photo has been
uploaded, generate thumbnails and extract EXIF metadatas). This should be done
as soon as possible after the events

### Sharing ?

While not absolutely necessary, having cozy A notify cozy B when a shared
document is changed allows for both better user experience (faster propagation)
and better performance (no need to poll every X minutes, the N cozy we are
sharing from).

## Client realtime tech choice

### Options

* **Polling:** regularly ask the server what happened since last time.
* **COMET:** Leaving a normal HTTP connection open sending data and heartbeets
  regularly to keep it open, reading xhr.responseText at intervals without
  waiting for readyState == 4. Restart the connection when it breaks.
* **SSE:** Normalized & standardized version of COMET with
  [half-decent browser support (86% users)](http://caniuse.com/#feat=eventsource)
  but easily polyfillable (it's just COMET). It is simpler and easier to debug.
  It has some limitations (no HTTP headers in JS api, counts toward the maximum
  number of http connection per domain).
* **Websocket:** keep a socket open, it allows 2 way data communication which we
  do not need, has
  [better server support (92% users)](http://caniuse.com/#feat=websockets) but
  is impossible to polyfill client side, more popular, there is a better
  [golang package](https://godoc.org/github.com/gorilla/websocket)
* **SockJS & cie** they are **a lot** of packages which imitate Websocket API
  while using complicated client&server polyfill to allow support of older
  browser. [SockJS](https://github.com/sockjs/) is a drop-in websocket
  replacement with a go package and javascript client.

### Choice = Websocket

While SSE appears at first glance like a better fit for our use case, its
limitation and lack of browser priority makes us choose websocket. In the event
older browser supports becomes necessary we can use SockJS.

### optimization paths (future)

* **bandwidth** Limiting the number of events sent by allowing the client to
  specified it is only interested in events matching a selector _(files app only
  care about changes in the files of the current folder view)_
* **number of connections** Instead of 1 socket / tab, we can probably make 1
  socket / browser using some hackish combination of SharedWorker /
  iframe.postMessage and a client-side demultiplexer.
* **both** No need for realtime if the user is not using the tab (for most
  usecases), we could cut the realtime feed depending on
  [Page Visibility API](https://www.w3.org/TR/2011/WD-page-visibility-20110602/)

## Go/Stack architecture

* We assume all couchdb changes will originate from the stack
* Events are generated at the stack level
* We do **NOT** rely on couchdb `_changes?continuous` nor `_db_udpates`

We create a realtime.Event interface, which we call in other packages. We accept
websocket connection and bind them to a realtime.Dispatcher object.

### Small cozy version

It all happens in RAM, realtime.Event are immediately transmited to the
dispatcher.

### Big cozy version (ie. multiple stack instance)

Redis pub/sub

## Websocket API

We start with a normal websocket handshake.

Websockets include a protocol description in handshake, the protocol described
below is hereby named `io.cozy.websocket`.

Changes to the websocket protocol should be given versions, support for older
version should be maintained when reasonable.

```http
GET /realtime/ HTTP/1.1
Host: mycozy.example.com
Upgrade: websocket
Connection: Upgrade
Origin: http://calendar.mycozy.example.com
Sec-WebSocket-Key: x3JrandomLkh9GBhXDw==
Sec-WebSocket-Protocol: io.cozy.websocket
Sec-WebSocket-Version: 13
```

Then messages are sent using json

```
client > {"method": "AUTH",
          "payload": "xxAppOrAuthTokenxx="}
client > {"method": "SUBSCRIBE",
          "payload": {"type": "io.cozy.files"}}
client > {"method": "SUBSCRIBE",
          "payload": {"type": "io.cozy.contacts"}}
server > {"event": "UPDATED",
          "payload": {"id": "idA", "rev": "2-705...", "type": "io.cozy.contacts", "doc": {embeded doc ...}}}
server > {"event": "DELETED",
          "payload": {"id": "idA", "rev": "3-541...", "type": "io.cozy.contacts"}}
server > {"event": "UPDATED",
          "payload": {"id": "idB", "rev": "6-457...", "type": "io.cozy.files", "doc": {embeded doc ...}}}
```

### AUTH

It must be the first command to be sent. The client gives its token with this
command, and the stack will use it to know which are the permissions of the app.

```
{"method": "AUTH", "payload": "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJhcHAiLCJpYXQiOjE0OTg4MTY1OTEsImlzcyI6ImNvenkudG9vbHM6ODA4MCIsInN1YiI6Im1pbmkifQ.eH9DhoHz7rg8gR7noAiKfeo8eL3Q_PzyuskO_x3T8Hlh9q_IV-4zqoGtjTiO7luD6_VcLboEU-6o3XBek84VTg"}
```

### SUBSCRIBE

A client can send a SUBSCRIBE request to be notified of changes. The payload is
a selector for the events it wishes to receive For now the only possible
selector is on type & optionaly id

```
{"method": "SUBSCRIBE", "payload": {"type": "[desired doctype]"}}
{"method": "SUBSCRIBE", "payload": {"type": "[desired doctype]", "id": "idA"}}
```

In order to subscribe, a client must have permission `GET` on the passed
selector. Otherwise an error is passed in the message feed.

```
server > {"event": "error",
          "payload": {
            "status": "403 Forbidden"
            "code": "forbidden"
            "title":"The Application can't subscribe to io.cozy.files"
            "source": {"method": "SUBSCRIBE", "payload": {"type":"io.cozy.files"} }
          }}
```
