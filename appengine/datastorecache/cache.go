// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastorecache

import (
	"math"
	"strings"
	"time"

	"github.com/luci/luci-go/appengine/memlock"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/gae/impl/prod/constraints"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"

	"golang.org/x/net/context"
)

const (
	initialLoadLockRetries = 3
	initialLoadDelay       = 10 * time.Millisecond

	// expireFactor determines when a cache entry is considered expired. If
	// a cache entry isn't updated within expireFactor refresh intervals, it will
	// be treated as expired. In a fail-open system this is a warning. In a fail-
	// closed system, this is an error.
	expireFactor = 3

	// accessUpdateDeltaFactor is the fraction of the access update interval to
	// use for prabilistic update staggering.
	//
	// If this value is N, the delta will be (ACCESS_UPDATE_INTERVAL / N). This
	// value should always be >2. Higher values will cause more datastore writes
	// but less potential concurrent writes. Lower values will cause more
	// potential concurrent conflicts, but will require a lower datastore load.
	// It's all generally unimpactful anyway because the operation is deconflicted
	// via memlock, so a lot of concurrent writes will end up being no-ops.
	accessUpdateDeltaFactor = 20

	// DefaultCacheNamespace is the default datastore namespace for cache entries.
	DefaultCacheNamespace = "luci.datastoreCache"
)

// ErrCacheExpired is a sentinel error returned by Get when an expired cache
// entry is encountered and the cache is configured not to fail open (see
// Handler's FailOpen method).
var ErrCacheExpired = errors.New("cache entry has expired")

// Value is a cached value.
type Value struct {
	// Schema is an optional schema string that will be encoded in the cache
	// entry.
	Schema string

	// Data is the cache entry's data value.
	Data []byte

	// Description is an optional description string that will be added to the
	// datastore entry for humans.
	Description string
}

// Cache defines a generic basic datastore cache. Content that is added to
// the cache is periodically refeshed and (if unused) pruned by a supportive
// cron task.
//
// Using this cache requires a cache handler to be installed and a cron task to
// be configured to hit the handler's endpoint.
//
// The cache works as follows:
//	- The cache is scanned for an item
//	- Memlocks are used to provide best-effort deduplication of refreshes for
//	  the same entity. Inability to lock will not prevent cache operations.
//	- Upon access a entry's "Accessed" timestamp will be updated to
//	  note that it is still in use. This happens probabilistically after an
//	  "access update interval" so we don't pointlessly incur the update cost
//	  every time.
//
// The support cron task will execute periodically and maintian the cache:
//	- If a cached entry's "Accessed" timestamp falls too far behind, it will be
//	  deleted as part of a periodic cron task.
//	- If the cached entry is near expiration, the cron task will refresh the
//	  entry's data.
//
// TODO(dnj): This caching scheme intentionally lends itself to sharding. This
// would be implemented by having the maintenance cron task kick off processing
// shard tasks, each querying a subset of the cached entity keyspace, rather
// than handling the whole key space itself. To this end, some areas of code for
// this cache will be programmed to operate on shards, even though currently the
// number of shards will always equal 1 (#0).
type Cache struct {
	// Name is the name of this cache. This must be unique from other caches
	// managed by this GAE instance, and will be used to differentiate cache
	// keys from other caches sharing the same datastore.
	//
	// If Name is empty, the cache will choose a default name. It is critical
	// that either Name or Namespace be unique to this cache, or else its cache
	// entries will conflict with other cache instances.
	Name string

	// Namespace, if not empty, overrides the datastore namespace where entries
	// for this cache will be stored. If empty, DefaultCacheNamespace will be
	// used.
	Namespace string

	// AccessUpdateInterval is the amount of time after a cached entry has been
	// last marked as accessed before it should be re-marked as accessed. We do
	// this sparingly, enough that the entity is not likely to be considered
	// candidate for pruning if it's being actively used.
	//
	// Recommended interval is 1 day. The only hard requirement is that this is
	// less than the PruneInterval. If this is <= 0, cached entities will never
	// have their access times updated.
	AccessUpdateInterval time.Duration

	// PruneFactor is the number of additional AccessUpdateInterval periods old
	// that a cache entry can be before it becomes candidate for pruning.
	//
	// An entry becomes candidate for pruning after
	// [AccessUpdateInterval * (PruneFactor+1)] time has passed since the last
	// AccessUpdateInterval.
	//
	// If this is <= 0, no entities will ever be pruned. This is potentially
	// acceptable when the total number of expected cached entries is expected to
	// be low.
	PruneFactor int

	// Parallel is the number of parallel refreshes that this Handler type should
	// execute during the course of a single maintenance run.
	//
	// If this is <= 0, at most one parallel request will happen per shard.
	Parallel int

	// HandlerFunc returns a Handler implementation to use for this cache. It is
	// used both by the running application and by the manager cron to perform
	// cache operations.
	//
	// If Handler is nil, or if Handler returns nil, the cache will not be
	// accessible.
	HandlerFunc func(context.Context) Handler
}

