[Table of contents](README.md#table-of-contents)

# Important changes

This section will list important changes to the stack or its usage, and migration procedures if any is needed.


## October 2019: Authentication

We are about to change our authentification logic. 

This change will be transparent for end-users. End-users don't need to change anything.

Developpers that used to manually send [POST /auth/login](https://docs.cozy.io/en/cozy-stack/auth/#post-authlogin), [POST /auth/passphrase_renew](https://docs.cozy.io/en/cozy-stack/auth/#post-authpassphrase_renew) or [POST /settings/passphrase](https://docs.cozy.io/en/cozy-stack/settings/) requests will need to update their code.

### What are the changes

Previously, we expected the user to type its passphrase on the browser. This passphrase was sent to the stack. There it was salted, hashed and compared to the salted-hashed version stored on our systems.

From today, the browser will send an intermediary secret to the server instead of your passphrase. The rest of the process will stay unchanged. We'll salt and hash this intermediary secret ans compare it the the salted-hashed version stored on our system. 

The three API endpoint upper have been updated and now require you to pass this intermediary secret instead of the old passphrase. The settings endpoint will also expect a parameter with a number of pbkdf2 iterations.


### Dirty details for developers

The intermediary secret is built from the user passphrase and the URL of its instance with the following formula:

	user-iterations = # from a /bitwarden/api/accounts/prelogin request
	user-salt = "me@" + host
    key = pbdkf2(passphrase, salt=user-salt, iterations=user-salt, alg=SHA-256)
    secret = pbkdf2(key, salt=passphrase, iterations=1, alg=SHA-256)

…where `passphrase` is the user passphrase in UTF-8 and `host` is the instance host string (without protocol, like "example.mycozy.cloud"). The default iteration number in the first step (`user-iterations`) is a per-user settings set by the stack. You can find it by emitting a [prelogin request](https://docs.cozy.io/en/cozy-stack/bitwarden/#post-bitwardenapiaccountsprelogin). 

You can also see the `user-salt` and `user-iterations` in [io.cozy.settings](https://github.com/cozy/cozy-doctypes/blob/master/docs/io.cozy.settings.md) or through a [GET /settings/passphrase](https://docs.cozy.io/en/cozy-stack/settings/#get-settingspassphrase) request. 

For most of users, the today's default is 100_000 pbkdf2 iterations. Users initializing their account with a Edge browser will default to 10_000 iterations (Edge does not have a native pbkdf2 function and this is a balance between security and user experience).

As always, if you have any doubt, you can explore our own source code. At the time of writing, there is a function `nativeHash` on [password-helpers.js](https://github.com/cozy/cozy-stack/blob/master/assets/scripts/password-helpers.js) used on the login page of the stack.

A schema of the new architecture is available in the [Bitwarden section of the stack](https://docs.cozy.io/en/cozy-stack/bitwarden/).

### Migration

Users will migrate transparently to the new authentification logic on their first login. The stack will intercept the passphrase sent by the browser, create the intermediary secret and store it salted-hashed for all future logins. Once the migration done, the stack will not allow direct passphrase logins anymore.

### Why this change

Some content will support end-to-end encryption in the future. To ease the use for most users, we'll use the same passphrase for to login in their cozy instance and to encrypt their data. As we do not want the server to be able to read this encrypted data, we do not want to know your real passphrase anymore.



