[Table of contents](README.md#table-of-contents)

# Links between the Virtual File System and the Data System

## What we want?

Cozy applications can use data from the Data System and files from the Virtual
File System. Of course, sometimes a link between data and files can be useful.
For example, the application can have an album with photos. The album will be
a document in CouchDB (with a title and other fields), but il will also list
the files to use as photos.

A direct way to do that is storing the files IDs in the album document. It's
simple and will work pretty well if the files are manipulated only from this
application. But, files are often accessed from other apps, like cozy-desktop
and cozy-files-v3. To improve the User eXperience, it should be nice to alert
the user when a file in an album is modified or deleted.

When a file is modified, we can offer the user the choice between keeping the
original version in the album, or using the new modified file. When a file is
moved to the trash, we can alert the user and let him/her restore the file.

Cozy-desktop, cozy-files-v3, and the other apps can't scan all the documents
with many different doctypes to find all the references to a file to detect
such cases. The goal of this document is to offer a way to do that.
