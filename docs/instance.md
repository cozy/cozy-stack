Instances
=========

**TODO** it's just a first draft that needs to be completed and more explicit

Creation
--------

An instance is created on the command line:

```sh
$ cozy-stack instances add <domain>
```

It registers the instance in a global database (**TODO** which one), and
creates some databases for these doctypes:

- `io.cozy.apps`
- `io.cozy.manifests`
- `io.cozy.files`
- `io.cozy.notifications`
- `io.cozy.settings`

Then, it creates some folders:

- `/`, with the id `io.cozy.folders-root`
- `/Apps`, with the id `io.cozy.folders-apps`
- `/Documents`, with the id `io.cozy.folders-documents`
- `/Documents/Downloads`, with the id `io.cozy.folders-downloads`
- `/Documents/Pictures`, with the id `io.cozy.folders-pictures`
- `/Documents/Music`, with the id `io.cozy.folders-music`
- `/Documents/Videos`, with the id `io.cozy.folders-videos`

The ids are forced to known values. So, even if these folders are moved or
renamed, they can still be found for the permissions.

Finally, the default application for the current environment are installed.

**TODO** explain that the folder names will be localized, and how
