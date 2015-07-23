In-memory appengine wrapper
===========================


Notes on the internal encodings
-------------------------------

All datatypes inside of the index Collections of the gkvlite Store are stored
in a manner which allows them to be compared entirely via bytes.Compare. All
types are prefixed by a sortable type byte which encodes the sort-order of types
according to the appengine SDK. Additionally, types have the following data
encoding:
  * ints
    * stored with the `funnybase` varint encoding
  * floats
    * http://stereopsis.com/radix.html
    * toBytes:
        ```
        b := math.Float64bits(f)
        return b ^ (-(b >> 63) | 0x8000000000000000)
        ```
    * fromBytes:
        ```
        return math.Float64frombits(b ^ ((b >> 63) - 1) | 0x8000000000000000)
        ```
  * string, []byte, BlobKey, ByteString
    * funnybase byte count
    * raw bytes
  * \*Key, GeoPoint
    * composite of above types
  * time.Time
    * composite of above types, stored with microsecond accuracy.
      * rounding to microseconds is a limitation of the real appengine.
		* toMicro: `return t.Unix()*1e6 + int64(t.Nanosecond()/1e3)`
		* fromMicro: `return time.Unix(t/1e6, (t%1e6)*1e3)`
  * nil, true, false
    * value is encoded directly in the type byte


Gkvlite Collection schema
-------------------------

In order to provide efficient result deduplication, the value of an index row
which indexes 1 or more properties is a concatenation of the previous values
which would show up in the same index. For example, if you have the property
list for the key K:

    bob: 1
    bob: 4
    bob: 7
    cat: "hello"
    cat: "world"

And the regular (non-ancestor) composite index was {bob, -cat}, you'd have the
rows in the index `idx:ns:kind|R|bob|-cat` (| in the row indicates
concatenation, each value has an implied type byte. `...` indicates that other
rows may be present):

    ...
    1|"world"|K = nil|nil
    ...
    1|"hello"|K = nil|"world"
    ...
    4|"world"|K = 1|nil
    ...
    4|"hello"|K = 1|"world"
    ...
    7|"world"|K = 4|nil
    ...
    7|"hello"|K = 4|"world"
    ...

This allows us to, start scanning at any point and be able to determine if we've
returned a given key already (without storing all of the keys in memory
for the duration of the Query run). We can do this because we can see if the
value of an index row falls within the original query filter parameters. If it
does, then we must already have returned they Key, and can safely skip the index
row.  AFAIK, real-datastore provides deduplication by keeping all the returned
keys in memory as it runs the query, and doing a set-check.

The end-result is semantically equivalent, with the exception that Query Cursors
on the real datastore will potentially return the same Key in the first Cursor
use as well as on the 2nd (or Nth) cursor use, where this method will not.

    collections
      ents:ns                    -> key -> value
                                    (rootkind, rootid, __entity_group__,1) -> {__version__: int}
                                    (rootkind, rootid, __entity_group_ids__,1) -> {__version__: int}
                                    (__entity_group_ids__,1) -> {__version__: int}
      // TODO(iannucci): Journal every entity write in a log with a globally
      // increasing version number (aka "timestamp").
      //
      // TODO(iannucci): Use the value in idx collection to indicate the last
      // global log version reflected in this index. Then index updates can happen
      // in parallel, in a truly eventually-consistent fashion (and completely
      // avoid holding the DB writelock while calculating index entries).
      // Unfortunately, copying new records (and removing old ones) into the DB
      // would still require holding global writelock.
      //
      // TODO(iannucci): how do we garbage-collect the journal?
      //
      // TODO(iannucci): add the ability in gkvlite to 'swap' a collection with
      // another one, transactionally? Not sure if this is possible to do easily.
      // If we had this, then we could do all the index writes for a given index
      // on the side, and then do a quick swap into place with a writelock. As
      // long as every index only has a single goroutine writing it, then this
      // would enable maximum concurrency, since all indexes could update in
      // parallel and only synchronize for the duration of a single pointer swap.
      idx                        -> kind|A?|[-?prop]* = nil
      idx:ns:kind                -> key = nil
      idx:ns:kind|prop           -> propval|key = [prev val]
      idx:ns:kind|-prop          -> -propval|key = [next val]
      idx:ns:kind|A|?prop|?prop  -> A|propval|propval|key = [prev/next val]|[prev/next val]
      idx:ns:kind|?prop|?prop    -> propval|propval|key = [prev/next val]|[prev/next val]
