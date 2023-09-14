[Table of contents](README.md#table-of-contents)

# Working with the stack assets

The cozy-stack has some assets: templates, CSS, JS, fonts, etc. For the
deployment in production, they are bundler in the go code and compiled to the
binary. But, it's also nice for developers to have them in a readable format
with a git history: they are also put in the `assets` directory, and some of
them are downloaded from other repositories and listed in `assets/.externals`.

## How to work on them on local?

In short:

```sh
$ scripts/build.sh debug-assets
$ go run . serve --assets debug-assets/
```

The first command creates a debug-assets directory, with symlinks for local
assets. It also downloads the external assets. The second command starts the
cozy-stack with those assets. If you modify one of the assets (local or
externals) and reload the page in your browser, you will see the new version.

Tip: if you are debugging an external asset, you may find it practical to
replace the file in `debug-assets` by a symlink from where you build this
asset. For example:

```sh
$ rm debug-assets/css/cozy-ui.min.css
$ ln -s path/to/cozy-ui/dist/cozy-ui.min.css debug/assets/css/cozy-ui.min.css
```

## `/dev` route

In development mode, a `/dev` route is available to render a template or a mail
with given parameter. For example:

### Emails

* http://cozy.localhost:8080/dev/mails/alert_account
* http://cozy.localhost:8080/dev/mails/archiver
* http://cozy.localhost:8080/dev/mails/confirm_flagship
* http://cozy.localhost:8080/dev/mails/export_error
* http://cozy.localhost:8080/dev/mails/import_error
* http://cozy.localhost:8080/dev/mails/import_success
* http://cozy.localhost:8080/dev/mails/magic_link
* http://cozy.localhost:8080/dev/mails/move_confirm
* http://cozy.localhost:8080/dev/mails/move_error
* http://cozy.localhost:8080/dev/mails/move_success
* http://cozy.localhost:8080/dev/mails/new_connection
* http://cozy.localhost:8080/dev/mails/new_registration
* http://cozy.localhost:8080/dev/mails/notifications_diskquota
* http://cozy.localhost:8080/dev/mails/notifications_oauthclients
* http://cozy.localhost:8080/dev/mails/notifications_sharing
* http://cozy.localhost:8080/dev/mails/passphrase_hint
* http://cozy.localhost:8080/dev/mails/passphrase_reset
* http://cozy.localhost:8080/dev/mails/sharing_request
* http://cozy.localhost:8080/dev/mails/sharing_to_confirm
* http://cozy.localhost:8080/dev/mails/support_request
* http://cozy.localhost:8080/dev/mails/two_factor?TwoFactorPasscode=123456
* http://cozy.localhost:8080/dev/mails/two_factor_mail_confirmation
* http://cozy.localhost:8080/dev/mails/update_email

### HTML pages

* http://cozy.localhost:8080/dev/templates/authorize.html
* http://cozy.localhost:8080/dev/templates/authorize_move.html
* http://cozy.localhost:8080/dev/templates/authorize_sharing.html
* http://cozy.localhost:8080/dev/templates/compat.html
* http://cozy.localhost:8080/dev/templates/confirm_auth.html
* http://cozy.localhost:8080/dev/templates/confirm_flagship.html?Email=jane@example.com
* http://cozy.localhost:8080/dev/templates/error.html?Error=oops&Button=Click%20me&ButtonURL=https://cozy.io/
* http://cozy.localhost:8080/dev/templates/import.html
* http://cozy.localhost:8080/dev/templates/instance_blocked.html?Reason=test
* http://cozy.localhost:8080/dev/templates/login.html
* http://cozy.localhost:8080/dev/templates/magic_link_twofactor.html
* http://cozy.localhost:8080/dev/templates/move_confirm.html?Email=jane@example.com
* http://cozy.localhost:8080/dev/templates/move_delegated_auth.html
* http://cozy.localhost:8080/dev/templates/move_in_progress.html
* http://cozy.localhost:8080/dev/templates/move_link.html?Link=https://jane.mycozy.cloud/&Illustration=no
* http://cozy.localhost:8080/dev/templates/move_vault.html
* http://cozy.localhost:8080/dev/templates/need_onboarding.html
* http://cozy.localhost:8080/dev/templates/new_app_available.html
* http://cozy.localhost:8080/dev/templates/oidc_login.html
* http://cozy.localhost:8080/dev/templates/oidc_twofactor.html
* http://cozy.localhost:8080/dev/templates/passphrase_choose.html
* http://cozy.localhost:8080/dev/templates/passphrase_reset.html?ShowBackButton=true&HasCiphers=true&HasHint=true
* http://cozy.localhost:8080/dev/templates/sharing_discovery.html?PublicName=Jane&RecipientDomain=mycozy.cloud&NotEmailError=true
* http://cozy.localhost:8080/dev/templates/oauth_clients_limit_exceeded.html
* http://cozy.localhost:8080/dev/templates/twofactor.html?TrustedDeviceCheckBox=true

## In production

The script `scripts/build.sh assets` download the externals assets, and
transform all the assets (local and externals) to go code. This command is used
by the maintainers of cozy-stack, so you should not have to worry about that
;-)

## Locales

The locales are managed on transifex. The `.po` files are put in `assets/locales`.
The master branch is synchronized with transifex via their github integration.

## Contexts

It's possible to overload some assets on a context with the `cozy-stack config
insert-asset` command. See [its
manpage](https://docs.cozy.io/en/cozy-stack/cli/cozy-stack_config_insert-asset/)
and [Customizing a context](https://docs.cozy.io/en/cozy-stack/config/#customizing-a-context)
for more details.
