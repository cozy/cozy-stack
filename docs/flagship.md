# Flagship App

## Introduction

The flagship application is a mobile application that can be used to run all
the Cozy webapps inside it. It also has a bunch of exclusive features like the
client-side connectors. But, to be able to work, it needs a very powerful
access to the Cozy instance. Technically, it means that the flagship app is an
OAuth client with a scope that covers everything. And we want to ensure that
only the flagship can have this power. So, we require the application to be
certified. It can be certified via the Google and Apple stores, and when it's
not available, the user has the possibility to manually certify the flagship
app (lineage OS users for example).

Let's see the workflow when a user opens the flagship app for the first time.
The app will load, and it will open a page of the cloudery for the onboarding.
The user can type their email address, the cloudery sends an email, the user
opens a link in this email, and then, we have two cases: the user already have
a Cozy instance, or they want to create a new one. In both cases, the flagship
will register its-self as an OAuth client on the Cozy and will type their
passphrase before going to the home of the Cozy inside the flagship app. It is
just that for a new Cozy, there are a few additional steps like accepting the
terms of services.

**Note:** the flagship app can be used with self-hosted stack. The user will
have to type their Cozy address instead of using their email address, but
everything else should work, including the certification via the Google and
Apple stores (there are some config parameters, but they have a default value
that make this possible).

## Create the OAuth client & certification

When the flagship app knows the Cozy instance of the user, it will register
itself as an OAuth client, and will try to attest that it is really the
flagship app via the Google and Apple APIs. It is done by calling these 3
routes from the stack:

1. `POST /auth/register`
2. `POST /auth/clients/:client-id/challenge`
3. `POST /auth/clients/:client-id/attestation`

Between 2 and 3, the app will ask the mobile OS to certify that this is really
the flagship app. It is done via the [SafetyNet attestation
API](https://developer.android.com/training/safetynet/attestation) on Android,
and the [AppAttest API](https://developer.apple.com/documentation/devicecheck)
on iOS.

## New Cozy instance

On a new Cozy instance, the user will choose a passphrase that will be
registered on the Cozy by sending its PBKDF2 hash to
`POST /settings/passphrase/flagship`.

## Existing Cozy instance

On an existing Cozy instance, the app will fetch some parameters with
`GET /public/prelogin`, then the user can type their passphrase, and the PBKDF2
hash will be sent to `POST /auth/login/flagship` to give the app access to the
whole Cozy.

## Manual certification

When the certification from the Google and Apple stores has failed, the app
will have to do a few more steps to have access to the Cozy:

1. Opening an in-app browser on the authorize page of the stack
   `GET /auth/authorize?scope=*`
   (note: PKCE is mandatory for the flagship app)

2. The stack sends a mail with a 6 digits code in that case. The email explains
   that this code can be used to certify the flagship app.

3. The user can type this code on the authorize page and submit it. It makes
   a `POST /auth/clients/:client-id/flagship`.

4. If successful, it redirects back to the same page
   (`GET /auth/authorize?scope=*`), but this time, the user has button to
   accept using the flagship app.

5. The user accepts, it sends a request to `POST /auth/authorize`, and the
   flagship app can finish the OAuth danse and have a full access on the Cozy.

## Special routes

Some routes of the stack are dedicated to the flagship app, like:

- creating a session code with `POST /auth/session_code`
- getting the parameters to open a webapp with `GET /apps/:slug/open`

And some routes accept a `session_code` to open a session in a webview or
inAppBrowser, like:

- serving the `index.html` of a webapp
- starting the OAuth danse of a konnector (`/accounts/:slug/start`)
- starting the reconnect BI flow (`/accounts/:slug/reconnect`).
