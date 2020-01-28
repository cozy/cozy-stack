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

![Setting a new passphrase](diagrams/bitwarden-onboarding.png)

A cozy organization is also created: it will be used to share some passwords
with the stack, to be used for the konnectors.

![Creating the organization key](diagrams/bitwarden-organization.png)

The bitwarden clients can connect to the cozy-stack APIs by setting their URL
to `https://<instance>/bitwarden`.

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
clientName=Cozy&
devicePushToken=
```

If authentication with two factors is enabled on the instance, this request
will fail with a 400 status, but it will send an email with the code. The
request can be retried with an additional paramter: `twoFactorToken`.

**Note:** the `clientName` parameter is optional, and is not sent by the
official bitwarden clients (a default value is used).

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

### GET /bitwarden/api/accounts/profile

#### Request

```http
GET /bitwarden/api/accounts/profile HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "Id": "0fbfc68d-ba11-416a-ac8a-a82600f0e601",
  "Name": "Alice",
  "Email": "me@alice.example.com",
  "EmailVerified": false,
  "Premium": false,
  "MasterPasswordHint": null,
  "Culture": "en-US",
  "TwoFactorEnabled": false,
  "Key": "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ=",
  "PrivateKey": null,
  "SecurityStamp": "5d203c3f-bc89-499e-85c4-4431248e1196",
  "Organizations": [],
  "Object": "profile"
}
```

### PUT /bitwarden/api/accounts/profile

This route allows to change the profile (currently, only the hint for the
master password). It can also be called with a `POST` (I think it is used by
the web vault).

#### Request

```http
PUT /bitwarden/api/accounts/profile HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "masterPasswordHint": "blah blah blah"
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "Id": "0fbfc68d-ba11-416a-ac8a-a82600f0e601",
  "Name": "Alice",
  "Email": "me@alice.example.com",
  "EmailVerified": false,
  "Premium": false,
  "MasterPasswordHint": "blah blah blah",
  "Culture": "en-US",
  "TwoFactorEnabled": false,
  "Key": "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ=",
  "PrivateKey": null,
  "SecurityStamp": "5d203c3f-bc89-499e-85c4-4431248e1196",
  "Organizations": [],
  "Object": "profile"
}
```

### POST /bitwarden/api/accounts/keys

This route is used to save a key pair (public and private keys), to be used
with organizations.

#### Request

```http
POST /bitwarden/api/accounts/keys HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "encryptedPrivateKey": "2.wZuKkufLV31Cpw1v1TQUDA==|u6bUNTaaGxu...y7s=",
  "publicKey": "MIIBIjANBgkqhkiG9w...AQAB"
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "Id": "0fbfc68d-ba11-416a-ac8a-a82600f0e601",
  "Name": "Alice",
  "Email": "me@alice.example.com",
  "EmailVerified": false,
  "Premium": false,
  "MasterPasswordHint": null,
  "Culture": "en-US",
  "TwoFactorEnabled": false,
  "Key": "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ=",
  "PrivateKey": "2.wZuKkufLV31Cpw1v1TQUDA==|u6bUNTaaGxu...y7s=",
  "SecurityStamp": "5d203c3f-bc89-499e-85c4-4431248e1196",
  "Organizations": [],
  "Object": "profile"
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

### GET /bitwarden/api/accounts/revision-date

It returns the date of the last change on the server, as a number of
milliseconds since epoch (sic). It is used by the clients to know if they have
to do a sync or if they are already up-to-date.

#### Request

```http
GET /bitwarden/api/accounts/revision-date HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 200 OK

1569571388892
```

### PUT /bitwarden/api/settings/domains

This route is also available via a `POST`, for compatibility with the web vault.

#### Request

```http
PUT /bitwarden/api/settings/domains HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "equivalentDomains": [
    ["stackoverflow.com", "serverfault.com", "superuser.com"]
  ],
  "globalEquivalentDomains": [42, 69],
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "EquivalentDomains": [
    ["stackoverflow.com", "serverfault.com", "superuser.com"]
  ],
  "GlobalEquivalentDomains": [
    { Type: 2, Domains: ["ameritrade.com", "tdameritrade.com"], Excluded: false },
    { Type: 3, Domains: ["bankofamerica.com", "bofa.com", "mbna.com", "usecfo.com"], Excluded: false },
    { Type: 42, Domains: ["playstation.com", "sonyentertainmentnetwork.com"], Excluded: true },
    { Type: 69, Domains: ["morganstanley.com", "morganstanleyclientserv.com"], Excluded: true }
  ],
  "Object": "domains"
}
```