func (cache *Cache) withNamespace(c context.Context) context.Context {
	// Put our basic sanity check here.
	if cache.Name == "" && cache.Namespace == "" {
		panic(errors.New("a Cache must specify either a Name or a Namespace"))
	}

	ns := cache.Namespace
	if ns == "" {
		ns = DefaultCacheNamespace
	}
	return info.MustNamespace(c, ns)
}

// pruneInterval calculates the prune interval. This is a function of the
// cache's AccessUpdateInterval and PruneFactor.
//
// If either AccessUpdateInterval or PruneFactor is <= 0, this will return 0,
// indicating no prune interval.
func (cache *Cache) pruneInterval() time.Duration {
	if cache.AccessUpdateInterval > 0 && cache.PruneFactor > 0 {
		return cache.AccessUpdateInterval * time.Duration(1+cache.PruneFactor)
	}
	return 0
}

func (cache *Cache) manager() *manager {
	return &manager{
		cache:          cache,
		queryBatchSize: constraints.DS().QueryBatchSize,
	}
}

// InstallCronRoute installs a handler for this Cache's management cron task
// into the supplied Router at the specified path.
//
// It is recommended to assert in the middleware that this endpoint is only
// accessible from a cron task.
func (cache *Cache) InstallCronRoute(path string, r *router.Router, base router.MiddlewareChain) {
	m := cache.manager()
	m.installCronRoute(path, r, base)
}

// Get retrieves a cached Value from the cache.
//
// If the value is not defined, the Handler's Refresh function will be used to
// obtain the value and, upon success, the Value will be added to the cache
// and returned.
//
// If the Refresh function returns an error, that error will be propagated
// as-is and returned.
func (cache *Cache) Get(c context.Context, key []byte) (Value, error) {
	// Leave any current transaction.
	c = datastore.WithoutTransaction(c)

	var h Handler
	if hf := cache.HandlerFunc; hf != nil {
		h = hf(c)
	}
	if h == nil {
		return Value{}, errors.New("unable to generate Handler")
	}

	bci := boundCacheInst{
		Cache: cache,
		h:     h,
		clientID: strings.Join([]string{
			"datastore_cache",
			info.RequestID(c),
		}, "\x00"),
	}
	return bci.get(c, key)
}

// boundCacheInst is a Cache instance bound to a specific HTTP request.
//
// It contains a generated Handler and is bound to a specific cache request.
type boundCacheInst struct {
	*Cache

	h        Handler
	clientID string
}

