[Table of contents](README.md#table-of-contents)

# How-to customize a context?

## Intro

In the config file of cozy-stack, it's possible to declare some contexts, that
are a way to regroup some cozy instances to give similar configuration. For
example, it is possible to give a `default_redirection` that will be used
when the user logs into their cozy. You can find more example in the example
config file.

## Assets

The visual appearance of a cozy instance can be customized via some assets
(CSS, JS, images). These assets can be inserted from the command-line with the
[`cozy-stack config insert-asset`](../cli/cozy-stack_config_insert-asset.md)
command.

Here are a small list of assets that you may want to customize:

- `/styles/theme.css`: a CSS file where you can override the colors and put
  other CSS rules
- `/favicon-16x16.png` and `/favicon-32x32.png`: the two variants of the
  favicon
- `/apple-touch-icon.png`: the same but for Apple
- `/images/default-avatar.png`: the image to use as the default avatar.
