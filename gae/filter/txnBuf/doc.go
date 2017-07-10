// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package txnBuf contains a transaction buffer filter for the datastore
// service.
//
// By default, datastore transactions take a snapshot of the entity group as
// soon as you Get or Put into it. All subsequent Get (and query) operations
// reflect the state of the ORIGINAL transaction snapshot, regardless of any
// Put/Delete operations you've done since the beginning of the transaction.
//
// If you've installed this transaction buffer, then:
//   - All mutations will be reflected in all read operations (see LIMITATIONS).
//     Without the buffer, read operations always observe the state of the
//     entity group(s) at the time that the transaction started.
//
//   - All mutation operations will be buffered until the close of the
//     transaction. This can help reduce the transaction size, and thus avoid
//     the transaction size limit (currently 10MB). Multiple puts to the same
//     entity will not increase the transaction size multiple times.
//
//   - Transactions inside of an existing transaction will add to their outer
//     transaction if they don't cause the outer transaction to exceed its
//     size budget.
//
//   - If an inner transaction would cause the OUTERMOST transaction to exceed
//     the appengine-imposed 10MB transaction size limit, an error will be
//     returned from the inner transaction, instead of adding it into the
//     outermost transaction. This only applies to the first level of inner
//     transactions, and does not apply to recursive transactions. The reason
//     for this is that it's entirely feasible for inner transactions to
//     temporarially exceed the limit, but still only commit an outer
//     transaction which is under the limit. An example of this would be having
//     one inner-inner transaction add a lot of large entities and then having
//     a subsequent inner-inner transaction delete some of those entities.
//
// LIMITATIONS (only inside of a transaction)
//   - KeysOnly/Projection/Count queries are supported, but may incur additional
//     costs.
//
//     These query types are implemented via projection queries, but will
//     project all order-by fields in addition to any specified in the original
//     query.
//
//   - Distinct Projection queries do all 'distinct' deduplication in-memory.
//     This could make them substantially more expensive than their native
//     equivalent.
//
//   - Metadata entities (e.g. `__entity_group__`) will reflect their values as
//     they were at the beginning of the transaction, and will not increment
//     as you write inside of the transaction.
//
//   - Query cursors are not supported. Since the cursor format for the
//     in-memory datastore implementation isn't compatible with the production
//     cursors, it would be pretty tricky to make it so that cursors were
//     viable outside the transaction as well as inside of it while also having
//     it accurately reflect the 'merged' query results.
//
//   - No parallel access* to datastore while in a transaction; all nested
//     operations are serialized. This is done for simplicity and correctness.
//
//     * The exception is that callbacks inside of
//     a Run/GetMulti/DeleteMulti/PutMulti query MAY read/write the current
//     transaction. Modifications to the datastore during query executions will
//     not affect the query results (e.g. the query has snapshot consistency
//     from the moment that it begins iteration). Note, however, that datastore
//     operations within the callback are still synchronized. This behavior is
//     so that the user is not forced to buffer all of the query results before
//     doing work with them, but can treat the query like a stream of events,
//     if they so choose.
//
//   - The changing of namespace inside of a transaction is undefined... This is
//     just generally a terrible idea anyway, but I thought it was worth
//     mentioning.
//
//   - Currently, the soft transactions are not directly accessible using the
//     CurrentTransaction interface; it returns the wrapped datastore's
//     transaction. While this is still correct, it could definitely be made
//     more useful by adding transaction buffer metadata to the returned
//     object.
package txnBuf
