[Table of contents](README.md#table-of-contents)

# Konnectors

## What we want?

[Konnectors](https://github.com/cozy-labs/konnectors) is an application for
Cozy v2 that fetch data from different web sites and services, and save them
into a Cozy. The 50+ connectors represent a lot of work from the community.
So, we want to port it to Cozy v3. There will be 2 parts:

- My Accounts, a client-side app, that will offer the possibility for the user
  to configure her accounts, and choose when to start the import of data
  (see [the architecture
  doc](https://github.com/cozy-labs/konnectors/blob/development/docs/client-side-architecture.md)).
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
this path and isolate the konnectors inside docker.

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

## Konnector isolation study

The short list of tools which will be tested to isolate connectors is Rkt and NsJail which on paper better fullfill our needs.

### NsJail

NsJail is a lightweight process isolation tool, making use of Linux namespaces and seccomp-bpf syscall filters. It is not a container tool like docker. Its features are quiet extensive regarding isolation. The [README](https://github.com/google/nsjail) gives the full list of options available. Although available in the google github, it is not an official google tool.

NsJail is:

- easy to install : juste a make away with standard build tools
- offers a full list or isolation tools
- lightly documented the only documentation is nsjail -h (also available in the main github page) and it is quiet cryptic for a non-sysadmin         
like me. I could not find any help in any search engine. Some examples are available to run a back in an isolated process and work but I could not run a full nodejs (only nodejs -v worked)
- The konnectors will need a full nodejs installed on the host
- Is still actively maintained

### Rkt

Rkt is very similar to docker. It can even directly run docker images from the docker registry, which gives us a lot of existing images to use, even if we want to be able to use other languages than node. For example, we could have also a container dedicated to weboob, another container could use phantomjs or casper and without forcing self-hosted users to do complicated installation procedures.

Rkt is :
- easy to install : debian, rpm package available, archlinux community package : https://github.com/coreos/rkt/releases
- has network isolation like docker
- offers CPU, memory limitation, seccomp isolation (but the set of rules to use is out of my understanding)
- is well [documented](https://coreos.com/rkt/docs/latest/), complete man pages, but not as well known as docker, then there is not a lot of things to find outside the official documentation.
- can use docker image directly or can convert them to one runnable aci file with one simple cli command (rkt export)
- is in active developpement but relatively stable regarding core features.
- container images can be easily signed and the signature is checked by default when running a container.

I managed to run a nodejs container with just the following commands :

    rkt run --insecure-options=image --interactive docker://node:slim --name nodeslim -- -v
    rkt list   # to get the container uuid
    rkt export --app=nodeslim <uuid> nodeslim.aci
    rkt run --insecure-options=image --interactive nodeslim.aci -- -v  # to run node -v in the new container
    
Note: the --insecure-options param is to avoid the check of the image signature to ease the demonstration

### Choice

The best choice would be Rkt for it's ease of use (which is good for contribution) and wide range of isolation features + access to the big docker echosystem without beeing a burden for the host administrator.
Note : the limitation of NsJail I saw might be due to my lack of knowledge regarding system administration.

### Proposed use of rkt regarding connectors

#### Installation

As stated before, rkt is easy to install. It may also be possible to make it available in the cozy-stack docker image but I did not test it (TODO)

#### Image creation

To create an ACI file image, you just need to run a docker image one time :
    
    rkt run  --uuid-file-save=$PWD/uuid --insecure-options=image --interactive docker://node:slim --name nodeslim -- -v
    rkt export --app=nodeslim `cat uuid` nodeslim.aci && rm uuid


The node:slim image weights 84M at this time. The node:alpine image also exists and is way lighter (19M) but I had problems with DNS with this, which could be solved with more time.

#### Running a connector

A path dedicated to run the konnectors with a predefined list of node packages available (the net module could be mocked with special limitations to blacklist some urls)

A script will run the node container giving as option the script to launch. The path is mounted inside the container. The following script does just that

    #!/usr/bin/env bash
    rm -rf ./container_dir
    cp -r ./container_dir_template ./container_dir
    rkt run --uuid-file-save=$PWD/uuid --volume data,kind=host,source=$PWD/container_dir --insecure-options=image nodeslim.aci --cpu=100m --memory=128M --name rktnode --mount volume=data,target=/usr/src/app --exec node -- /usr/src/app/$1 &
    sleep 60   # 1 min seems enough to run a connector
    rkt stop --force --uuid-file=uuid
    rkt rm --uuid-file=uuid
    # read needed information in the container_dir directory : new documents, logs, etc
    rm -rf ./container_dir

This script can be run like this :

    ./rkt.sh mynewkonnector.js

If the mynewkonnector.js file is available in the container_dir_template directory.

Pros : no breach regarding localhost network, more secure
Cons: a file communication protocol with the stack need to be defined (any other idea ?), it may be more difficult to migrate the existing connectors.

If we choose to give access to the host network to the container (but with a limited mocked net module maybe), the solution will be slightly modified : 

    #!/usr/bin/env bash
    rm -rf ./container_dir
    cp -r ./container_dir_template ./container_dir
    rkt run --net=host --uuid-file-save=$PWD/uuid --volume data,kind=host,source=$PWD/container_dir --insecure-options=image nodeslim.aci --cpu=100m --memory=128M --name rktnode --mount volume=data,target=/usr/src/app --exec node -- /usr/src/app/$1 $2 &
    # the container will handle itself the communication with the stack
    sleep 60
    rkt stop --force --uuid-file=uuid
    rkt rm --uuid-file=uuid
    rm -rf ./container_dir

This script can be run like this :

    ./rkt.sh mynewkonnector.js COZY_STACK_CREDENTIALS

If the mynewkonnector.js file is available in the container_dir_template directory.

Pros: the container can directly the stack to make its own updates, like before-> less work on migrating konnectors
Cons : must forbid acces to port 5984 and 6060 + SMTP server

For both solutions, the limitation of time, CPU and memory will avoid most DOS attacks (to my knowledge). For memory use, I still don't see a way to prevent the excessive use of swap from the container.
To prevent the connectors from listening to each other, they should be run one by one in their own container and not at the same time.

## TODO

- [ ] How to install and update the konnectors?
- [ ] Are the konnectors installed once per server or per instance (in the VFS
  like client-side apps)?
- [ ] One git repository with all the konnectors (like now), or one repos per
  konnector? Same question for package.json
- [ ] What API to list the konnectors for My Accounts?
- [ ] What workflow for developing a konnector?
- [ ] How to test konnectors?
- [ ] How are managed the locales?
- [ ] Which version of nodejs?
- [ ] Do you keep coffeescript? Or move every konnector to ES2017?
  - 28 konnectors in coffee
  - 22 konnectors in JS
- [ ] What about weboob?
- [ ] What roadmap for transforming the konnectors-v2 in konnectors-v3?
- [ ] What format for the konnectors manifest?
- [ ] What permissions for a konnector?
- [ ] For konnectors that import files, how can we let the user select a
  folder and have an associated permission for the konnector in this folder
  (and not anywhere else on the virtual file system)?
- [ ] Can we associate the data retrieved by a konnector to a "profile"? The
  goal is to allow a client-side to have a permission on this profile and be
  able to read all the data fetched by a given konnector (or is tied to an
  account)?
- [ ] How are logged the data exported/synchronized by a "push" konnector?
- [X] Analyze the konnectors node_modules
    - no compiled modules currently
    - 28 dependencies that install 65 MB for 271 modules in production
    - 71 dependencies that install 611 MB for 858 modules with dev dependencies
- [ ] How are persisted the accounts?
- [ ] How is executed a konnector? In particular, how the credentials are
  given to the konnector?
- [ ] what should expose a konnector (data, functions, etc)?
    - https://github.com/cozy-labs/konnectors/issues/695
- [ ] How can we support konnectors with OAuth?
