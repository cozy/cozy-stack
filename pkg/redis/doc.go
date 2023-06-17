/*
Package redis provides an universal redis client.

This universal client is an abstraction around three
different client setup.

You can find a complete explanation on this [article](https://medium.com/hepsiburadatech/redis-solutions-standalone-vs-sentinel-vs-cluster-f46e703307a9)
or an quick explanation below.

# The Single Node client

This client is setup when you precise only one url inside the "addr" field and
nothing inside the "master" field.

This client is the easiest, you have only one redis node and you exchange with it.
There is no replication and no failover.

# The Failover client

This client is setup when you precise the "master" field, only a single url is required
inside the "addrs" field.

This client talk to a single node, the master. This master will replicate the data to slaves
which can take read-only, no up-to-date queries. If the master fail, one slave will be promoted
as new master.

# The Cluster client

This client is setup when you have several urls inside the "addrs" field and nothing inside the
"master" field.

This client is a multi master cluster where the data is automatically partitionned. Each master has
the same failover than the Failover client.
*/
package redis
