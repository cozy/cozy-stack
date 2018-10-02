[Table of contents](README.md#table-of-contents)

# Notifications

Cozy applications can send notifications to the user, in order to alert or
notify for services message or state change that could be of interest to the
user, in an asynchronous manner.

These notifications are part of a "Notification Center" where the user can
configure the behavior of these notifications and the channel in which they are
sent.

## Declare application's notifications

Each application have to declare in its manifest the notifications it needs to
send. The `notifications` fields of the manifest can be used to define all these
notifications, with the following properties:

-   `collapsible` (boolean): defines a notification category for which only the
    last value is of interest to the user. For instance, a account-balance quota
    for a user's bank account: such notification is only useful as its last
    value.
-   `stateful` (boolean): defines a notification storing a piece of state: for
    each new notification, the stack will check that the last sent notification
    has a different state.
-   `multiple` (boolean): specify the possibility for a notification to have
    different sub-categories, defined by a programmable/dynamic identifier.
    `collapsible` and `stateful` properties are inherited for each sub-
    categories.
-   `default_priority`: default priority to use, with values "high" or "normal".
    This is propagated to the underlying mobile notifications system.
-   `templates`: a link list to templates file contained in the application
    folder that can be used to write the content of the notification, depending
    on the communication channel.

In this documentation, we take the example of an application with the following
notification:

```json
{
    "notifications": {
        "account-balance": {
            "description": "Alert the user when its account balance is negative",
            "collapsible": true, // only interested in the last value of the notification
            "multiple": true, // require sub-categories for each account
            "stateful": true, // piece of state to distinguish notifications
            "default_priority": "high", // high priority for this notification
            "templates": {
                "mail": "file:./notifications/account-balance-mail.tpl"
            }
        }
    }
}
```

## Creating a notification

### POST /notifications

This endpoint can be used to push a new notification to the user.

Notifications fields are:

-   `category` (string): name of the notification category
-   `category_id` (string): category name if the category is multiple
-   `title` (string): title of the notification (optionnal)
-   `message` (string): message of of the notification (optionnal)
-   `priority` (string): priority of the notification (`high` or `normal`), sent
    to the underlying channel to prioritize the notification
-   `state` (string): state of the notification, used for `stateful`
    notification categories, to distinguish notifications
-   `preferred_channels` (array of string): to select a list of preferred
    channels for this notification: either `"mobile"` or `"mail"`. The stack may
    chose another channels.
-   `data` (map): key/value map used to create the notification from its
    template, or sent in the notification payload for mobiles

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
        "attributes": {
            "category": "account-balance",
            "category_id": "my-bank",
            "title": "Your account balance is not OK",
            "message": "Warning: we have detected a negative balance in your my-bank",
            "priority": "high",
            "state": "-1",
            "preferred_channels": ["mobile"],
            "data": {
                "key1": "value1",
                "key2": "value2"
            }
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
            "rev": "1-1f2903f9a867"
        },
        "attributes": {
            "source_id": "cozy/app/bank/account-balance/my-bank",
            "originator": "app",
            "slug": "bank",
            "category": "account-balance",
            "category_id": "my-bank",
            "title": "Your account balance is not OK",
            "message": "Warning: we have detected a negative balance in your my-bank",
            "priority": "high",
            "state": "-1",
            "data": {
                "key1": "value1",
                "key2": "value2"
            }
        }
    }
}
```
