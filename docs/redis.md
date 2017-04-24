# Usage of Redis for cozy-stack


It will be important to figure out if we use a cluster of Redis or only one.
Assuming a cluster allows us to scale better in the future but for now we will focus on a single redis target for the stack.

## As a cache / store (KV)

Already implemented in https://github.com/cozy/cozy-stack/blob/master/pkg/cache/cache.go

To use with sessions / download manager


## As a simple lock

Usage ?

ref
https://redis.io/topics/distlock
(algorithm for cluster)


## As a FS lock (range+RW)

Using a Range+RW lock is an optimization, it would be acceptable to use a simple lock instead to start.

Sortedset & lexical sort can be used to implement a simple range-lock
More complex logic would allow to implement range+RW lock
Lua can be used to ensure


## As a jobs/trigger/event queue

sortedset by timestamp ?
