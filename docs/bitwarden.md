[Table of contents](README.md#table-of-contents)

# Bitwarden

Cozy-stack exposes an API compatible with
[Bitwarden](https://github.com/bitwarden) on `/bitwarden`.

The author of the [unofficial Bitwarden-ruby
server](https://github.com/jcs/rubywarden) did some reverse engineering and
wrote a short [API
documentation](https://github.com/jcs/rubywarden/blob/master/API.md).

## Setup

The signup is disabled, there is one account per Cozy instance, with the email
`me@<domain>`. When the user chooses his/her password (onboarding), an encryption key
is also generated to keep safe the secrets in the bitwarden vault.

![Setting a new passphrase](diagrams/bitwarden-cheatsheet.png)

## Routes

### POST /bitwarden/api/accounts/prelogin

It allows the client to know the number of KDF iterations to apply when hashing
the master password.

#### Request

```http
POST /bitwarden/api/accounts/prelogin HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "email": "me@alice.example.com"
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "Kdf": 0,
  "KdfIterations": 10000,
}
```
