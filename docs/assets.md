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

## In production

The script `scripts/build.sh assets` download the externals assets, and
transform all the assets (local and externals) to go code. This command is used
by the maintainers of cozy-stack, so you should not have to worry about that
;-)

## Locales

The locales are managed on transifex. The `.po` files are put in `assets/locales`.
The source (english) is kept in the git repository, and is used to push the new
translations to transifex. The other locales are pulled from transifex to the
assets directory while building the go code.

## Contexts

It's possible to overload some assets on a context with the `cozy-stack config
insert-asset` command. See [its
manpage](https://docs.cozy.io/en/cozy-stack/cli/cozy-stack_config_insert-asset/)
for more details.