func (bci *boundCacheInst) get(c context.Context, key []byte) (Value, error) {
	// Enter our cache namespace. Retain the user's namespace so, in the event
	// that we have to call into the Handler, we do so with the user's setup.
	userNS := info.GetNamespace(c)
	c = bci.withNamespace(c)

	// Get our current time. We will pass this around so that all methods that
	// need to examine or assign time will use the same moment for this round.
	now := datastore.RoundTime(clock.Now(c).UTC())

	// Configure our datastore cache entry to Get.
	e := entry{
		CacheName: bci.Name,
		Key:       key,
	}

	// Install logging fields.
	c = log.SetFields(c, log.Fields{
		"namespace": bci.Namespace,
		"key":       e.keyHash(),
	})

	// Load the current cache entry.
	var (
		entryWasMissing  = false
		createCacheEntry = true
	)
	switch err := datastore.Get(c, &e); err {
	case nil:
		// We successfully retrieved the entry. Do we need to update its access
		// time?
		if ai := bci.AccessUpdateInterval; ai > 0 && shouldAttemptUpdate(ai, now, e.LastAccessed, bci.getRandFunc(c)) {
			bci.tryUpdateLastAccessed(c, &e, now)
		}

		// Check if our cache entry has expired. If it has, we may be using stale
		// data.
		if bci.checkExpired(c, &e, now) {
			// The cache entry has expired. If we are not failing open, this is a
			// hard failure.
			log.Errorf(c, "Cache entry has expired.")

			if !bci.h.FailOpen() {
				return Value{}, ErrCacheExpired
			}
		}
		return e.toValue(), nil

	case datastore.ErrNoSuchEntity:
		// No cache entry in the datastore. We will have to Refresh.
		entryWasMissing = true

	default:
		// Unexpected error from datastore.
		log.WithError(err).Errorf(c, "Failed to retrieve cache entry from datastore.")

		if !bci.h.FailOpen() {
			// Not failing open, so propagate this error.
			return Value{}, errors.Annotate(err).Err()
		}

		// We are failing open, so log the error. We will use Refresh to reload the
		// cache entry, but will refrain from actually committing it, since this is
		// a failure mode.
		createCacheEntry = false
		entryWasMissing = true
	}

	// We've chosen to refresh our entry. Many other parallel processes may also
	// have chosen to do this, so let's memlock around that refresh in an attempt
	// to make this a singleton.
	//
	// We'll try a few times, sleeping a short time in between if we fail to get
	// the lock. We don't want to wait too long and stall our parent application,
	// though.
	var (
		attempts     = 0
		acquiredLock = false
		refreshValue Value
		delta        time.Duration
	)
	err := retry.Retry(clock.Tag(c, "datastoreCacheLockRetry"),
		retry.TransientOnly(bci.refreshRetryFactory), func() error {

			// This is a retry. If the entry was missing before, check to see if some
			// other process has since added it to the datastore.
			// We only check this on the 2+ retry because we already checked this
			// condition above in the initial datastore Get.
			//
			// If this fails, we'll continue to try and perform the Refresh.
			if entryWasMissing && attempts > 0 {
				switch err := datastore.Get(c, &e); err {
				case nil:
					// Successfully loaded the entry!
					refreshValue = e.toValue()
					createCacheEntry = false
					return nil

				case datastore.ErrNoSuchEntity:
					// The entry doesn't exist yet.

				default:
					log.WithError(err).Warningf(c, "Error checking datastore for cache entry.")
				}
			}
			attempts++

			// Take out a lock on this entry and Refresh it.
			err := e.tryWithLock(c, bci.clientID, func(c context.Context) (err error) {
				acquiredLock = true

				// Perform our initial refresh.
				refreshValue, delta, err = doRefresh(c, bci.h, &e, userNS)
				return
			})
			switch err {
			case nil:
				// Successfully loaded.
				return nil

			case memlock.ErrFailedToLock:
				// Retry after delay.
				return errors.WrapTransient(err)

			default:
				log.WithError(err).Warningf(c, "Unexpected failure obtaining initial load lock.")
				return err
			}
		}, func(err error, d time.Duration) {
			log.WithError(err).Debugf(c, "Failed to acquire initial refresh lock. Retrying...")
		})
	if err != nil {
		// If we actually acquired the lock, then this refresh was a failure, and
		// we will propagate the error (verbatim).
		if acquiredLock {

			return Value{}, err
		}

		log.WithError(err).Warningf(c, "Failed to cooperatively Refresh entry. Manually Refreshing.")
		if refreshValue, delta, err = doRefresh(c, bci.h, &e, userNS); err != nil {
			// Propagate the Refresh error verbatim.
			log.WithError(err).Errorf(c, "Refresh returned an error.")
			return Value{}, err
		}
	}

	// We successfully performed the initial Refresh! Do we store the entry?
	if createCacheEntry {
		e.Created = now
		e.LastRefreshed = now
		e.LastAccessed = now
		e.LastRefreshDelta = int64(delta)
		e.loadValue(refreshValue)

		switch err := datastore.Put(c, &e); err {
		case nil:
			log.Debugf(c, "Performed initial cache data Refresh for: %s", e.Description)

		default:
			log.WithError(err).Warningf(c, "Failed to Put initial cache data Refresh entry.")
		}
	}

	return refreshValue, nil
}

