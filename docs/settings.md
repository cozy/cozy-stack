[Table of contents](./README.md#table-of-contents)

# Settings

## Theme

### GET /settings/theme.css

It serves a CSS with variables that can be used as as theme. It contains the
following CSS variables:

Variable name    | Description
-----------------|------------------------------------------------------------------
`--logo-url`     | URL for a SVG logo of Cozy
`--base00-color` | Default Background
`--base01-color` | Lighter Background (Used for status bars)
`--base02-color` | Selection Background
`--base03-color` | Comments, Invisibles, Line Highlighting
`--base04-color` | Dark Foreground (Used for status bars)
`--base05-color` | Default Foreground, Caret, Delimiters, Operators
`--base06-color` | Light Foreground (Not often used)
`--base07-color` | Light Background (Not often used)
`--base08-color` | Variables, XML Tags, Markup Link Text, Markup Lists, Diff Deleted
`--base09-color` | Integers, Boolean, Constants, XML Attributes, Markup Link Url
`--base0A-color` | Classes, Markup Bold, Search Text Background
`--base0B-color` | Strings, Inherited Class, Markup Code, Diff Inserted
`--base0C-color` | Support, Regular Expressions, Escape Characters, Markup Quotes
`--base0D-color` | Functions, Methods, Attribute IDs, Headings
`--base0E-color` | Keywords, Storage, Selector, Markup Italic, Diff Changed
`--base0F-color` | Deprecated, Opening/Closing Embedded Language Tags e.g. <?php ?>

The variable names for colors are directly inspired from
[Base16 styling guide](https://github.com/chriskempson/base16/blob/master/styling.md).

For people with a bootstrap background, you can consider these equivalences:

Bootstrap color name | CSS Variable name in `theme.css`
---------------------|---------------------------------
Primary              | `--base0D-color`
Success              | `--base0B-color`
Info                 | `--base0C-color`
Warning              | `--base09-color`
Danger               | `--base08-color`

If you want to know more about CSS variables, I recommend to view this video:
[Lea Verou - CSS Variables: var(--subtitle);](https://www.youtube.com/watch?v=2an6-WVPuJU&app=desktop)
