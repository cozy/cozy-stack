[Table of contents](README.md#table-of-contents)

# Konnectors

:warning: **Note:** this documentation is outdated. It is kept for historical
reasons and shows the design and trade-offs we have made initially. But a lot
of things have changed since, so don't expect the things to be still the same.

## What we want ?

[Konnectors](https://github.com/cozy-labs/konnectors) is an application for Cozy
v2 that fetch data from different web sites and services, and save them into a
Cozy. The 50+ connectors represent a lot of work from the community. So, we want
to port it to Cozy v3. There will be 2 parts:

-   My Accounts, a client-side app, that will offer the possibility for the user
    to configure her accounts, and choose when to start the import of data (see
    [the architecture doc](https://github.com/cozy-labs/konnectors/blob/development/docs/client-side-architecture.md)).
-   Konnectors, a worker for the [job service](jobs.md), with the code to import
    data from the web sites.

## Security

### The risks

Konnectors is not just a random application. It's a very good target for attacks
on Cozy because of these specificities:

-   It run on the server, where there is no Content Security Policy, or firewall
    to protect the stack.
-   It has access to Internet, by design.
-   It is written in nodejs, with a lot of dependencies where it is easy to hide
    malicious code.
-   It is a collection of connectors written by a lot of people. We welcome
    these contributions, but it also means that we take into account that we
    can't review in depth all the contributions.

#### Access to couchdb

The stack has the admin credentials of couchdb. If a rogue code can read its
configuration file or intercept connexions between the stack and couchdb, it
will have access to couchdb with the admin credentials, and can do anything on
couchdb.

#### Access to the stack

An attacker can try to profit of konnectors for accessing the stack. It can
target the port 6060, used by the stack to manage the cozy instances. Or, it can
use its privileged position for timing attacks on passwords.

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

[Row hammer](https://en.wikipedia.org/wiki/Row_hammer) can be a way to gain root
access on a server.

### Possible measures

#### Permissions

We can forbid the konnectors to speak directly with couchdb, and pass by the
stack for that. And use the [permissions](permissions.md) to restrict what each
konnectors can do with the cozy-stack.

#### ignore-scripts for npm/yarn

Npm and yarn can execute scripts defined in package.json when installing nodejs
dependencies. We can use the
[`ignore-scripts`](https://docs.npmjs.com/misc/config#ignore-scripts) option to
disable this behaviour.

#### Forbid addons in nodejs

Nodejs can require [addons](https://nodejs.org/api/addons.html), ie C/C++
compiled libraries. I've found no flag to disable the install of such modules
for npm/yarn, and no flag for nodejs to prevent loading them. We can try to
detect and remove such modules just after the installation of node modules. They
should have a `.node` extension.

**Note**: not having a compiler on the server is not enough. Npm can install
precompiled modules.

#### vm/sandbox for Nodejs

[vm2](https://github.com/patriksimek/vm2) is a sandbox that can run untrusted
code with whitelisted Node's built-in modules.

#### Mock net

We can mock the net module of nodejs to add some restrictions on what it can do.
For example, we can check that it does only http/https, and blacklist connection
to localhost:6060. It is only effective if the konnector has no way to start a
new node processus.

#### Timeout

If a konnector takes too long, after a timeout, it should be killed. It implies
that the cozy-stacks supervises the konnectors.

#### Chroot

[Chroot](https://en.wikipedia.org/wiki/Chroot) is a UNIX syscall that makes an
application see only a part of the file-system. In particular, we can remove
access to `/proc` and `/sys` by not mounting them, and limit access to `/dev` to
just `/dev/null`, `/dev/zero`, and `/dev/random` by symlinks them.

#### Executing as another user

We can create UNIX users that will just serve to execute the konnectors, and
nothing else. It's a nice way to give more isolation, but it means that we have
to find a way to execute the konnectors: either run the cozy-stack as root, or
have a daemon that launches the konnectors.

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

-   in a pid namespace you become PID 1 and then your children are other
    processes. All the other programs are gone
-   in a networking namespace you can run programs on any port you want without
    it conflicting with what’s already running
-   in a mount namespace you can mount and unmount filesystems without it
    affecting the host filesystem. So you can have a totally different set of
    devices mounted (usually less).

It turns out that making namespaces is totally easy! You can just run a program
called [unshare](http://man7.org/linux/man-pages/man1/unshare.1.html).

Source:
[What even is a container, by Julia Evans](https://jvns.ca/blog/2016/10/10/what-even-is-a-container/)

#### Cgroups

[cgroups](https://en.wikipedia.org/wiki/Cgroups) (abbreviated from control
groups) is a Linux kernel feature that limits, accounts for, and isolates the
resource usage (CPU, memory, disk I/O, network, etc.) of a collection of
processes.

#### Seccomp BPF

Seccomp BPF is an extension to seccomp that allows filtering of system calls
using a configurable policy implemented using Berkeley Packet Filter rules.

#### Isolation in a docker

[Isode](https://github.com/tjanczuk/isode) is a 3 years old project that aims to
isolate nodejs apps in docker containers. A possibility would be to follow this
path and isolate the konnectors inside docker.

It's a real burden for administrators. And its command line options often
changes from one version to another, making difficult to deploy something
reliable for self-hosted users. So we will try to avoid it.

Isolation in docker contains is mostly a combination of Linux Namespaces,
Cgroups, and Seccomp BPF. There are other options with those (see below).

#### Rkt

[Rkt](https://coreos.com/rkt/) is a security-minded, standard-based container
engine. It is similar to Docker, but Docker needs running a daemon whereas rkt
can be launched from command-line with no daemon.

#### NsJail / FireJail

[NsJail](https://google.github.io/nsjail/) and
[FireJail](https://firejail.wordpress.com/) are two tools that use Linux
Namespaces and Seccomp BPF to reduce the risks to run untrusted applications on
Linux. FireJail seems to be more suited for graphical apps, and NsJail for
networking services.

#### NaCl / ZeroVM

[ZeroVM](http://www.zerovm.org/) is an open source virtualization technology
that is based on the Chromium
[Native Client](https://en.wikipedia.org/wiki/Google_Native_Client) (NaCl)
project. ZeroVM creates a secure and isolated execution environment which can
run a single thread or application. But NaCl is
[no longer maintained](https://bugs.chromium.org/p/chromium/issues/detail?id=239656#c160)
and ZeroVM has some severe limitations, so, it won't be used.

## Konnector isolation study

The short list of tools which will be tested to isolate connectors is Rkt and
NsJail which on paper better fullfill our needs.

### NsJail

NsJail is a lightweight process isolation tool, making use of Linux namespaces
and seccomp-bpf syscall filters. It is not a container tool like docker. Its
features are quite extensive regarding isolation. The
[README](https://github.com/google/nsjail) gives the full list of available
options. Although available in the google github, it is not an official google
tool.

NsJail is:

-   easy to install : just a make away with standard build tools
-   offers a full list or isolation tools
-   lightly documented the only documentation is nsjail -h (also available in
    the main github page) and it is quite cryptic for a non-sysadmin like me. I
    could not find any help in any search engine. Some examples are available to
    run a back in an isolated process and work but I could not run a full nodejs
    (only nodejs -v worked)
-   The konnectors will need a full nodejs installed on the host
-   Is still actively maintained

### Rkt

Rkt is very similar to docker. It can even directly run docker images from the
docker registry, which gives us a lot of existing images to use, even if we want
to be able to use other languages than node. For example, we could have also a
container dedicated to weboob, another container could use phantomjs or casper
and without forcing self-hosted users to do complicated installation procedures.

Rkt is :

-   easy to install : debian, rpm package available, archlinux community package
    : https://github.com/coreos/rkt/releases
-   has network isolation like docker
-   offers CPU, memory limitation, seccomp isolation (but the set of rules to
    use is out of my understanding)
-   is well [documented](https://coreos.com/rkt/docs/latest/), complete man
    pages, but not as well known as docker, then there is not a lot of things to
    find outside the official documentation.
-   can use docker image directly or can convert them to one runnable aci file
    with one simple cli command (rkt export)
-   is in active developpement but relatively stable regarding core features.
-   container images can be easily signed and the signature is checked by
    default when running a container.

I managed to run a nodejs container with just the following commands :

    rkt run --insecure-options=image --interactive docker://node:slim --name nodeslim -- -v
    rkt list   # to get the container uuid
    rkt export --app=nodeslim <uuid> nodeslim.aci
    rkt run --insecure-options=image --interactive nodeslim.aci -- -v  # to run node -v in the new container

Note: the --insecure-options param is to avoid the check of the image signature
to ease the demonstration

### Choice

The best choice would be Rkt for it's ease of use (which is good for
contribution) and wide range of isolation features + access to the big docker
ecosystem without beeing a burden for the host administrator. Note : the
limitation of NsJail I saw might be due to my lack of knowledge regarding system
administration.

### Proposed use of rkt regarding connectors

#### Installation

As stated before, rkt is easy to install. It may also be possible to make it
available in the cozy-stack docker image but I did not test it (TODO)

#### Image creation

To create an ACI file image, you just need to run a docker image one time :

    rkt run  --uuid-file-save=$PWD/uuid --insecure-options=image --interactive docker://node:slim --name nodeslim -- -v
    rkt export --app=nodeslim `cat uuid` nodeslim.aci && rm uuid

The node:slim image weights 84M at this time. The node:alpine image also exists
and is way lighter (19M) but I had problems with DNS with this, and alpine can
cause some nasty bugs that are difficult to track.

#### Running a connector

A path dedicated to run the konnectors with a predefined list of node packages
available (the net module could be mocked with special limitations to blacklist
some urls)

A script will run the node container giving as option the script to launch. The
path is mounted inside the container. The following script does just that

    #!/usr/bin/env bash
    rm -rf ./container_dir
    cp -r ./container_dir_template ./container_dir
    rkt run --net=host --environment=CREDENTIAL=value;COZY_URL=url --uuid-file-save=$PWD/uuid --volume data,kind=host,source=$PWD/container_dir --insecure-options=image nodeslim.aci --cpu=100m --memory=128M --name rktnode --mount volume=data,target=/usr/src/app --exec node -- /usr/src/app/$1 $2 &
    # the container will handle itself the communication with the stack
    sleep 60
    rkt stop --force --uuid-file=uuid
    rkt rm --uuid-file=uuid
    rm -rf ./container_dir

This script can be run like this :

    ./rkt.sh mynewkonnector.js

If the mynewkonnector.js file is available in the container_dir_template
directory.

Cons : must forbid access to port 5984 and 6060 + SMTP server

The limitation of time, CPU and memory will avoid most DOS attacks (to my
knowledge). For memory use, I still don't see a way to prevent the excessive use
of swap from the container. To prevent the connectors from listening to each
other, they should be run in containers with different uid, avoiding them to
listen to each other.

#### Solution to limit access of the container to 5984 and 6060 ports + SMTP

The container must be started in bridged mode. With that, the container still
has access to localhost but through a specific IP address visible with ifconfig.
That way, the host can have iptable rules to forbid access to specified ports to
the bridge.

To connect a container in bridge mode :

On the host create the file /etc/rkt/net.d/10-containers.conf

    {
        "name": "bridge",
        "type": "bridge",
        "bridge": "rkt-bridge-nat",
        "ipMasq": true,
        "isGateway": true,
        "ipam": {
            "type": "host-local",
            "subnet": "10.2.0.0/24",
            "routes": [
                   { "dst": "0.0.0.0/0" }
            ]
        }
    }

and run your container with the "--net=bridge" option. That way, a new interface
is available in the container and gives you access to the host.

## Konnector install and run details

### Install

The konnectors will be installed in the .cozy_konnectors directory which is in
the VFS using git clone (like the apps at the moment).

The konnectors installation may be triggered when the user says he wants to use
it. The resulting repository is then kept for each run of the konnector. It may
then be given to the user the possibility to upgrade the konnector to the latest
version if any.

To update a given konnector, a `git pull` command is run on the konnector.

### Details about running a konnector

To run a given konnector, the stack will copy this connector in a "run"
directory, which is not in the VFS. This directory will be given to the rocket
container as the current working directory with full read and write access on
it. This is where the container will put its logs and any temp file needed.
There will be also cozy-client.js and the shared libraries in a lib directory
inside this directory. The lib directory will be the content of the
[actual server lib directory](https://github.com/cozy-labs/konnectors/tree/master/server/lib).

The konnector will be run with the following environment variables :

-   `COZY_CREDENTIALS` : containing the response to Oauth request as json string
-   `COZY_URL` : to know what instance is running the konnector
-   `COZY_FIELDS` : as a json string with all the values from the account
    associated to the konnector.
-   `COZY_PARAMETERS` : optional json string associated with the application,
    used to parameterize a konnector based on a common set of code.

In the end of the konnector execution (or timeout), the logs are read in the
log.txt file and added to the konnector own log file (in VFS) and the run
directory is then destroyed.

## Multi-account handling

This section is devoted to allow the user to use one account for multiple
konnectors. It will follow the following constraints in mind:

-   The migration path must be as easy as possible
-   The developpement and maintainance of konnector must also be as easy as
    possible

### New doctype : io.cozy.accounts

A new doctype will have to be created to allow to keep konnector accounts
independently from each konnector. The one once used by the email application
seems to be a good candidate : io.cozy.accounts

Here is an example document with this doctype :

```
{
    _id: "ojpiojpoij",
    name: "user decided name for the account",
    accountType: "google",
    login: "mylogin",
    password: "123456"
}
```

Any attribute needed for the account may be added : email, etc...

### Updates needed in existing application and konnectors

CRUD manipulation of io.cozy.accounts and linking them with konnectors will be
handled by the "my accounts" client application.

Each konnector need also to declare a new field in the "fields" attribute which
will be the type of account, related to the accountType field in the new account
docType.

Ex:

```
module.exports = baseKonnector.createNew({
  name: 'Trainline',
  vendorLink: 'www.captaintrain.com',
  category: 'transport',
  color: {
    hex: '#48D5B5',
    css: '#48D5B5'
  },
  fields: {
    login: {
      type: 'text'
    },
    password: {
      type: 'password'
    },
    folderPath: {
      type: 'folder',
      advanced: true
    },
    accountType: "trainline"
  },
  dataType: ['bill'],
  models: [Bill],
  fetchOperations: [
    ...
  ]
})
```

With this new field, which will appear also in the io.cozy.konnectors docType,
the "my account" client appliction will be able to propose existing accounts of
the good type for activating a new konnector.

### Migration path

For the migration of existing, activated konnectors in V2, the type of account
for each konnector will have to be indicated in a V2 "my account" application
update. After that, it will be possible to create the accounts associated to
each activated konnectors an link the konnectors to these accounts in a
migration script.

## Study on konnectors installation on VFS

The VFS is slow and installing npm packages on it will cause some performance
problem. We are trying to find solution to handle that.

We found 3 possible solutions :

-   Install the konnector on VFS as tar.gz files with all the dependencies
    included by the konnector developper
    -   advantages : easy for the konnector developper, as performant as a cp,
        no nedd for a compiled version of the konnector source, no duplication
        of code in the repo
    -   drawbacks : The source are not readable in files application then more
        complicated to study the konnector source, not really nice... , still
        could take a lot of space on VFS
-   Use webpack with `target: node` option to make a node bundle of the
    dependencies
    -   advantages : the konnector itself stays in clear on the VFS
    -   drawbacks : forces a compilation of the sources and then a sync between
        the source and bundle in the git repo by the konnector developper,
        forces konnector developpers to use webpack.
-   Install the npm dependencies with yarn in an immutable cache (--cache-folder
    option) in a directory like deps-${konnector-git-sha1} not in VFS.
    -   advantages : easier for the konnector developper, no particular
        dependency handling, no mandatory compilation, just a package.json in
        the git repository, the cache can be shared by instances
    -   drawbacks : node only solution, maybe more work on the cozy-stack side

## TODO

-   [x] How to install and update the konnectors?
-   [x] Are the konnectors installed once per server or per instance (in the VFS
        like client-side apps)?
-   [x] One git repository with all the konnectors (like now), or one repos per
        konnector? Same question for package.json
-   [ ] What API to list the konnectors for My Accounts?
-   [ ] What workflow for developing a konnector?
-   [ ] How to test konnectors?
-   [x] How are managed the locales? : declared in manfiest.konnector
-   [x] Which version of nodejs? Last LTS version bundled in a rocket container
-   [ ] Do you keep coffeescript? Or move every konnector to ES2017? _ 28
        konnectors in coffee _ 22 konnectors in JS
-   [ ] What about weboob?
-   [ ] What roadmap for transforming the konnectors-v2 in konnectors-v3?
-   [x] What format for the konnectors manifest?
-   [x] What permissions for a konnector?
-   [ ] For konnectors that import files, how can we let the user select a
        folder and have an associated permission for the konnector in this
        folder (and not anywhere else on the virtual file system)?
-   [ ] Can we associate the data retrieved by a konnector to a "profile"? The
        goal is to allow a client-side to have a permission on this profile and
        be able to read all the data fetched by a given konnector (or is tied to
        an account)?
-   [ ] How are logged the data exported/synchronized by a "push" konnector?
-   [x] Analyze the konnectors node_modules _ no compiled modules currently _ 28
        dependencies that install 65 MB for 271 modules in production \* 71
        dependencies that install 611 MB for 858 modules with dev dependencies
-   [ ] How are persisted the accounts?
-   [x] How is executed a konnector? In particular, how the credentials are
        given to the konnector?
-   [ ] what should expose a konnector (data, functions, etc)? \*
        https://github.com/cozy-labs/konnectors/issues/695
-   [ ] How can we support konnectors with OAuth?
