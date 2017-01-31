[Table of contents](README.md#table-of-contents)

# Konnectors

## What we want?

[Konnectors](https://github.com/cozy-labs/konnectors) is an application for
Cozy v2 that fetch data from different web sites and services, and save them
into a Cozy. The 50+ connectors represent a lot of work from the community.
So, we want to port it to Cozy v3. There will be 2 parts:

- My Accounts, a client-side app, that will offer the possibility for the user
  to configure her accounts, and choose when to start the import of data.
- Konnectors, a worker for the [job service](jobs.md), with the code to import
  data from the web sites.


## Security risks

Konnectors is not just a random application. It's a very good target for
attacks on Cozy because of these specificities:

- It run on the server, where there is no Content Security Policy, or firewall
  to protect the stack.
- It has access to Internet, by design.
- It is written in nodejs, with a lot of dependencies where it is easy to hide
  malicious code.
- It is a collection of connectors written by a lot of people. We welcome
  these contributions, but it also means that we take into account that we
  can't review in depth all the contributions.

### Access to couchdb

The stack has the admin credentials of couchdb. If a rogue code can read its
configuration file or intercept connexions between the stack and couchdb, it
will have access to couchdb with the admin credentials, and can do anything on
couchdb.

### Access to the stack

An attackant can try to profit of konnectors for accessing the stack. It can
target the port 6060, used by the stack to manage the cozy instances. Or, it
can use its privileged position for timing attacks on passwords.

### Spying other connectors

A rogue connector may try to spy other connectors to pick the credentials for
external web sites. It can be done by reading the environment variables or
[ptracing](https://en.wikipedia.org/wiki/Ptrace) them.

### DoS

A connector can use a lot of CPU, Ram, or generate a lot of disk I/O to make a
deny of service on the server. The connector can remove files on the server to
make konnectors stop working.

### Exploiting the CPU or the bandwidth

The resources of the server can be seen as valuable: the CPU can be used for
bitcoins mining. The bandwidth can be used for DDoS of an external target.

### Sending spam

Profit of the configured SMTP server to send spams.

### Be root

[Row hammer](https://en.wikipedia.org/wiki/Row_hammer) can be a way to gain
root access on a server.


## Security measures

## Roadmap
