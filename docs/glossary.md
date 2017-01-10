[Table of contents](README.md#table-of-contents)

# Glossary

## Instance

An instance is a logical space owned by a user and identified by a domain. For
example, zoe.cozycloud.cc can be the cozy instance of Zoé. This instance has a
space for storing files and some CouchDB databases for storing the documents
of its owner.

## Environment

When creating an instance, it's possible to give an environment, `dev`, `test`
or `prod`. The default apps won't be the same on all environments. For
example, in the `dev` environment, some devtools will be installed to help the
front developers to create their own apps.

## Cozy Stack Build Mode

The cozy stack can run in several modes, set by a UNIX environment variable:

- `production`, the default
- `development`, for coding on the cozy stack.

This mode is set when compiling the cozy-stack. It is used to show more or
less logs, and what is acceptable to be displayed in errors.

Even if the Cozy Stack Build Mode and Environment have similar values, they
are not the same. The Cozy Stack Mode will be used by core developers to hack
on the cozy stack. The environment will be used by front developers to hack on
cozy apps.
