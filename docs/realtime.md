[Table of contents](README.md#table-of-contents)

# Realtime

The stack offers a way for applications to be notified in real-time of what
happens on the server via a websocket connection.

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

Then messages are sent using json:

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

## AUTH

It must be the first command to be sent. The client gives its token with this
command, and the stack will use it to know which are the permissions of the app.

```
{"method": "AUTH", "payload": "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJhcHAiLCJpYXQiOjE0OTg4MTY1OTEsImlzcyI6ImNvenkudG9vbHM6ODA4MCIsInN1YiI6Im1pbmkifQ.eH9DhoHz7rg8gR7noAiKfeo8eL3Q_PzyuskO_x3T8Hlh9q_IV-4zqoGtjTiO7luD6_VcLboEU-6o3XBek84VTg"}
```

## SUBSCRIBE

A client can send a SUBSCRIBE request to be notified of changes. The payload is
a selector for the events it wishes to receive For now the only possible
selector is on type & optionally id.

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

## Synthetic types

The stack an inject some synthetic events for documents that are not persisted
in CouchDB like classical doctypes:

- [Initial sync for sharings](https://docs.cozy.io/en/cozy-stack/sharing/#real-time-via-websockets)
- [Thumbnails for files](https://docs.cozy.io/en/cozy-stack/files/#real-time-via-websockets)
- [Telepointers for notes](https://docs.cozy.io/en/cozy-stack/notes/#real-time-via-websockets)
