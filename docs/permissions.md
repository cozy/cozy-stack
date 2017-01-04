[Table of contents](./README.md#table-of-contents)

# Permissions

An application has a list of permissions that the users has allowed. Each
permission has a key, a description and an optional access level. The key is
composed of two parts: the service that will perform the operation, and a
subtype specific for each service. The description should explain why the
permission is explained (the system can already gives a message on the "what"
by using the key), and can be localized in the manifest.

For data, the permission key is composed of `data/` and the doctype. The
access can be `read`, `write` or `readwrite`.

For files, the permission key is composed of `files/` and a type of files. The
access can also be `read`, `write` or `readwrite`. The type can be :

**Type**    | **Description**
------------|---------------------------------------------------------
`app`       | the folder `Apps/:app_name` and the files inside it
`data`      | the folder `Documents/:app_name` and the files inside it
`downloads` | the folder `Documents/downloads` and the file inside it
`pictures`  | the folder `Documents/pictures` and the files inside it
`music`     | the folder `Documents/music` and the files inside it
`videos`    | the folder `Documents/videos` and the files inside it

The `file/app` permission is powerful, it gives the app the possibility to
modify itself. It can be dangerous, but it allows to create some static files
for when JS is not an option. For example, a blog application can generate an
RSS feed and upload it to this folder.

For jobs, the permission key is composed of `jobs/` and the worker name. Some
workers can use the `access` to restrict the permission (e.g. `konnectors` use
the `access` to say which konnector can be used).

For settings, the permission key is composed of `settings/` and a type. The
access can be `read`, `write` and `readwrite`. The type can be:

**Type**     | **Description**
-------------|--------------------------------------------
`locale`     | the default locale for the cozy instance
`background` | the background for the home
`theme`      | the CSS theme
`owner`      | the name of the owner of this cozy instance
`all`        | all the things list above

Example:

```json
{
  "permissions": {
    "data/io.cozy.contacts": {
      "description": "Required for autocompletion on @name",
      "access": "read"
    },
    "files/images": {
      "description": "Required for the background",
      "access": "read"
    },
    "jobs/sendmail": {
      "description": "Required to send a congratulations email to your friends"
    },
    "settings/theme": {
      "description": "Required to use the same colors as other cozy apps",
      "access": "read"
    }
  }
}
```


