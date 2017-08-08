Datastore implementation internals
==================================

This document contains internal implementation details for this in-memory
version of datastore. It's mostly helpful to understand what's going on in this
implementation, but it also can reveal some insight into how the real appengine
datastore works (though note that the specific encodings are different).

Additionally note that this implementation cheats by moving some of the Key
bytes into the table (collection) names (like the namespace, property name for
the builtin indexes, etc.). The real implementation contains these bytes in the
table row keys, I think.


Internal datastore key/value collection schema
----------------------------------------------

The datastore implementation here uses several different tables ('collections')
to manage state for the data. The schema for these tables is enumerated below
to make the code a bit easier to reason about.

All datastore user objects (Keys, Properties, PropertyMaps, etc.) are serialized
using `go.chromium.org/gae/service/datastore/serialize`, which in turn uses the
primitives available in `go.chromium.org/luci/common/cmpbin`. The encodings
are important to understanding why the schemas below sort correctly when
compared only using `bytes.Compare` (aka `memcmp`). This doc will assume that
you're familiar with those encodings, but will point out where we diverge from
the stock encodings.

All encoded Property values used in memory store Keys (i.e. index rows) are
serialized using the settings `serialize.WithoutContext`, and
`datastore.ShouldIndex`.

### Primary table

The primary table maps datastore keys to entities.

- Name: `"ents:" + namespace`
- Key: serialized datastore.Property containing the entity's datastore.Key
- Value: serialized datastore.PropertyMap

This table also encodes values for the following special keys:

- Every entity root (e.g. a Key with nil Parent()) with key K has:
  - `Key("__entity_group__", 1, K)` -> `{"__version__": PTInt}`
    A child entity with the kind `__entity_group__` and an id of `1`. The value
    has a single property `__version__`, which contains the version number of
    this entity group. This is used to detect transaction conflicts.
  - `Key("__entity_group_ids__", 1, K)` -> `{"__version__": PTInt}`
    A child entity with the kind `__entity_group__` and an id of `1`. The value
    has a single property `__version__`, which contains the last automatically
    allocated entity ID for entities within this entity group.
- A root entity with the key `Key("__entity_group_ids__",1)` which contains the
  same `__version__` property, and indicates the last automatically allocated
  entity ID for root entities.

### Compound Index table

The next table keeps track of all the user-added 'compound' index descriptions
(not the content for the indexes). There is a row in this table for each
compound index that the user adds by calling `ds.Raw().Testable().AddIndexes`.

- Name: `"idx"`
- Key: normalized, serialized `datastore.IndexDefinition` with the SortBy slice
  in reverse order (i.e. `datastore.IndexDefinition.PrepForIdxTable()`).
- Value: empty

The Key format here requires some special attention. Say you started with
a compound IndexDefinition of:

    IndexDefinition{
      Kind: "Foo",
      Ancestor: true,
      SortBy: []IndexColumn{
        {Property: "Something", Direction: DESCENDING},
        {Property: "Else", Direction: ASCENDING},
        {Property: "Cool", Direction: ASCENDING},
      }
    }

After prepping it for the table, it would be equivalent to:

    IndexDefinition{
      Kind: "Foo",
      Ancestor: true,
      SortBy: []IndexColumn{
        {Property: "__key__", Direction: ASCENDING},
        {Property: "Cool", Direction: ASCENDING},
        {Property: "Else", Direction: ASCENDING},
        {Property: "Something", Direction: DESCENDING},
      }
    }

The reason for doing this will be covered in the `Query Planning` section, but
it boils down to allowing the query planner to use this encoded table to
intelligently scan for potentially useful compound indexes.

### Index Tables

Every index (both builtin and compound) has one index table per namespace, which
contains as rows every entry in the index, one per row.

- Name: `"idx:" + namespace + IndexDefinition.PrepForIdxTable()`
- Key: concatenated datastore.Property values, one per SortBy column in the
  IndexDefinition (the non-PrepForIdxTable version). If the SortBy column is
  DESCENDING, the serialized Property is inverted (e.g. XOR 0xFF).
