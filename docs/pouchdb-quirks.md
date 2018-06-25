# Pouchdb Mango Quirks

The findings below were obtained while working on sorting performance in Cozy Drive. For reference, the final PR [can be seen here](https://github.com/cozy/cozy-drive/pull/1002/files).

## Understanding what's going on

Before diving into some of the quirks, it's important to understand some things when it comes to Pouchdb and especially Mango queries.

First, you can add a plugin called `pouchdb-debug` and enable extra logs with `PouchDB.debug.enable( "pouchdb:find" );`. This will add explanation about the queries you run in the console and it's very helpful to understand what's going on under the hood.

You will realize that Pouchdb operates in 2 phases : one part of your query may be done using indexes, and the other may be done in memory. Long story short: anything done in memory has significant performance impacts, especially as the number of items gets larger. A more detailed guide can be found [here](https://www.bennadel.com/blog/3258-understanding-the-query-plan-explained-by-the-find-plugin-in-pouchdb-6-2-0.htm).

## About indexes

Creating an index takes some time, but the first query will *also* take time — you are encouraged to warm up the indexes by firing a query that uses it before it is actually needed. An exemple implementation can be found [here](https://github.com/cozy/cozy-drive/blob/0326e3d253ca51e0fdb18a9e9b3b5c8ff0b87eba/src/drive/mobile/lib/replication.js#L15-L80).

If there is a change in the underlying documents, the index will be partially recalculated on the next query. [The post-replication callback may be a good place to warm up the index again.](https://github.com/cozy/cozy-drive/blob/0326e3d253ca51e0fdb18a9e9b3b5c8ff0b87eba/src/drive/mobile/lib/replication.js#L86-L91)

By default, Pouch will try to find the best index to use on your query. For more advanced queries, you generally want to force it with the `use_index` option. If the query and the index you force are not compatible, Pouch will emit an error and not run the query at all.

## Indexing more than one field

Creating an index on several fields is *not* the same as creating multiple indexes on one field each. The effects of a single index on multiple fields is illustrated in the [official docs](https://pouchdb.com/guides/mango-queries.html#more-than-one-field) and is important to understand.

Furthermore, the order in which fields are indexed on a multi-index is significant, most notably when it comes to sorting. If you declare an index on the fields `['name', 'age']`, you should also sort them by `['name', 'age']`. Sorting them in a different order or using other fields will likely be done in memory and kill your performance.

## Avoiding in memory selectors

Filtering results with a selector on a field that has not been indexed is almost guaranteed to be done in memory and should be avoided. Since you can't have too many indexes, some filtering may have to be done in your own code — but if you can narrow down the results beforehand, that shouldn't be a problem.

Even selectors on indexed field may end up being done in memory. Operators that don't rely on equality such as `$ne` should typically be avoided. Even equality operators run on secondary fields tend to be done in memory, and should therefor be used with care.
