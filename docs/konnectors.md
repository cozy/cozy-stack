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


## Security

### The risks

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

#### Access to couchdb

The stack has the admin credentials of couchdb. If a rogue code can read its
configuration file or intercept connexions between the stack and couchdb, it
will have access to couchdb with the admin credentials, and can do anything on
couchdb.

#### Access to the stack

An attacker can try to profit of konnectors for accessing the stack. It can
target the port 6060, used by the stack to manage the cozy instances. Or, it
can use its privileged position for timing attacks on passwords.

#### Spying other connectors

A rogue connector may try to spy other connectors to pick the credentials for
external web sites. It can be done by reading the environment variables or
[ptracing](https://en.wikipedia.org/wiki/Ptrace) them.

#### DoS

A connector can use a lot of CPU, Ram, or generate a lot of disk I/O to make a
deny of service on the server. The connector can remove files on the server to
make konnectors stop working.

#### Exploiting the CPU or the bandwidth

The resources of the server can be seen as valuable: the CPU can be used for
bitcoins mining. The bandwidth can be used for DDoS of an external target.

#### Sending spam

Profit of the configured SMTP server to send spams.

#### Be root

[Row hammer](https://en.wikipedia.org/wiki/Row_hammer) can be a way to gain
root access on a server.


### Possible measures

#### Permissions

We can forbid the konnectors to speak directly with couchdb, and pass by the
stack for that. And use the [permissions](permissions.md) to restrict what
each konnectors can do with the cozy-stack.

#### ignore-scripts for npm/yarn

Npm and yarn can execute scripts defined in package.json when installing
nodejs dependencies. We can use the
[`ignore-scripts`](https://docs.npmjs.com/misc/config#ignore-scripts) option
to disable this behaviour.

#### Forbid addons in nodejs

Nodejs can require [addons](https://nodejs.org/api/addons.html), ie C/C++
compiled libraries. I've found no flag to disable the install of such modules
for npm/yarn, and no flag for nodejs to prevent loading them. We can try to
detect and remove such modules just after the installation of node modules.
They should have a `.node` extension.

**Note**: not having a compiler on the server is not enough. Npm can install
precompiled modules.

#### vm/sandbox for Nodejs

[vm2](https://github.com/patriksimek/vm2) is a sandbox that can run untrusted
code with whitelisted Node's built-in modules.

#### Mock net

We can mock the net module of nodejs to add some restrictions on what it can
do. For example, we can check that it does only http/https, and blacklist
connection to localhost:6060. It is only effective if the konnector has no way
to start a new node processus.

#### Timeout

If a konnector takes too long, after a timeout, it should be killed. It
implies that the cozy-stacks supervises the konnectors.

#### Chroot

[Chroot](https://en.wikipedia.org/wiki/Chroot) is a UNIX syscall that makes an
application see only a part of the file-system. In particular, we can remove
access to `/proc` and `/sys` by not mounting them, and limit access to `/dev`
to just `/dev/null`, `/dev/zero`, and `/dev/random` by symlinks them.

#### Executing as another user

We can create UNIX users that will just serve to execute the konnectors, and
nothing else. It's a nice way to give more isolation, but it means that we
have to find a way to execute the konnectors: either run the cozy-stack as
root, or have a daemon that launches the konnectors.

#### Ulimit & Prlimit

[ulimit](http://ss64.com/bash/ulimit.html) provides control over the resources
available to the shell and to processes started by it, on systems that allow
such control. It can be used to linit the number of processes (protection
against fork bombs), or the memory that can be used.

[prlimit](http://man7.org/linux/man-pages/man1/prlimit.1.html) can do the same
for just one command (technically, for a new session, not the current one).

#### Linux namespaces

One feature Linux provides here is namespaces. There are a bunch of different
kinds:

- in a pid namespace you become PID 1 and then your children are other
  processes. All the other programs are gone
- in a networking namespace you can run programs on any port you want without
  it conflicting with what’s already running
- in a mount namespace you can mount and unmount filesystems without it
  affecting the host filesystem. So you can have a totally different set of
  devices mounted (usually less).

It turns out that making namespaces is totally easy! You can just run a
program called [unshare](http://man7.org/linux/man-pages/man1/unshare.1.html).

Source: [What even is a container, by Julia
Evans](https://jvns.ca/blog/2016/10/10/what-even-is-a-container/)

#### Cgroups

[cgroups](https://en.wikipedia.org/wiki/Cgroups) (abbreviated from control
groups) is a Linux kernel feature that limits, accounts for, and isolates the
resource usage (CPU, memory, disk I/O, network, etc.) of a collection of
processes.

#### Seccomp BPF

Seccomp BPF is an extension to seccomp that allows filtering of system calls
using a configurable policy implemented using Berkeley Packet Filter rules.

#### Isolation in a docker

[Isode](https://github.com/tjanczuk/isode) is a 3 years old project that aims
to isolate nodejs apps in docker containers. A possibility would be to follow
this path and isolate the konnectors inside docker. It's a real burden for
administrators, so we will try to avoid it.

Isolation in docker contains is mostly a combination of Linux Namespaces,
Cgroups, and Seccomp BPF.

#### Rkt

[Rkt](https://coreos.com/rkt/) is a security-minded, standard-based container
engine. It is similar to Docker, but Docker needs running a daemon whereas rkt
can be launched from command-line with no daemon.

#### NsJail / FireJail

[NsJail](https://google.github.io/nsjail/) and
[FireJail](https://firejail.wordpress.com/) are two tools that use Linux
Namespaces and Seccomp BPF to reduce the risks to run untrusted applications
on Linux. FireJail seems to be more suited for graphical apps, and NsJail for
networking services.

#### NaCl / ZeroVM

[ZeroVM](http://www.zerovm.org/) is an open source virtualization technology
that is based on the Chromium [Native
Client](https://en.wikipedia.org/wiki/Google_Native_Client) (NaCl) project.
ZeroVM creates a secure and isolated execution environment which can run a
single thread or application. But NaCl is [no longer
maintained](https://bugs.chromium.org/p/chromium/issues/detail?id=239656#c160)
and ZeroVM has some severe limitations, so, it won't be used.
