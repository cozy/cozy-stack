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

## Routes for accounts and connect

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

### POST /bitwarden/identity/connect/token

#### Request (initial connection)

```http
POST /bitwarden/identity/connect/token HTTP/1.1
Host: alice.example.com
Content-Type: application/x-www-form-urlencoded
```

```
grant_type=password&
username=me@alice.example.com&
password=r5CFRR+n9NQI8a525FY+0BPR0HGOjVJX0cR1KEMnIOo=&
scope=api offline_access&
client_id=browser&
deviceType=3&
deviceIdentifier=aac2e34a-44db-42ab-a733-5322dd582c3d&
deviceName=firefox&
devicePushToken=
```

#### Request (refresh token)

```http
POST /bitwarden/identity/connect/token HTTP/1.1
Host: alice.example.com
Content-Type: application/x-www-form-urlencoded
```

```
grant_type=refresh_token&
client_id=browser&
refresh_token=28fb1911ef6db24025ce1bae5aa940e117eb09dfe609b425b69bff73d73c03bf
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsImtpZCI6IkJDMz[...](JWT string)",
  "expires_in": 3600,
  "token_type": "Bearer",
  "refresh_token": "28fb1911ef6db24025ce1bae5aa940e117eb09dfe609b425b69bff73d73c03bf",
  "Key": "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ="
}
```

### POST /bitwarden/api/accounts/security-stamp

It allows to set a new security stamp, which has the effect to disconnect all
the clients. It can be used, for example, if the encryption key is changed to
avoid the clients to corrupt the vault with ciphers encrypted with the old key.

#### Request

```http
POST /bitwarden/api/accounts/security-stamp HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "masterPasswordHash": "r5CFRR+n9NQI8a525FY+0BPR0HGOjVJX0cR1KEMnIOo="
}
```

#### Response

```http
HTTP/1.1 204 No Content
```

## Routes for folders

### GET /bitwarden/api/folders

It retrieves the list of folders

#### Request

```http
GET /bitwarden/api/folders HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "Data": [
    {
      "Id": "14220912-d002-471d-a364-a82a010cb8f2",
      "Name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=",
      "RevisionDate": "2017-11-13T16:18:23.3078169Z",
      "Object": "folder"
    }
  ],
  "Object": "list"
}
```

### POST /bitwarden/api/folders

It adds a new folder on the server. The name is encrypted on client-side.

#### Request

```http
POST /bitwarden/api/folders HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o="
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Id": "14220912-d002-471d-a364-a82a010cb8f2",
	"Name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=",
	"RevisionDate": "2017-11-13T16:18:23.3078169Z",
	"Object": "folder"
}
```

### GET /bitwarden/api/folders/:id

#### Request

```http
GET /bitwarden/api/folders/14220912-d002-471d-a364-a82a010cb8f2 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Id": "14220912-d002-471d-a364-a82a010cb8f2",
	"Name": "2.FQAwIBaDbczEGnEJw4g4hw==|7KreXaC0duAj0ulzZJ8ncA==|nu2sEvotjd4zusvGF8YZJPnS9SiJPDqc1VIfCrfve/o=",
	"RevisionDate": "2017-11-13T16:18:23.3078169Z",
	"Object": "folder"
}
```

### PUT /bitwarden/api/folders/:id

This route is used to rename a folder. It can also be called via
`POST /bitwarden/api/folders/:id` (I think it is used by the web vault).

#### Request

```http
PUT /bitwarden/api/folders/14220912-d002-471d-a364-a82a010cb8f2 HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io="
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Id": "14220912-d002-471d-a364-a82a010cb8f2",
	"Name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
	"RevisionDate": "2017-11-13T16:18:23.3078169Z",
	"Object": "folder"
}
```

