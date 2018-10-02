[Table of contents](README.md#table-of-contents)

# Moving

This document is a stub to hold information about moving one's cozy from an
hosting provider to another.

We do moving by exporting data to a tarball, and then importing the tarball. The
files inside the tarball should be organized in a documented way, and with
standard formats, to open interoperability with cloud solutions.

You also have DNS and TLS certificates to change

Once we start doing some intercozy communication, we might have issues with the
transition period.

-   I export my "bob.cozycloud.cc" from host1 as a tarball
-   My friend A's cozy send me an update notification the message gets to host1
-   I trigger DNS change
-   My friend B's cozy send me something, its DNS is not up-to-date, the message
    gets to host1
-   DNS change is complete, all further messages will reach host2

The sharing protocol have to take into account the fact that a cozy can have
accepted a message, but then forget about it (better, as it also covers the
crash case).