### GET /bitwarden/api/settings/domains

#### Request

```http
GET /bitwarden/api/settings/domains HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "EquivalentDomains": [
    ["stackoverflow.com", "serverfault.com", "superuser.com"]
  ],
  "GlobalEquivalentDomains": [
    { Type: 2, Domains: ["ameritrade.com", "tdameritrade.com"], Excluded: false },
    { Type: 3, Domains: ["bankofamerica.com", "bofa.com", "mbna.com", "usecfo.com"], Excluded: false },
    { Type: 42, Domains: ["playstation.com", "sonyentertainmentnetwork.com"], Excluded: true },
    { Type: 69, Domains: ["morganstanley.com", "morganstanleyclientserv.com"], Excluded: true }
  ],
  "Object": "domains"
}
```

## Route for sync

### GET /bitwarden/api/sync

The main action of the client is a one-way sync, which just fetches all objects
from the server and updates its local database.

#### Request

```http
GET /bitwarden/api/sync HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Profile": {
		"Id": "0fbfc68d-ba11-416a-ac8a-a82600f0e601",
		"Name": "Alice",
		"Email": "me@alice.example.com",
		"EmailVerified": false,
		"Premium": false,
		"MasterPasswordHint": null,
		"Culture": "en-US",
		"TwoFactorEnabled": false,
		"Key": "0.uRcMe+Mc2nmOet4yWx9BwA==|PGQhpYUlTUq/vBEDj1KOHVMlTIH1eecMl0j80+Zu0VRVfFa7X/MWKdVM6OM/NfSZicFEwaLWqpyBlOrBXhR+trkX/dPRnfwJD2B93hnLNGQ=",
		"PrivateKey": null,
		"SecurityStamp": "5d203c3f-bc89-499e-85c4-4431248e1196",
		"Organizations": [{
      "Id": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
      "Name": "Cozy",
      "Key": "4.HUzVDQVAFc4JOpW3/j/QwZeET0mXOiDW5s/HdpxLZ2GFnGcxOm1FE4XD2p7XTSwORXO/Lo8y0A87UhXKEXzfHZmpJR04pbpUPr4NJbjRKv/cSkNFlvm0rIUw/m0Jkft/gew9v3QfkVSGdSZk5XIimwkTQ5WM+WCStxbQJIKAH+AoEA5q6t9mpNNlTAQvMgqs8u7CJwSjeZ7qbabfEUVX1HIPgxC3BtVUkySRSws/gUNeMwY23kAJJQYT+uuMooZUr7umU6YkEHG2RQZwCCjVHX4czxZRWsVo/xQOYoNr7DjgCf92D7OrJlFmDtQjzSy2BjotN6vn+1SwtHbeDILWaQ==",
      "BillingEmail": "me@cozy.tools",
      "Plan": "TeamsAnnually",
      "PlanType": 5,
      "Seats": 2,
      "MaxCollections": 1,
      "MaxStorageGb": 1,
      "SelfHost": true,
      "Use2fa": true,
      "UseDirectory": false,
      "UseEvents": false,
      "UseGroups": false,
      "UseTotp": true,
      "UsersGetPremium": true,
      "Enabled": true,
      "Status": 2,
      "Type": 2,
      "Object": "profileOrganization"
    }],
		"Object": "profile"
	},
	"Folders": [
		{
			"Id": "14220912-d002-471d-a364-a82a010cb8f2",
			"Name": "2.tqb+y2z4ChCYHj4romVwGQ==|E8+D7aR5CNnd+jF7fdb9ow==|wELCxyy341G2F+w8bTb87PAUi6sdXeIFTFb4N8tk3E0=",
			"RevisionDate": "2017-11-13T16:20:56.5633333",
			"Object": "folder"
		}
	],
	"Ciphers": [
		{
			"FolderId": null,
			"Favorite": false,
			"Edit": true,
			"Id": "0f01a66f-7802-42bc-9647-a82600f11e10",
			"OrganizationId": null,
			"Type":1,
			"Login":{
				"Uris": [
					{
						"Uri": "2.6DmdNKlm3a+9k/5DFg+pTg==|7q1Arwz/ZfKEx+fksV3yo0HMQdypHJvyiix6hzgF3gY=|7lSXqjfq5rD3/3ofNZVpgv1ags696B2XXJryiGjDZvk=",
						"Match": null,
					},
				],
				"Username": "2.4Dwitdv4Br85MABzhMJ4hg==|0BJtHtXbfZWwQXbFcBn0aA==|LM4VC+qNpezmub1f4l1TMLDb9g/Q+sIis2vDbU32ZGA=",
				"Password":"2.OOlWRBGib6G8WRvBOziKzQ==|Had/obAdd2/6y4qzM1Kc/A==|LtHXwZc5PkiReFhkzvEHIL01NrsWGvintQbmqwxoXSI=",
				"Totp":null,
			},
			"Name": "2.zAgCKbTvGowtaRn1er5WGA==|oVaVLIjfBQoRr5EvHTwfhQ==|lHSTUO5Rgfkjl3J/zGJVRfL8Ab5XrepmyMv9iZL5JBE=",
			"Notes": "2.NLkXMHtgR8u9azASR4XPOQ==|6/9QPcnoeQJDKBZTjcBAjVYJ7U/ArTch0hUSHZns6v8=|p55cl9FQK/Hef+7yzM7Cfe0w07q5hZI9tTbxupZepyM=",
			"Fields": null,
			"Attachments": null,
			"OrganizationUseTotp": false,
			"RevisionDate": "2017-11-09T14:37:52.9033333",
			"Object":"cipher"
		}
	],
  "Collections": [{
    "Id": "385aaa2a-d48d-11e9-bb5f-6b31dfebcb4d",
    "OrganizationId": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
    "Name": "2.PowfE263ZLz7+Jqrpuezqw==|OzuXDsJnQdfa/eMKxsms6Q==|RpEB7qqs26X9dqa+KaxSE5+52TFVs4dAdfU7DCu3QXM=",
    "Object": "collection"
  }],
	"Domains": {
		"EquivalentDomains": null,
		"GlobalEquivalentDomains": null,
		"Object": "domains"
	},
	"Object": "sync"
}
```