- Value: empty

If the IndexDefinition has `Ancestor: true`, then the very first column of the
Key contains the partial Key for the entity. So if an entity has the datastore
key `/Some,1/Thing,2/Else,3`, it would have the values `/Some,1`,
`/Some,1/Thing,2`, and `/Some,1/Thing,2/Else,3` as value in the ancestor column
of indexes that it matches.

#### Builtin (automatic) indexes

The following indexes are automatically created for some entity with a key
`/Kind,*`, for every property (with `ShouldIndex` values) named "Foo":

    IndexDefinition{ Kind: "Kind", Ancestor: false, SortBy: []IndexColumn{
      {Property: "__key__", Direction: ASCENDING},
    }}
    IndexDefinition{ Kind: "Kind", Ancestor: false, SortBy: []IndexColumn{
      {Property: "Foo", Direction: ASCENDING},
      {Property: "__key__", Direction: ASCENDING},
    }}
    IndexDefinition{ Kind: "Kind", Ancestor: false, SortBy: []IndexColumn{
      {Property: "Foo", Direction: DESCENDING},
      {Property: "__key__", Direction: ASCENDING},
    }}

Index updates
-------------

(Note that this is a LARGE departure from how the production appengine datastore
does this. This model only works because the implementation is not distributed,
and not journaled. The real datastore does index updates in parallel and is
generally pretty fancy compared to this).

Index updates are pretty straightforward. On a mutation to the primary entity
table, we take the old entity value (remember that entity values are
PropertyMaps), the new property value, create index entries for both, merge
them, and apply the deltas to the affected index tables (i.e. entries that
exist in the old entities, but not the new ones, are deleted. Entries that exist
in the new entities, but not the old ones, are added).

Index generation works (given an slice of indexes []Idxs) by:

* serializing all ShouldIndex Properties in the PropertyMap to get a
  `map[name][]serializedProperty`.
* for each index idx
  * if idx's columns contain properties that are not in the map, skip idx
  * make a `[][]serializedProperty`, where each serializedProperty slice
    corresponds with the IndexColumn of idx.SortBy
    * duplicated values for multi-valued properties are skipped.
  * generate a `[]byte` row which is the contatenation of one value from each
    `[]serializedProperty`, permuting through all combinations. If the SortBy
    column is DESCENDING, make sure to invert (XOR 0xFF) the serializedProperty
    value!.
  * add that generated []byte row to the index's corresponding table.

Note that we choose to serialize all permutations of the saved entity. This is
so that we can use repeated-column indexes to fill queries which use a subset of
the columns. E.g. if we have the index `duck,duck,duck,goose`, we can
theoretically use it to fill a query for `duck=1,duck=2,goose>"canadian"`, by
pasting 1 or 2 as the value for the 3rd `duck` column. This simplifies index
selection at the expense of larger indexes. However, it means that if you have
the entity:

    duck = 1, 2, 3, 4
    goose = "færøske"

It generates the following index entries:

    duck=1,duck=1,duck=1,goose="færøske"
    duck=1,duck=1,duck=2,goose="færøske"
    duck=1,duck=1,duck=3,goose="færøske"
    duck=1,duck=1,duck=4,goose="færøske"
    duck=1,duck=2,duck=1,goose="færøske"
    duck=1,duck=2,duck=2,goose="færøske"
    duck=1,duck=2,duck=3,goose="færøske"
    duck=1,duck=2,duck=4,goose="færøske"
    duck=1,duck=3,duck=1,goose="færøske"
    ... a lot ...
    duck=4,duck=4,duck=4,goose="færøske"

This is a very large number of index rows (i.e. an 'exploding index')!

