[Table of contents](README.md#table-of-contents)

# Adding a new doctype

There is currently several steps to add a new doctype. We hope to improve that
in the near future. For the moment, this guide can help you to not forget a step.

## Choosing a name for your doctype

The doctype name is something like `io.cozy.contacts`. The `io.cozy` prefix is
reserved for doctypes created and managed by Cozy Cloud. If you have an
official website, you can use it (in reversed order) for the prefix. Else, you
can use your github or gitlab handle: `github.nono` or `gitlab.nono`.

Then, you can add a plural noun to indicate what is the type of the documents.
If you have several related doctypes, it is common to nest them. For example,
`io.cozy.contacts.accounts` is the accounts of external services used to
synchronize the `io.cozy.accounts`.

## Add documentation about your doctype

The doctypes are documented on https://docs.cozy.io/en/cozy-doctypes/docs/README/
to help other developers to reuse the same doctypes. If you think that your
doctype may be useful to others, you can make a pull request on the
https://github.com/cozy/cozy-doctypes repository.

**Note:** it's the **docs** directory that you should update. The other
directories are used by [remote doctypes](./remote.md).

## Add your doctype to the store and stack

Both the store and the stack knows of the doctypes, for showing permissions.
For the store, it is shown to the user before installing an application that
uses this doctype. For the stack, it is used for showing permissions for
sharings and OAuth clients. In both cases, there is a short description of
the doctype, localized on transifex, and an icon to illustrate it.

Here are the relevant places:

- https://github.com/cozy/cozy-store/blob/master/src/locales/en.json
- https://github.com/cozy/cozy-store/blob/master/src/config/permissionsIcons.json
- https://github.com/cozy/cozy-store/tree/master/src/assets/icons/permissions
- https://github.com/cozy/cozy-stack/blob/master/assets/locales/en.po
- https://github.com/cozy/cozy-stack/blob/master/assets/styles/stack.css

## Using the doctype in your application

Of course, after all those efforts, you want to use your new doctype in your
application. Do not forget to add a permission in the manifest to use it!