## Routes for ciphers

### GET /bitwarden/api/ciphers

It retrieves the list of ciphers.

#### Request

```http
GET /bitwarden/api/ciphers HTTP/1.1
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
      "Object": "cipher",
      "Id": "4c2869dd-0e1c-499f-b116-a824016df251",
      "Type": 1,
      "Favorite": false,
      "Name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
      "FolderId": null,
      "OrganizationId": null,
      "Notes": null,
      "Login": {
        "Uris": [
          {
            "Uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
            "Match": null,
          },
        ],
      },
      "Username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
      "Password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
      "Totp": null,
      "Fields": null,
      "Attachments": null,
      "RevisionDate": "2017-11-07T22:12:22.235914Z",
      "Edit": true,
      "OrganizationUseTotp": false
    }
  ],
  "Object": "list"
}
```

### POST /bitwarden/api/ciphers

When a new item (login, secure note, etc.) is created on a device, it is sent
to the server with its fields encrypted via this route.

#### Request

```http
POST /bitwarden/api/ciphers HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
	"type": 1,
	"favorite": false,
	"name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
	"folderId": null,
	"organizationId": null,
	"notes": null,
	"login": {
		"uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
		"username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
		"password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
		"totp": null
	}
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Object": "cipher",
	"Id": "4c2869dd-0e1c-499f-b116-a824016df251",
	"Type": 1,
	"Favorite": false,
	"Name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
	"FolderId": null,
	"OrganizationId": null,
	"Notes": null,
	"Login": {
		"Uris": [
			{
				"Uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
				"Match": null,
			},
		],
	},
	"Username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
	"Password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
	"Totp": null,
	"Fields": null,
	"Attachments": null,
	"RevisionDate": "2017-11-07T22:12:22.235914Z",
	"Edit": true,
	"OrganizationUseTotp": false
}
```

### POST /bitwarden/api/ciphers/create

This route also allows to create a cipher, but this time, it is for a cipher
shared with an organization.

#### Request

```http
POST /bitwarden/api/ciphers/create HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "cipher": {
    "type": 1,
    "favorite": false,
    "name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
    "folderId": null,
    "organizationId": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
    "notes": null,
    "login": {
      "uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
      "username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
      "password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
      "totp": null
    }
  },
  "collectionIds": ["385aaa2a-d48d-11e9-bb5f-6b31dfebcb4d"]
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Object": "cipher",
	"Id": "4c2869dd-0e1c-499f-b116-a824016df251",
	"Type": 1,
	"Favorite": false,
	"Name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
	"FolderId": null,
	"OrganizationId": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
	"Notes": null,
	"Login": {
		"Uris": [
			{
				"Uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
				"Match": null,
			},
		],
	},
	"Username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
	"Password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
	"Totp": null,
	"Fields": null,
	"Attachments": null,
	"RevisionDate": "2017-11-07T22:12:22.235914Z",
	"Edit": true,
	"OrganizationUseTotp": false
}
```