An alternate design would be to only generate unique permutations of elements
where the index has repeated columns of a single property. This only makes sense
because it's illegal to have an equality and an inequality on the same property,
under the current constraints of appengine (though not completely ridiculous in
general, if inequality constraints meant the same thing as equality constraints.
However it would lead to a multi-dimensional query, which can be quite slow and
is very difficult to scale without application knowledge). If we do this, it
also means that we need to SORT the equality filter values when generating the
prefix (so that the least-valued equality constraint is first). If we did this,
then the generated index rows for the above entity would be:

    duck=1,duck=2,duck=3,goose="færøske"
    duck=1,duck=2,duck=4,goose="færøske"
    duck=1,duck=3,duck=4,goose="færøske"
    duck=2,duck=3,duck=4,goose="færøske"

Which be a LOT more compact. It may be worth implementing this restriction
later, simply for the memory savings when indexing multi-valued properties.

If this technique is used, there's also room to unambiguously index entities
with repeated equivalent values. E.g. if duck=1,1,2,3,4 , then you could see
a row in the index like:

    duck=1,duck=1,duck=2,goose="færøske"

Which would allow you to query for "an entity which has duck values equal to 1,
1 and 2". Currently such a query is not possible to execute (it would be
equivalent to "an entity which has duck values equal to 1 and 2").

Query planning
--------------

Now that we have all of our data tabulated, let's plan some queries. The
high-level algorithm works like this:

* Generate a suffix format from the user's query which looks like:
  * orders (including the inequality as the first order, if any)
  * projected fields which aren't explicitly referenced in the orders (we
    assume ASCENDING order for them), in the order that they were projected.
  * `__key__` (implied ascending, unless the query's last sort order is for
    `__key__`, in which case it's whatever order the user specified)
* Reverse the order of this suffix format, and serialize it into an
  IndexDefinition, along with the query's Kind and Ancestor values. This
  does what PrepForIdxTable did when we added the Index in the first place.
* Use this serialized reversed index to find compound indexes which might
  match by looking up rows in the "idx" table which begin with this serialized
  reversed index.
* Generate every builtin index for the inequality + equality filter
  properties, and see if they match too.

An index is a potential match if its suffix *exactly* matches the suffix format,
and it contains *only* sort orders which appear in the query (e.g. the index
contains a column which doesn't appear as an equality or inequlity filter).

The index search continues until:

* We find at least one matching index; AND
* The combination of all matching indexes accounts for every equality filter
  at least once.

If we fail to find sufficient indexes to fulfill the query, we generate an index
description that *could* be sufficient by concatenating all missing equality
filters, in ascending order, followed by concatenating the suffix format that we
generated for this query. We then suggest this new index to the user for them to
add by returing an error containing the generated IndexDefinition. Note that
this index is not REQUIRED for the user to add; they could choose to add bits
and pieces of it, extend existing indexes in order to cover the missing columns,
invert the direction of some of the equality columns, etc.

Recall that equality filters are expressed as
`map[propName][]serializedProperty`. We'll refer to this mapping as the
'constraint' mapping below.

To actually come up with the final index selection, we sort all the matching
indexes from greatest number of columns to least. We add the 0th index (the one
with the greatest number of columns) unconditionally. We then keep adding indexes
which contain one or more of the remaining constraints, until we have no
more constraints to satisfy.

Adding an index entails determining which columns in that index correspond to
equality columns, and which ones correspond to inequality/order/projection
columns. Recall that the inequality/order/projection columns are all the same
for all of the potential indices (i.e. they all share the same *suffix format*).
We can use this to just iterate over the index's SortBy columns which we'll use
for equality filters. For each equality column, we remove a corresponding value
from the constraints map. In the event that we _run out_ of constraints for a
given column, we simply _pick an arbitrary value_ from the original equality
filter mapping and use that. This is valid to do because, after all, they're
equality filters.

Note that for compound indexes, the ancestor key counts as an equality filter,
and if the compound index has `Ancestor: true`, then we implicitly put the
ancestor as if it were the first SortBy column. For satisfying Ancestor queries
with built-in indexes, see the next section.

Once we've got our list of constraints for this index, we concatenate them all
together to get the *prefix* for this index. When iterating over this index, we
only ever want to see index rows whose prefix exactly matches this. Unlike the
suffix formt, the prefix is per-index (remember that ALL indexes in the
query must have the same suffix format).

Finally, we set the 'start' and 'end' values for all chosen indexes to either
the Start and End cursors, or the Greater-Than and Less-Than values for the
inequality. The Cursors contain values for every suffix column, and the
inequality only contains a value for the first suffix column. If both cursors
and an inequality are specified, we take the smaller set of both (the
combination which will return the fewest rows).

That's about it for index selection! See Query Execution for how we actually use
the selected indexes to run a query.

### Ancestor queries and Built-in indexes

You may have noticed that the built-in indexes can be used for Ancestor queries
with equality filters, but they don't start with the magic Ancestor column!

There's a trick that you can do if the suffix format for the query is just
`__key__` though (e.g. the query only contains equality filters, and/or an
inequality filter on `__key__`). You can serialize the datastore key that you're
planning to use for the Ancestor query, then chop off the termintating null byte
from the encoding, and then use this as additional prefix bytes for this index.
So if the builtin for the "Val" property has the column format of:

    {Property: "Val"}, {Property: "__key__"}

And your query holds Val as an equality filter, you can serialize the
ancestor key (say `/Kind,1`), and add those bytes to the prefix. So if you had
an index row:

    PTInt ++ 100 ++ PTKey ++ "Kind" ++ 1 ++ CONTINUE ++ "Child" ++ 2 ++ STOP

(where CONTINUE is the byte 0x01, and STOP is 0x00), you can form a prefix for
the query `Query("Kind").Ancestor(Key(Kind, 1)).Filter("Val =", 100)` as:

    PTInt ++ 100 ++ PTKey ++ "Kind" ++ 1

Omitting the STOP which normally terminates the Key encoding. Using this prefix
will only return index rows which are `/Kind,1` or its children.

"That's cool! Why not use this trick for compound indexes?", I hear you ask :)
Remember that this trick only works if the prefix before the `__key__` is
*entirely* composed of equality filters. Also recall that if you ONLY use
equality filters and Ancestor (and possibly an inequality on `__key__`), then
you can always satisfy the query from the built-in indexes! While you
technically could do it with a compound index, there's not really a point to
doing so. To remain faithful to the production datastore implementation, we
don't implement this trick for anything other than the built-in indexes.

### Cursor format

Cursors work by containing values for each of the columns in the suffix, in the
order and Direction specified by the suffix. In fact, cursors are just encoded
versions of the []IndexColumn used for the 'suffix format', followed by the
raw bytes of the suffix for that particular row (incremented by 1 bit).

This means that technically you can port cursors between any queries which share
precisely the same suffix format, regardless of other query options, even if the
index planner ends up choosing different indexes to use from the first query to
the second. No state is maintained in the service implementation for cursors.

I suspect that this is a more liberal version of cursors than how the production
appengine implements them, but I haven't verified one way or the other.

Query execution
---------------

Last but not least, we need to actually execute the query. After figuring out
which indexes to use with what prefixes and start/end values, we essentially
have a list of index subsets, all sorted the same way. To pull the values out,
we start by iterating the first index in the list, grabbing its suffix value,
and trying to iterate from that suffix in the second, third, fourth, etc index.

If any index iterates past that suffix, we start back at the 0th index with that
suffix, and continue to try to find a matching row. Doing this will end up
skipping large portions of all of the indexes in the list. This is the algorithm
known as "zigzag merge join", and you can find talks on it from some of the
appengine folks. It has very good algorithmic running time and tends to scale
with the number of full matches, rather than the size of all of the indexes
involved.

A hit occurs when all of the iterators have precisely the same suffix. This hit
suffix is then decoded using the suffix format information. The very last column
of the suffix will always be the datastore key. The suffix is then used to call
back to the user, according to the query type:

* keys-only queries just directly return the Key
* projection queries return the projected fields from the decoded suffix.
  Remember how we added all the projections after the orders? This is why. The
  projected values are pulled directly from the index, instead of going to the
  main entity table.
* normal queries pull the decoded Key from the "ents" table, and return that
  entity to the user.
