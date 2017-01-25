[Table of contents](README.md#table-of-contents)

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


## Disk usage

### GET /settings/disk-usage

Says how many bytes are used to store files.

#### Request

```http
GET /settings/disk-usage HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Authorization: Bearer ...
```

#### Response

```http
HTTP/1.1 200 OK
Content-type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.disk-usage",
    "attributes": {
      "used": "12345678"
    }
  }
}
```

## Passphrase

### POST /settings/passphrase

The onboarding application can send a request to this endpoint to register the
passphrase of the user. The registration token can only be used once.

#### Request

```http
POST /settings/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "register_token": "37cddf40d7724988860fa0e03efd30fe",
  "passphrase": "ThisIsTheNewShinnyPassphraseChoosedByAlice"
}
```

#### Response

```http
HTTP/1.1 204 No Content
Set-Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

### PUT /settings/passphrase

The user can change its passphrase with this route

#### Request

```http
PUT /settings/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/json
Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa
```

```
{
  "current_passphrase": "ThisIsTheNewShinnyPassphraseChoosedByAlice",
  "new_passphrase": "AliceHasChangedHerPassphraseAndThisIsTheNewPassphrase"
}
```

#### Response

```http
HTTP/1.1 204 No Content
Set-Cookie: cozysessid=AAAAShoo3uo1Maic4VibuGohlik2eKUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

## Instance

### GET /settings/instance

If the user is logged in, display all instance settings. If the user is not
logged in, the register token can be used to read the informations.

#### Request

```http
GET /settings/instance HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Cookie: sessionid=xxxx
```

#### Response
```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.instance",
    "meta": {
      "rev": "3-56521545485448482"
    },
    "attributes": {
      "locale":"fr",
      "email": "alice@example.com",
      "public_name":"Alice Martin"
    }
  }
}
```

### PUT /settings/instance

If the user is logged in, allow to set the instance fields

#### Request
```http
POST /settings/instance HTTP/1.1
Host: alice.example.com
Accept: application/vnd.api+json
Content-type: application/vnd.api+json
Cookie: sessionid=xxxxx
Authorization: Bearer settings-token
```
```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.instance",
    "meta": {
      "rev": "3-56521545485448482"
    },
    "attributes": {
      "locale":"fr",
      "email": "alice@example.com",
      "public_name":"Alice Martin",
      "timezone": "Europe/Berlin"
    }
  }
}
```

#### Response
```
HTTP/1.1 200 OK
Content-type: application/json
Cookie: sessionid=xxxxx
```
```json
{
  "data": {
    "type": "io.cozy.settings",
    "id": "io.cozy.settings.instance",
    "meta": {
      "rev": "4-5a3e315e"
    },
    "attributes": {
      "locale":"fr",
      "email": "alice@example.com",
      "public_name":"Alice Martin",
      "timezone": "Europe/Berlin"
    }
  }
}
```
