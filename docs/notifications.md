[Table of contents](README.md#table-of-contents)

# Notifications

Inspiration:
https://developer.android.com/guide/topics/ui/notifiers/notifications.html

Notifications are couchdb documents with the doctype `io.cozy.notifications`.

Schema will be defined in cozy-doctypes, along the lines of

```
source: applicationID
reference: text // useful to "hide notif like this"
title: text
content: text
icon?: image
actions?: [{text, intent}]
```

All applications, services and konnectors can create notifications if they have
a permission on `io.cozy.notifications` with the `POST` verb.

* Notifications will appear in the cozy-bar.
* Cozy mobile app(s) may display notifications on mobile.
* Some notifications may also be transmitted by email, eventually as summaries
  over period of time.
* The settings/notifications app will have "notifications center" tab, allowing
  to silence notifications and pick which should be sent to mobile / mail

## Listing/getting notifications

You can use the `/data/io.cozy.notifications` routes to read the notifications.

## Creating a notification

### POST /notifications

#### Request

```http
POST /notifications HTTP/1.1
Host: alice.cozy.tools
Authorization: Bearer ...
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notifications",
    "attributes": {
      "reference": "new operations",
      "title": "5 new operations on your bank account",
      "content":
        "You have 5 new operations on your bank account:\n-2 debit operations\n-3 credit operations",
      "icon": "https://calendar.alice.cozy.tools/alert.png",
      "actions": [
        {
          "text": "Show these operations",
          "intent": { "action": "OPEN", "type": "io.cozy.bank.operations" }
        }
      ]
    }
  }
}
```

**Note** `source` is automatically filled by the stack with the value extracted
from the token.

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notifications",
    "id": "c57a548c-7602-11e7-933b-6f27603d27da",
    "meta": {
      "rev": "1-1f2903f9a867"
    },
    "attributes": {
      "source": "190973ce-7605-11e7-ae6f-37c643b07905",
      "reference": "new operations",
      "title": "5 new operations on your bank account",
      "content":
        "You have 5 new operations on your bank account:\n-2 debit operations\n-3 credit operations",
      "icon": "https://calendar.alice.cozy.tools/alert.png",
      "actions": [
        {
          "text": "Show these operations",
          "intent": { "action": "OPEN", "type": "io.cozy.bank.operations" }
        }
      ]
    }
  }
}
```

## Updating a notification

### PATCH /notifications/:id

#### HTTP headers

It's possible to send the `If-Match` header, with the previous revision of the
notifications (optional).

#### Request

```http
PATCH /notifications/c57a548c-7602-11e7-933b-6f27603d27da HTTP/1.1
Host: alice.cozy.tools
Authorization: Bearer ...
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notifications",
    "id": "c57a548c-7602-11e7-933b-6f27603d27da",
    "attributes": {
      "title": "6 new operations on your bank account",
      "content":
        "You have 6 new operations on your bank account:\n-3 debit operations\n-3 credit operations"
    }
  }
}
```

#### Response

```http
HTTP/1.1 201 Created
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.notifications",
    "id": "c57a548c-7602-11e7-933b-6f27603d27da",
    "meta": {
      "rev": "2-037e9170adf1"
    },
    "attributes": {
      "source": "190973ce-7605-11e7-ae6f-37c643b07905",
      "reference": "new operations",
      "title": "6 new operations on your bank account",
      "content":
        "You have 6 new operations on your bank account:\n-1 debit operations\n-3 credit operations",
      "icon": "https://calendar.alice.cozy.tools/alert.png",
      "actions": [
        {
          "text": "Show these operations",
          "intent": { "action": "OPEN", "type": "io.cozy.bank.operations" }
        }
      ]
    }
  }
}
```

## Deleting a notification

### DELETE /notifications/:id

#### Request

```http
DELETE /notifications/c57a548c-7602-11e7-933b-6f27603d27da HTTP/1.1
Authorization: Bearer ...
```

#### Response

```http
HTTP/1.1 204 No Content
```
