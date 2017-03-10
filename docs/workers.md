[Table of contents](README.md#table-of-contents)

# Workers

This page list all the currently available workers on the cozy-stack. It
describes their input arguments object. See the [jobs document](jobs.md) to
know more about the API context in which you can see how to use these
arguments.

## sendmail worker

The `sendmail` worker can be used to send mail from the stack. It implies that
the stack has properly configured an access to an SMTP server. You can see an
example of configuration in the [cozy.example.yaml](../cozy.example.yaml) file
at the root of this repository.

`sendmail` options fields are the following:

- `mode`: string specifying the mode of the send:
    - `noreply` to send a notification mail to the user
    - `from` to send a mail from the user
- `to`: list of object `{name, email}` representing the addresses of the
  recipients. (should not be used in `noreply` mode)
- `subject`: string specifying the subject of the mail
- `parts`: list of part objects `{type, body}` listing representing the
  content parts of the
    - `type` string of the content type: either `text/html` or `text/plain`
    - `body` string of the actual body content of the part

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
