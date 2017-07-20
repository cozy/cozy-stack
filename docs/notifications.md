[Table of contents](README.md#table-of-contents)

## Notifications

Inspiration : https://developer.android.com/guide/topics/ui/notifiers/notifications.html

Notifications are couchdb document with doctype `io.cozy.notifications`.

Schema will be defined in cozy-doctypes, along the lines of
```
notificationRef // useful to update a notification (X mails unread)
notificationType: // useful to "hide notif like this"
source: applicationID
title: text
content?: text
icon?: image
actions?: [{text, intents}]
```

All applications/service/konnector can create notifications.

- notifications will appears in the cozy-bar
- Cozy mobile app(s) may display notifications on mobile.
- Some notifications may also be transmitted by email, eventually as summaries over period of time.
- The settings/notifications app will have "notifications center" tab, allowing to silence notifications and pick which should be sent to mobile / mail

**WIP should we have a permission for notification creation?**
**WIP should notification have a type defined by the app, allowing finer control in the notification center?**
