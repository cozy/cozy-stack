[Table of contents](README.md#table-of-contents)

# Security

## Checklist

1. Strong security is centered on the user
1. Defense in depth is still worthy
1. Have a strict control of all the accesses to the Cozy
1. Protect client-side apps by default
1. Don't trust inputs, always sanitize them
1. Encryption is not the solution to all the problems, but it helps
1. Use standards (https, OAuth2)
1. Have a reliable way to deploy, with sane defaults
1. No one is perfect, code must be reviewed
1. Be open to external contributors

## Rationale

### Strong security is centered on the user

For systems well designed, the user is often the weakest link in the security
chain. Engineers have a tendancy to overestimate the technical risks and
underestimate the human interactions. It doesn't mean that we can avoid
technical measures. But it's very important to take in account the possible
behaviour of the user. That means that adding a text to explain him/her the
consequences of a click on a button can increase the security a lot much than
forcing him/her to do things. For example, forcing users to change regulary
their password makes them choose passwords that are a lot weaker (but easier for
them to remember).

[![Security XKCD](https://imgs.xkcd.com/comics/security.png)](https://xkcd.com/538/)

### Defense in depth is still worthy

Also known as layered defense, defense in depth is a security principle where
single points of complete compromise are eliminated or mitigated by the
incorporation of a series or multiple layers of security safeguards and
risk-mitigation countermeasures. Have diverse defensive strategies, so that if
one layer of defense turns out to be inadequate, another layer of defense will
hopefully prevent a full breach.

### Have a strict control of all the accesses to the Cozy

All the requests to the cozy stack have a strict access control. It is based on
several informations:

-   Is the user connected?
-   What is the application that makes this request?
-   What are the permissions for this application?
-   Which grant is used, in particular for applications with public pages?
-   What are the permissions for this grant?

More informations [here](apps.md).

### Protect client-side apps by default

This is mostly applying the state of the art:

-   Using HTTPS, with HSTS.
-   Using secure, httpOnly,
    [sameSite](https://tools.ietf.org/html/draft-ietf-httpbis-cookie-same-site-00)
    cookies to avoid cookies theft or misuse.
-   Using a Content Security Policy (CSP).
-   Using X-frame-options http header to protect against click-jacking.

But we will use a CSP very restrictive by default (no access to other web
domains for example).

### Don't trust inputs, always sanitize them

If we take
[the OWASP top 10 vulnerabilities](https://www.owasp.org/index.php/Top_10_2013-Top_10),
we can see that a lot of them are related to trusting inputs. It starts with the
first one, injections, but it goes also to XSS and unvalidated forwards (in
OAuth2 notably). Always sanitizing inputs is a good pratice that improves
security, but has also some nice effects like helping developers discover hidden
bugs.

### Encryption is not the solution to all the problems, but it helps

Some data are encrypted before being saved in CouchDB (passwords for the
accounts for example). Encrypting everything has some downsides:

-   It's not possible to index encryped documents or do computations on the
    encrypted fields in reasonable time
    ([homomorphic encryption](https://en.wikipedia.org/wiki/Homomorphic_encryption)
    is still an open subject).
-   Having more encrypted data can globally weaken the encryption, if it's not
    handled properly.
-   If the encryption key is lost or a bug happen, the data is lost with no way
    to recover them.

So, we are more confortable to encrypt only some fields. And later, when we will
have more experience and feedbacks from the user, extend the encryption to more
fields.

We are also working with [SMIS](https://project.inria.fr/smis/), a research lab,
to find a way to securely store and backup the encryption keys.

### Use standards (https, OAuth2)

Standards like https and OAuth2 had a lot of eyes to look at them, and are
therefore more robust. Reinventing the wheel can be valuable for some things.
But for security, the cost is very high and only some very particular
constraints can justify such an high cost. Cozy don't have these constraints, so
we will stick to the standards.

### Have a reliable way to deploy, with sane defaults

It's important that deploying a cozy is well documented, doesn't require too
many steps and can be automatized. An error on the installation can have
dramatic effects, like the database being leaked on internet. So, we need to
really take care of the devops experience. In particular, having sane defaults
for the configuration will help to minimize the number of things he/she has to
do, and such the number of places where he/she can make faux pas.

### No one is perfect, code must be reviewed

No code is directly pushed to the master branch in git. It has to be reviewed by
at least one member of the core team (ideally the whole team), and this person
can't be the author of the change. Even the best developers can make mistakes,
and we don't pretend to be them. Our force is to work as team.

### Be open to external contributors

Our code is Open Source, external contributors can review it. If they (you?)
find a weakness, please contact us by sending an email to security AT
cozycloud.cc. This is a mailing-list specially setup for responsible disclosure
of security weaknesses in Cozy. We will respond in less than 72 hours.

When a security flaw is found, the process is the following:

-   Make a pull-request to fix (on our private git instance) and test it.
-   Deploy the fix on cozycloud.cc
-   Publish a new version, announce it on
    [the forum](https://forum.cozy.io/c/latest-information-about-cozy-security)
    as a security update and on the mailing-lists.
-   15 days later, add the details on the forum.
