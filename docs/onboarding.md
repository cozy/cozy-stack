[Table of contents](./README.md#table-of-contents)

# Onboarding

This document explains the architecture and process allowing a cozy instance owner to register to its cozy instance.

Compatibility with the current developments on cozy onboarding is a goal : [The following documents has been consulted for this proposal]( https://github.com/cozy/cozy-proxy/blob/bf9af7f2342e3fc183a8b4e72bcedb909afa3eb8/docs/client/)

## Instance creation

Creating an instance is done through CLI or through the (future) partner farm manager system. Some **settings** can be pre-defined on instance creation. ([doc](./instance.md#creation)).

The CLI also allows to specify which source to use for `onboarding` and `home` applications. The defaults will be hosted on `github.com/cozy`.

After creation, an instance has a `registerToken` generated randomly.

All cozy-instance fields are considered private unless explictly prefixed by `public-`. Public instance field can be seen from the `instance` endpoint. Only public fields we can think of for now are `public-name`, `public-locale`.

## Onboarding steps

This document and the cozy-stack are only concerned with login and passphrase registering step which are important for security.

All other steps are handled by the `onboarding` application.

The `onboarding` application SHOULD therefore provide the following features
- When started with a `registerToken`, allow the user to create a passphrase
- When started with a `contextToken` ([see auth doc](./auth.md#how-to-get-a-token)) use it to retrieve instance document.
  - If the instance document is complete **according to the `onboarding` app**, redirect to `home` application.
  - Otherwise, performs whatever steps it deems necessary to fill out the instance (ask for user email, help set up `myaccounts` accounts, say thank you...)

This makes cozy-stack simple and safer while allowing behaviour modification for several install types by picking the correct `onboarding` application / branch.

This makes it easier to add more onboarding steps and have them run on already-installed cozy : On next login after onboarding application update, it will ask the user.

## Redirections

When an user attempts to access the root of its instance (`https://example.cozycloud.cc`) or an application (`https://contacts.example.cozycloud.cc`), and she is not logged-in, she is redirected :

- If the instance has a `passphrase` set, to the `/login` page
- If the instance has a `registerToken` set, to the `onboarding` application.

After login, the user is always redirected to the `onboarding` application. It is the `onboarding` application responsibility to check if registering is complete and reredirect to home.


## Routes

### POST /auth/passphrase

The onboarding application can send a request to this endpoint to register the
passphrase of the user. The `registrationToken` can only be used once.

#### Request

```http
POST /auth/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/x-www-form-urlencoded

registerToken=37cddf40d7724988860fa0e03efd30fe&passphrase=oGh2aek2Thoh8daeeoXohk9uOhz4aeSo
```

#### Response

```http
HTTP/1.1 303 See Other
Location: https://onboarding.alice.example.com/
Set-Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

### PUT /auth/passphrase

The user can change its passphrase with this route

#### Request

```http
PUT /auth/passphrase HTTP/1.1
Host: alice.example.com
Content-Type: application/x-www-form-urlencoded
Cookie: cozysessid=AAAAAFhSXT81MWU0ZTBiMzllMmI1OGUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa

current-passphrase=oGh2aek2Thoh8daeeoXohk9uOhz4aeSo&new-passphrase=Ee0vohChUQuohch5urahN9yuLeexex5a
```

#### Response

```http
HTTP/1.1 303 See Other
Location: https://home.alice.example.com/
Set-Cookie: cozysessid=AAAAShoo3uo1Maic4VibuGohlik2eKUyMmZiN2Q0YTYzNDAxN2Y5NjCmp2Ja56hPgHwufpJCBBGJC2mLeJ5LCRrFFkHwaVVa; Path=/; Domain=alice.example.com; Max-Age=604800; HttpOnly; Secure
```

### GET /instance

If the user is not logged in, display public instance information

#### Request
```http
GET https://alice.example.com/instance HTTP/1.1
```

#### Response
```json
{
  "public-locale":"fr",
  "public-name":"Alice Martin"
}
```

If the user is logged in, display all instance information, except passphrase

#### Request
```http
GET https://alice.example.com/instance HTTP/1.1
Cookie: sessionid=xxxx
```

#### Response
```json
{
  "_rev":"3-56521545485448482",
  "domain": "alice.example.com",
  "owner-email": "alice@example.com",
  "public-locale":"fr",
  "public-name":"Alice Martin"
}
```

### POST /instance

If the user is logged in, allow to set instance fields

#### Request
```http
POST https://alice.example.com/instance HTTP/1.1
Content-type: application/json
Cookie: sessionid=xxxxx
Authorization: base64(onboarding:onboardingapptoken)
```
```json
{
  "_rev":"3-56521545485448482",
  "tz": "Europe/Berlin"
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
  "_rev":"4-15454854484828845221285",
  "tz": "Europe/Berlin",
  "domain": "alice.example.com",
  "owner-email": "alice@example.com",
  "public-locale":"fr",
  "public-name":"Alice Martin"
}
```

## Flow Example

- The server administrator Bob creates an instance through the CLI. He knows the instance should be in french for an user named `alice`.
```
cozy-stack instances add alice.example.com --locale fr --public-name "Alice Martin" --tz Europe/Paris
>> https://alice.cozycloud.cc?registerToken=42456565213125454842
```
The instance is created
```json
{
  "domain": "alice.example.com",
  "publicName": "Alice Martin",
  "tz": "Europe/Paris",
  "locale":"fr"
}
```
- Eve knows Alice just had an instance created, she goes to `https://alice.cozycloud.cc`. There is no `registerToken`, so she only see a message (in french) along the lines of "This is the cozy for Alice Martin, this register link is incorrect, if you are Alice Martin please ask your sysadmin for a new link".
- Alice navigates to `https://alice.cozycloud.cc?registerToken=42...42`
- She is redirected to the `onboarding` application
- The `onboarding` application receive the registerToken. It is the default onboarding application and therefore display the cozy cloud agreement and then ask for a Password.
- The `onboarding` application use its `registerToken` to register the passphrase. Registering the passphrase automatically log Alice in and redirect her back to the `onboarding` app.
- Afterward, the `onboarding` app receive its token normally through the `data-cozy-token` body attribute, as described in [./auth.md](auth documentation). and can do whatever it needs to do :
  - read from the instance document to prefill/bypass form fields
  - add more informations to the instance document.
  - create `io.cozy.accounts` documents for external accounts.
- When the onboarding application is satisfied, Alice is redirected to the `home` application
