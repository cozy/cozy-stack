[Table of contents](README.md#table-of-contents)

# Workers

This page list all the currently available workers on the cozy-stack. It
describes their input arguments object. See the [jobs document](jobs.md) to know
more about the API context in which you can see how to use these arguments.

## log worker

The `log` worker will just print in the log file the job sent to it. It can
useful for debugging for example.

## push worker

The `push` worker can be used to send push-notifications to a user's device. The
options are:

-   `client_id`: the ID of the oauth client to push a notification to.
-   `title`: the title of the notification
-   `message`: the content of the notification
-   `data`: key-value string map for additional metadata (optional)
-   `priority`: the notification priority: `high` or `normal` (optional)
-   `topic`: the topic identifier of the notification (optional)
-   `sound`: a sound associated with the notification (optional)

### Example

```json
{
    "client_id": "abcdef123123123",
    "title": "My Notification",
    "message": "My notification content.",
    "priority": "high"
}
```

### Permissions

To use this worker from a client-side application, you will need to ask the
permission. It is done by adding this to the manifest:

```json
{
    "permissions": {
        "push-notification": {
            "description": "Required to send notifications",
            "type": "io.cozy.jobs",
            "verbs": ["POST"],
            "selector": "worker",
            "values": ["push"]
        }
    }
}
```

## unzip worker

The `unzip` worker can take a zip archive from the VFS, and will unzip the files
inside it to a directory of the VFS. The options are:

-   `zip`: the ID of the zip file
-   `destination`: the ID of the directory where the files will be unzipped.

### Example

```json
{
    "zip": "8737b5d6-51b6-11e7-9194-bf5b64b3bc9e",
    "destination": "88750a84-51b6-11e7-ba90-4f0b1cb62b7b"
}
```

### Permissions

To use this worker from a client-side application, you will need to ask the
permission. It is done by adding this to the manifest:

```json
{
    "permissions": {
        "unzip-to-a-directory": {
            "description": "Required to unzip a file inside the cozy",
            "type": "io.cozy.jobs",
            "verbs": ["POST"],
            "selector": "worker",
            "values": ["unzip"]
        }
    }
}
```

## sendmail worker

The `sendmail` worker can be used to send mail from the stack. It implies that
the stack has properly configured an access to an SMTP server. You can see an
example of configuration in the [cozy.example.yaml](../cozy.example.yaml) file
at the root of this repository.

`sendmail` options fields are the following:

-   `mode`: string specifying the mode of the send:
    -   `noreply` to send a notification mail to the user
    -   `from` to send a mail from the user
-   `to`: list of object `{name, email}` representing the addresses of the
    recipients. (should not be used in `noreply` mode)
-   `subject`: string specifying the subject of the mail
-   `parts`: list of part objects `{type, body}` listing representing the
    content parts of the
    -   `type` string of the content type: either `text/html` or `text/plain`
    -   `body` string of the actual body content of the part
-   `attachments`: list of objects `{filename, content}` that represent the
    files attached to the email

### Examples

```js
// from mode sending mail from the user to a list of recipients
{
    "mode": "from",
    "to": [
        {"name": "John Doe 1", "email":"john1@doe"},
        {"name": "John Doe 2", "email":"john2@doe"}
    ],
    "subject": "Hey !",
    "parts": [
        {"type":"text/html", "body": "<h1>Hey !</h1>"},
        {"type":"text/plain", "body": "Hey !"}
    ]
}

// noreply mode, sending a notification mail to the user
{
    "mode": "noreply",
    "subject": "You've got a new file !",
    "parts": [
        {"type":"text/html", "body": "<h1>Hey !</h1>"},
        {"type":"text/plain", "body": "Hey !"}
    ]
}
```

### Permissions

To use this worker from a client-side application, you will need to ask the
permission. It is done by adding this to the manifest:

```json
{
    "permissions": {
        "mail-from-the-user": {
            "description": "Required to send mails from the user to his/her friends",
            "type": "io.cozy.jobs",
            "verbs": ["POST"],
            "selector": "worker",
            "values": ["sendmail"]
        }
    }
}
```

## export

The `export` worker can be used to generate allow the export of all data
contained in the cozy. At the end of the export, a mail is sent to the user
containing a link to access to its data.

The progress of the export process can be followed with realtime events on the
doctype `io.cozy.exports`.

Its options are:

-   `parts_size`: the size in bytes of the sizes index splitting done for
    multi-part download of files data
-   `max_age`: the maximum age duration of the archive before it expires
-   `with_doctypes`: list of string for a whitelist of doctypes to exports
    (exports all doctypes if empty)
-   `without_files`: boolean to avoid exporting the index (preventing download
    file data)

### Example

```json
{
    "parts_size": 52428800,
    "max_age": 60000000000, // 1 minute
    "with_doctypes": ["io.cozy.accounts"], // empty or null means all doctypes
    "without_files": false
}
```

### Permissions

To use this worker from a client-side application, you will need to ask the
permission. It is done by adding this to the manifest:

```json
{
    "permissions": {
        "mail-from-the-user": {
            "description": "Required to create a export of the user's data",
            "type": "io.cozy.jobs",
            "verbs": ["POST"],
            "selector": "worker",
            "values": ["export"]
        }
    }
}
```

## share workers

The stack have 3 workers to power the sharings (internal usage only):

1. `share-track`, to update the `io.cozy.shared` database
2. `share-replicate`, to start a replicator for most documents
3. `share-upload`, to upload files

### Share-track

The message is composed of 3 fields: the sharing ID, the rule index, and the
doctype. The event is similar to a realtime event: a verb, a document, and
optionaly the old version of this document.

### Share-replicate and share-upload

The message is composed of a sharing ID and a count of the number of errors
(i.e. the number of times this job was retried).