### GET /bitwarden/api/ciphers/:id

#### Request

```http
GET /bitwarden/api/ciphers/4c2869dd-0e1c-499f-b116-a824016df251 HTTP/1.1
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Object": "cipher",
	"Id": "4c2869dd-0e1c-499f-b116-a824016df251",
	"Type": 1,
	"Favorite": false,
	"Name": "2.d7MttWzJTSSKx1qXjHUxlQ==|01Ath5UqFZHk7csk5DVtkQ==|EMLoLREgCUP5Cu4HqIhcLqhiZHn+NsUDp8dAg1Xu0Io=",
	"FolderId": null,
	"OrganizationId": null,
	"Notes": null,
	"Login": {
		"Uris": [
			{
				"Uri": "2.T57BwAuV8ubIn/sZPbQC+A==|EhUSSpJWSzSYOdJ/AQzfXuUXxwzcs/6C4tOXqhWAqcM=|OWV2VIqLfoWPs9DiouXGUOtTEkVeklbtJQHkQFIXkC8=",
				"Match": null,
			},
		],
	},
	"Username": "2.JbFkAEZPnuMm70cdP44wtA==|fsN6nbT+udGmOWv8K4otgw==|JbtwmNQa7/48KszT2hAdxpmJ6DRPZst0EDEZx5GzesI=",
	"Password": "2.e83hIsk6IRevSr/H1lvZhg==|48KNkSCoTacopXRmIZsbWg==|CIcWgNbaIN2ix2Fx1Gar6rWQeVeboehp4bioAwngr0o=",
	"Totp": null,
	"Fields": null,
	"Attachments": null,
	"RevisionDate": "2017-11-07T22:12:22.235914Z",
	"Edit": true,
	"OrganizationUseTotp": false
}
```

### PUT /bitwarden/api/ciphers/:id

This route is used to change a cipher. It can also be called via
`POST /bitwarden/api/ciphers/:id` (I think it is used by the web vault).

#### Request

```http
PUT /bitwarden/api/ciphers/4c2869dd-0e1c-499f-b116-a824016df251 HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
	"type": 2,
	"favorite": true,
	"name": "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=",
	"folderId": "14220912-d002-471d-a364-a82a010cb8f2",
	"organizationId": null,
	"notes": "2.rSw0uVQEFgUCEmOQx0JnDg==|MKqHLD25aqaXYHeYJPH/mor7l3EeSQKsI7A/R+0bFTI=|ODcUScISzKaZWHlUe4MRGuTT2S7jpyDmbOHl7d+6HiM=",
	"secureNote": {
		"type": 0
	}
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Object": "cipher",
	"Id": "4c2869dd-0e1c-499f-b116-a824016df251",
	"Type": 2,
	"Favorite": true,
	"Name": "2.G38TIU3t1pGOfkzjCQE7OQ==|Xa1RupttU7zrWdzIT6oK+w==|J3C6qU1xDrfTgyJD+OrDri1GjgGhU2nmRK75FbZHXoI=",
	"FolderId": "14220912-d002-471d-a364-a82a010cb8f2",
	"OrganizationId": null,
	"Notes": "2.rSw0uVQEFgUCEmOQx0JnDg==|MKqHLD25aqaXYHeYJPH/mor7l3EeSQKsI7A/R+0bFTI=|ODcUScISzKaZWHlUe4MRGuTT2S7jpyDmbOHl7d+6HiM=",
  "SecureNote": {
    "Type": 0
  },
	"Fields": null,
	"Attachments": null,
	"RevisionDate": "2017-11-07T22:12:22.235914Z",
	"Edit": true,
	"OrganizationUseTotp": false
}
```

### POST /bitwarden/api/ciphers/:id/share

This route is used to share a cipher with an organization. The fields must be
encrypted with the organization key.

#### Request

```http
POST /bitwarden/api/ciphers/4c2869dd-0e1c-499f-b116-a824016df251/share HTTP/1.1
Host: alice.example.com
Content-Type: application/json
```

