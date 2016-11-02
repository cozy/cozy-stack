Authentication and access delegations
=====================================

Introduction
------------

In this document, we will cover how to protect the usage of the cozy-stack.
When the cozy-stack receives a request, it checks that the request is
authorized, and if yes, it processes it and answers it.


What about OAuth2?
------------------

TODO what is OAuth2, what it aims to do
TODO the 4 actors
TODO what is in OAuth2 and what is left as an exercise to the reader
TODO the 4 grant types and the 3 ways to send the access token
  -> Authorization code
  -> Implicit grant type
  -> Client credentials grant type
  -> Resource owner credentials grant type

TODO list assumptions made in OAuth2
  -> TLS
  -> trust on first use principle

If you want to learn OAuth 2 in details, I recommend the [OAuth 2 in Action
book](https://www.manning.com/books/oauth-2-in-action).


The cozy stack as an authorization server
-----------------------------------------

TODO: list the routes

TODO: 2FA


Client-side apps
----------------

**Important**: OAuth2 is not used here! The steps are similar (like obtaining
a token), but going in the details, it doesn't match.

### How to register the application?

The application is registered at install. See [app management](apps.md) for
details.

### How to get a token?

When a user access an application, she first loads the HTML page. Inside this
page, the `<body>` tag has a `data-cozy-token` attribute with a token. This
token is specific to a context, that can be either public or private.

We have prefered our custom solution to the implicit grant type of OAuth2 for
2 reasons:

1. It has a better User Experience. The implicit grant type works with 2
redirections (the application to the stack, and then the stack to the
application), and the first one needs JS to detect if the token is present or
not in the fragment hash. It has a strong impact on the time to load the
application.

2. The implicit grant type of OAuth2 has a severe drawback on security: the
token appears in the URL and is shown by the browser. It can also be leaked
with the HTTP `Referer` header.

### How to refresh a token?

### How to use a token?


Devices
-------

https://tools.ietf.org/html/draft-ietf-oauth-native-apps-05
[PKCE](https://tools.ietf.org/html/rfc7636) and chapter 10


Browser extensions
------------------

https://developer.chrome.com/apps/app_identity#non
https://developer.chrome.com/apps/identity#method-getRedirectURL
https://github.com/AdrianArroyoCalle/firefox-addons/blob/master/addon-google-oauth2/addon-google-oauth2.js#L26


Third-party websites
--------------------


Template
--------

### How to register the application?

See chapter 12

https://tools.ietf.org/html/rfc7591
https://tools.ietf.org/html/rfc7592

### How to get a token?

Chapter 11 about JWT

### How to refresh a token?

### How to use a token?


Security considerations
-----------------------

See https://tools.ietf.org/html/rfc6749#page-53
and https://tools.ietf.org/html/rfc6819
and https://tools.ietf.org/html/draft-ietf-oauth-closing-redirectors-00

Dynamically registered applications won't have access to some scopes. For
example, an application that has been dynamically registered can't ask the
cozy owner to give it the right to install other applications. This limitation
should improve security, as avoiding too powerful scopes to be used with
unknown applications.

TODO: rate limiting
