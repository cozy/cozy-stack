[Table of contents](README.md#table-of-contents)

# Accept from Flagship

Flagship app is a mobile app that allows users to access all their Cozy from their phone (see [here](https://github.com/cozy/cozy-flagship-app) for more details).

When the Flagship app is installed on the user's phone, then they can share a document with the app to upload it in their Cozy.

When doing so, the user will have to chose which cozy-app should receive the shared document from a list of elligible cozy-apps (i.e. cozy-drive, mespapiers, etc)

To be elligible, a cozy-app has to declare what it can receive. This is done in its `manifest.webapp` file.

## The manifest

The `manifest.webapp` file is used to declare the cozy-app's ability to receive files

### `accept_from_flagship` field

First field to declare is `accept_from_flagship`. It should be set to `true` to make the cozy-app visible in the list of elligible cozy-apps

Example:
```json
{
  "name": "Drive",
  "name_prefix": "Cozy",
  "slug": "drive",
  //...
  "accept_from_flagship": true,
  //...
}
```

### `accept_documents_from_flagship` field

Declaring `accept_from_flagship: true` is not enough to be able to receive files from the Flagship app. The app should also declare which kind of files it can handle.

The field `accept_documents_from_flagship` is an object containing all criteria that a file or a list of files should meet to be sharable with the cozy-app.

#### `accepted_mime_types` criteria

`accepted_mime_types` is used to declare each file type that can be handled by the cozy-app.

This field should contain a list of all mime types that are supported by the cozy-app.

Example of a cozy-app accepting PDF, and pictures:
```json
{
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "accepted_mime_types": ["application/pdf", "image/jpeg", "image/png"],
  }
}
```

In order to accept all files types, it is possible to use `'*/*'` mime type

Example of a cozy-app accepting all types of documents:
```json
{
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "accepted_mime_types": ["*/*"],
  }
}
```

#### `max_number_of_files` criteria

`max_number_of_files` is used to declare the maximum number of files that can be shared simultaneously with the cozy-app.

Example of a cozy-app accepting only 1 document at a time:
```json
{
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "max_number_of_files": 10,
  }
}
```

Example of a cozy-app accepting up to 10 documents at a time:
```json
{
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "max_number_of_files": 10,
  }
}
```

Setting a limit is mandatory. The Flagship app doesn't support unlimited file number.

#### `max_size_per_file_in_MB` criteria

`max_size_per_file_in_MB` is used to declare the maximum size of files that can be handled by the cozy-app.

The size limit is declared in MB.

The size limit is per file. If multiple files are shared with the cozy-app, then each file size should be under that limit.

Example of a cozy-app accepting documents up to 10MB:
```json
{
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "max_size_per_file_in_MB": 10,
  }
}
```

Setting a limit is mandatory. The Flagship app doesn't support unlimited file size.

#### `route_to_upload` criteria

`route_to_upload` is used to declare the cozy-app's route that should be used by the Flagship app when sharing files with the cozy-app.

The app should then implement a page on that route that will be responsible to handle those documents (i.e. ask the user where to save the document, analyse the document etc)

Example:
```json
{
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "route_to_upload": "/#/upload?fromFlagshipUpload=true",
  }
}
```

#### The complete manifest

Here is an example of a `manifest.webapp` file for an app accepting only up to 10 picture files, with a maximum of 10MB:
```json
{
  "name": "Drive",
  "name_prefix": "Cozy",
  "slug": "drive",
  //...
  "accept_from_flagship": true,
  "accept_documents_from_flagship": {
    "accepted_mime_types": ["image/jpeg", "image/png"],
    "max_number_of_files": 10,
    "max_size_per_file_in_MB": 10,
    "route_to_upload": "/#/upload?fromFlagshipUpload=true"
  }
  //...
}
```

Note that `accept_from_flagship` may seems to be redundant if `accept_documents_from_flagship` exists. This field is an optimization to allow cozy-client queries to filter only cozy-apps that accept sharings.

## The API

When a cozy-app is selected by the user to receive a shared file, this cozy-app is opened using the `route_to_upload` route.

Then the cozy-app can use the Flagship OsReceive API to handle shared files.

This API is documented [here](https://github.com/cozy/cozy-flagship-app/tree/master/src/app/domain/osReceive/os-receive-api.md)