```json
{
  "cipher": {
    "type": 2,
    "favorite": true,
    "name": "2.d00W2bB8LhE86LybnoPnEQ==|QqJqmzMMv2Cdm9wieUH66Q==|TV++tKNF0+4/axjAeRXMxAkTdRBuIsXnCuhOKE0ESh0=",
    "organizationId": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
    "notes": "2.9m3XIbiJLk86thmF3UsO/A==|YC7plTgNQuMCkzYZC3iRjQ==|o8wZNQ3czr9sdeGXjOCalQwgPWsqOHZVnA2utZ+o/l4=",
    "secureNote": {
      "type": 0
    }
  },
  "collectionIds": ["385aaa2a-d48d-11e9-bb5f-6b31dfebcb4d"]
}
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
	"Object": "cipher",
	"Id": "4c2869dd-0e1c-499f-b116-a824016df251",
	"Type": 2,
	"Favorite": true,
  "Name": "2.d00W2bB8LhE86LybnoPnEQ==|QqJqmzMMv2Cdm9wieUH66Q==|TV++tKNF0+4/axjAeRXMxAkTdRBuIsXnCuhOKE0ESh0=",
  "OrganizationId": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
  "Notes": "2.9m3XIbiJLk86thmF3UsO/A==|YC7plTgNQuMCkzYZC3iRjQ==|o8wZNQ3czr9sdeGXjOCalQwgPWsqOHZVnA2utZ+o/l4=",
  "SecureNote": {
    "Type": 0
  },
	"Fields": null,
	"Attachments": null,
	"RevisionDate": "2017-11-07T22:12:22.235914Z",
	"Edit": true,
	"OrganizationUseTotp": false
}
```

### DELETE /bitwarden/api/ciphers/:id

This route is used to delete a cipher. It can also be called via
`POST /bitwarden/api/ciphers/:id/delete` (I think it is used by the web vault).

#### Request

```http
DELETE /bitwarden/api/ciphers/4c2869dd-0e1c-499f-b116-a824016df251 HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 204 No Content
```

## Routes for folders

### GET /bitwarden/api/folders

It retrieves the list of folders.

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

### DELETE /bitwarden/api/folders/:id

This route is used to delete a folder. It can also be called via
`POST /bitwarden/api/folders/:id/delete` (I think it is used by the web vault).

#### Request

```http
DELETE /bitwarden/api/folders/14220912-d002-471d-a364-a82a010cb8f2 HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 200 OK
```

## Cozy Organization

### GET /bitwarden/organizations/cozy

This route can be used to get information about the Cozy Organization. It
requires a permission on the whole `com.bitwarden.organizations` doctype to
access it. In particular, it gives the key to encrypt/decrypt the ciphers in
this organization (encoded in base64).

#### Request

```http
GET /bitwarden/organizations/cozy HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "organizationId": "38ac39d0-d48d-11e9-91bf-f37e45d48c79",
  "collectionId": "385aaa2a-d48d-11e9-bb5f-6b31dfebcb4d",
  "organizationKey": "oWeRYokoCMFsAja6lrp3RQ1PYOrex4tgAMECP4nX+a4IXdijbejQscvWqy9bMgLsX0HRc2igqBRMWdsPuFK0PQ=="
}
```

## Icons

### GET /bitwarden/icons/:domain/icon.png

This route returns an icon for the given domain, that can be used by the
bitwarden clients. No authorization token is required.

#### Request

```http
GET /bitwarden/icons/cozy.io/icon.png HTTP/1.1
Host: alice.example.com
```

## Hub

The hub is a way to get notifications in real-time about cipher and folder
changes.

### POST /bitwarden/notifications/hub/negotiate

Before connecting to the hub, the client make a request to this endpoint to
know what are the transports and formats supported by the server.

#### Request

```http
POST /bitwarden/notifications/hub/negotiate HTTP/1.1
Host: alice.example.com
```

#### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "connectionId": "NzhhYjU4NjgtZTA1NC0xMWU5LWIzNzAtM2I1YzM3YWQyOTc1Cg",
  "availableTransports": [
    { "Transport": "WebSockets", "Formats": ["Binary"] }
  ]
}
```

### GET /bitwarden/notifications/hub

This endpoint is used for WebSockets to get the notifications in real-time. The
client must do 3 things, in this order:

1. Make the HTTP request, with the token in the query string (`access_token`)
2. Upgrade the connection to WebSockets
3. Send JSON payload with `{"protocol": "messagepack", "version": 1}`.
