[Table of contents](README.md#table-of-contents)

# Notifications

Inspiration: https://developer.android.com/guide/topics/ui/notifiers/notifications.html

Notifications are couchdb documents with the doctype `io.cozy.notifications`.

Schema will be defined in cozy-doctypes, along the lines of
```
source: applicationID
notificationType: text // useful to "hide notif like this"
title: text
content?: text
icon?: image
actions?: [{text, intent}]
```

All applications/services/konnector can create notifications (no permissions needed).
But for reading notifications, a permission is needed on `io.cozy.notifications`:

- Notifications will appear in the cozy-bar.
- Cozy mobile app(s) may display notifications on mobile.
- Some notifications may also be transmitted by email, eventually as summaries over period of time.
- The settings/notifications app will have "notifications center" tab, allowing to silence notifications and pick which should be sent to mobile / mail


## Listing/getting notifications

You can use the `/data/io.cozy.notifications` routes to read the notifications.


## Creating a notification

### POST /notifications

#### Request

```http
POST /notifications HTTP/1.1
Host: alice.cozy.tools
Authentication: Bearer ...
Content-Type: application/vnd.api+json
```
```json
{
  "data": {
    "type": "io.cozy.notifications",
    "attributes": {
      "notificationType": "new operations",
      "title": "5 new operations on your bank account",
      "content": "You have 5 new operations on your bank account:\n-2 debit operations\n-3 credit operations",
      "icon": "https://calendar.alice.cozy.tools/alert.png",
      "actions": [
        {
          "text": "Show these operations"
          "intent": {"action": "OPEN", "type": "io.cozy.bank.operations"}
        }
      ]
    }
  }
}
```

**Note** `source` is automatically filled by the stack with the value extracted from the token.

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
      "notificationType": "new operations",
      "title": "5 new operations on your bank account",
      "content": "You have 5 new operations on your bank account:\n-2 debit operations\n-3 credit operations",
      "icon": "https://calendar.alice.cozy.tools/alert.png",
      "actions": [
        {
          "text": "Show these operations"
          "intent": {"action": "OPEN", "type": "io.cozy.bank.operations"}
        }
      ]
    }
  }
}
```


## Updating a notification

### PUT /notifications/:id