func (bci *boundCacheInst) checkExpired(c context.Context, e *entry, now time.Time) bool {
	// Is this entry significantly past its expiration threshold?
	if now.Sub(e.LastRefreshed) >= (expireFactor * bci.h.RefreshInterval(e.Key)) {
		log.Fields{
			"lastRefreshed": e.LastRefreshed,
		}.Warningf(c, "Stale cache entry is past its refresh interval.")
		return true
	}
	return false
}

func (bci *boundCacheInst) refreshRetryFactory() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:   initialLoadDelay,
			Retries: initialLoadLockRetries,
		},
	}
}

func (bci *boundCacheInst) tryUpdateLastAccessed(c context.Context, e *entry, now time.Time) {
	// Take out a memcache lock on this entry. We do this to prevent multiple
	// cache Gets that all examine "e" at the same time don't waste each other's
	// time spamming last accessed updates.
	err := e.tryWithLock(c, bci.clientID, func(c context.Context) error {
		e.LastAccessed = now
		return datastore.Put(c, e)
	})
	switch err {
	case nil, memlock.ErrFailedToLock:
		// If we didn't get to update it, no big deal. The next access will try
		// again now that the lock is free.

	default:
		// Something unexpected happened, so let's log it.
		log.WithError(err).Warningf(c, "Failed to update cache entry 'last accessed' time.")
	}
}

func (bci *boundCacheInst) getRandFunc(c context.Context) randFunc {
	return func() float64 { return mathrand.Float64(c) }
}

func doRefresh(c context.Context, h Handler, e *entry, userNS string) (v Value, delta time.Duration, err error) {
	// Execute the Refresh in the user's namespace.
	start := clock.Now(c)
	if v, err = h.Refresh(info.MustNamespace(c, userNS), e.Key, e.toValue()); err != nil {
		return
	}
	delta = clock.Now(c).Sub(start)
	return
}

// randFunc returns a random number between [0,1).
type randFunc func() float64

// shouldAttemptUpdate  returns true if a cache entry update should be attempted
// given the supplied parameters.
//
// We probabilistically update the "last accessed" field based on how close we
// are to the configured AccessUpdateInterval using the xFetch function.
func shouldAttemptUpdate(ai time.Duration, now, lastAccessed time.Time, rf randFunc) bool {
	return xFetch(now, lastAccessed.Add(ai), (ai / accessUpdateDeltaFactor), 1, rf)
}

// xFetch returns true if recomputation should be performed.
//
// The decision is probabilistic, based on the evaluation of a probability
// distribution that scales according to the time since the value was last
// recomputed.
//
// delta is a factor representing the amount of time that it takes to
// recalculate the value. Higher delta values allow for earlier recomputation
// for values that take longer to calculate.
//
// beta can be set to >1 to favor earlier recomputations; however, in practice
// beta=1 works well.
func xFetch(now, expiry time.Time, delta time.Duration, beta float64, rf randFunc) bool {
	offset := time.Duration(float64(delta) * beta * math.Log(rf()))
	return !now.Add(-offset).Before(expiry)
}
