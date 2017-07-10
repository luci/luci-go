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

// Package dscache provides a transparent cache for RawDatastore which is
// backed by Memcache.
//
// Inspiration
//
// Although this is not a port of any particular implementation, it takes
// inspiration from these fine libraries:
//   - https://cloud.google.com/appengine/docs/python/ndb/
//   - https://github.com/qedus/nds
//   - https://github.com/mjibson/goon
//
// Algorithm
//
// Memcache contains cache entries for single datastore entities. The memcache
// key looks like
//
//   "gae:" | vers | ":" | shard | ":" | Base64_std_nopad(SHA1(datastore.Key))
//
// Where:
//   - vers is an ascii-hex-encoded number (currently 1).
//   - shard is a zero-based ascii-hex-encoded number (depends on shardsForKey).
//   - SHA1 has been chosen as unlikely (p == 1e-18) to collide, given dedicated
//     memcache sizes of up to 170 Exabytes (assuming an average entry size of
//     100KB including the memcache key). This is clearly overkill, but MD5
//     could start showing collisions at this probability in as small as a 26GB
//     cache (and also MD5 sucks).
//
// The memcache value is a compression byte, indicating the scheme (See
// CompressionType), followed by the encoded (and possibly compressed) value.
// Encoding is done with datastore.PropertyMap.Write(). The memcache value
// may also be the empty byte sequence, indicating that this entity is deleted.
//
// The memcache entry may also have a 'flags' value set to one of the following:
//   - 0 "entity" (cached value)
//   - 1 "lock"   (someone is mutating this entry)
//
// Algorithm - Put and Delete
//
// On a Put (or Delete), an empty value is unconditionally written to
// memcache with a LockTimeSeconds expiration (default 31 seconds), and
// a memcache flag value of 0x1 (indicating that it's a put-locked key). The
// random value is to preclude Get operations from believing that they possess
// the lock.
//
// NOTE: If this memcache Set fails, it's a HARD ERROR. See DANGER ZONE.
//
// The datastore operation will then occur. Assuming success, Put will then
// unconditionally delete all of the memcache locks. At some point later, a
// Get will write its own lock, get the value from datastore, and compare and
// swap to populate the value (detailed below).
//
// Algorithm - Get
//
// On a Get, "Add" a lock for it (which only does something if there's no entry
// in memcache yet) with a nonce value. We immediately Get the memcache entries
// back (for CAS purposes later).
//
// If it doesn't exist (unlikely since we just Add'd it) or if its flag is
// "lock" and the Value != the nonce we put there, go hit the datastore without
// trying to update memcache.
//
// If its flag is "entity", decode the object and return it. If the Value is
// the empty byte sequence, return ErrNoSuchEntity.
//
// If its flag is "lock" and the Value equals the nonce, go get it from the
// datastore. If that's successful, then encode the value to bytes, and CAS
// the object to memcache. The CAS will succeed if nothing else touched the
// memcache in the meantime (like a Put, a memcache expiration/eviction, etc.).
//
// Algorithm - Transactions
//
// In a transaction, all Put memcache operations are held until the very end of
// the transaction. Right before the transaction is committed, all accumulated
// Put memcache items are unconditionally set into memcache.
//
// NOTE: If this memcache Set fails, it's a HARD ERROR. See DANGER ZONE.
//
// If the transaction is sucessfully committed (err == nil), then all the locks
// will be deleted.
//
// The assumption here is that get operations apply all outstanding
// transactions before they return data (https://cloud.google.com/appengine/docs/go/datastore/#Go_Datastore_writes_and_data_visibility),
// and so it is safe to purge all the locks if the transaction is known-good.
//
// If the transaction succeeds, but RunInTransaction returns an error (which can
// happen), or if the transaction fails, then the lock entries time out
// naturally. This will mean 31-ish seconds of direct datastore access, but it's
// the more-correct thing to do.
//
// Gets and Queries in a transaction pass right through without reading or
// writing memcache.
//
// Cache control
//
// An entity may expose the following metadata (see
// datastore.PropertyLoadSaver.GetMeta) to control the behavior of its cache.
//
//   - `gae:"$dscache.enable,<true|false>"` - whether or not this entity should
//      be cached at all. If ommitted, dscache defaults to true.
//   - `gae:"$dscache.expiration,#seconds"` - the number of seconds of
//     persistance to use when this item is cached. 0 is infinite. If omitted,
//     defaults to 0.
//
// In addition, the application may set a function shardsForKey(key) which
// returns the number of shards to use for a given datastore key. This function
// is set with the invocation of FilterRDS.
//
// Shards have the effect that all write (Put/Delete) operations clear all
// memcache entries for the given datastore entry, and all reads read (and
// possibly populate) one of the memcache entries. So if an entity has 4 shards,
// a datastore Get for it will pull from one of the 4 possible memcache keys
// at random. This is good for heavily-read, but infrequently updated, entities.
// The purpose of sharding is to alleviate hot memcache keys, as recommended by
// https://cloud.google.com/appengine/articles/best-practices-for-app-engine-memcache#distribute-load .
//
// Caveats
//
// A couple things to note that may differ from other appengine datastore
// caching libraries (like goon, nds, or ndb).
//
//   - It does NOT provide in-memory ("per-request") caching.
//   - It's INtolerant of some memcache failures, but in exchange will not return
//     inconsistent results. See DANGER ZONE for details.
//   - Queries do not interact with the cache at all.
//   - Negative lookups (e.g. ErrNoSuchEntity) are cached.
//
// DANGER ZONE
//
// As mentioned in the Put/Delete/Transactions sections above, if the memcache
// Set fails, that's a HARD ERROR. The reason for this is that otherwise in the
// event of transient memcache failures, the memcache may be permanently left in
// an inconsistent state, since there will be nothing to actually ensure that
// the bad value is flushed from memcache. As long as the Put is allowed to
// write the lock, then all will be (eventually) well, and so all other memcache
// operations are best effort.
//
// So, if memcache is DOWN, you will effectively see tons of errors in the logs,
// and all cached datastore access will be essentially degraded to a slow
// read-only state. At this point, you have essentially 3 mitigration
// strategies:
//   - wait for memcache to come back up.
//   - dynamically disable all memcache access by writing the datastore entry:
//       /dscache,1 = {"Enable": false}
//     in the default namespace. This can be done by invoking the
//     SetDynamicGlobalEnable method. This can take up to 5 minutes to take
//     effect. If you have very long-running backend requests, you may need to
//     cycle them to have it take effect. This dynamic bit is read essentially
//     once per http request (when FilteRDS is called on the context).
//   - push a new version of the application disabling the cache filter
//     by setting InstanceEnabledStatic to false in an init() function.
//
// On every dscache.FilterRDS invocation, it takes the opportunity to fetch this
// datastore value, if it hasn't been fetched in the last
// GlobalEnabledCheckInterval time (5 minutes). This equates to essentially once
// per http request, per 5 minutes, per instance.
//
// AppEngine's memcache reserves the right to evict keys at any moment. This is
// especially true for shared memcache, which is subject to pressures outside of
// your application. When eviction happens due to memory pressure,
// least-recently-used values are evicted first.
//
// https://cloud.google.com/appengine/docs/go/memcache/#Go_How_cached_data_expires
//
// Eviction presents a potential race window, as lock items that were put in
// memcache may be evicted prior to the locked operations completing (or
// failing), causing concurrent Get operations to cache bad data indefinitely.
//
// In practice, a dedicated memcache will be safe. LRU-based eviction means that
// that locks recently added will almost certainly not be evicted before their
// operations are complete, and a dedicated memcache lowers eviction pressure to
// a single application's operation. Production systems that have data integrity
// requirements are encouraged to use a dedicated memcache.
//
// Note that flusing memcache of a running application may also induce this
// race. Flushes should be performed with this concern in mind.
//
// TODO: A potential mitigation to lock eviction poisoning is to use memcache
// Statistics to identify the oldest memcache item and use that age to bound
// the lifetime of cached datastore entries. This would cause dscache items
// created around the time of a flush to expire quickly (instead of never),
// bounding the period of time when poisoned data may reside in the cache.
package dscache
