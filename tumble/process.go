// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tumble

import (
	"bytes"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/appengine/memlock"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/sync/parallel"

	"github.com/luci/gae/filter/txnBuf"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gae/service/info"
	mc "github.com/luci/gae/service/memcache"

	"golang.org/x/net/context"
)

const (
	// minNoWorkDelay is the minimum amount of time to sleep in between rounds if
	// there was no work done in that round.
	minNoWorkDelay = time.Second
)

// expandedShardBounds returns the boundary of the expandedShard order that
// currently corresponds to this shard number. If Shard is < 0 or > NumShards
// (the currently configured number of shards), this will return a low > high.
// Otherwise low < high.
func expandedShardBounds(c context.Context, cfg *Config, shard uint64) (low, high int64) {
	if shard < 0 || uint64(shard) >= cfg.NumShards {
		logging.Warningf(c, "Invalid shard: %d", shard)
		// return inverted bounds
		return 0, -1
	}

	expandedShardsPerShard := int64(math.MaxUint64 / cfg.NumShards)
	low = math.MinInt64 + (int64(shard) * expandedShardsPerShard)
	if uint64(shard) == cfg.NumShards-1 {
		high = math.MaxInt64
	} else {
		high = low + expandedShardsPerShard
	}
	return
}

func processShardQuery(c context.Context, cfg *Config, shard uint64) *ds.Query {
	low, high := expandedShardBounds(c, cfg, shard)
	if low > high {
		return nil
	}

	q := ds.NewQuery("tumble.Mutation").
		Gte("ExpandedShard", low).Lte("ExpandedShard", high).
		Project("TargetRoot").Distinct(true)

	batchSize := cfg.ProcessMaxBatchSize
	if batchSize > 0 {
		if batchSize > math.MaxInt32 {
			batchSize = math.MaxInt32
		}
		q = q.Limit(int32(batchSize))
	}
	return q
}

// processShard is the tumble backend endpoint. This accepts a shard number
// which is expected to be < GlobalConfig.NumShards.
func processShard(c context.Context, cfg *Config, timestamp time.Time, shard uint64, loop bool) error {

	logging.Fields{
		"shard": shard,
	}.Infof(c, "Processing tumble shard.")

	q := processShardQuery(c, cfg, shard)
	if q == nil {
		logging.Warningf(c, "dead shard, quitting")
		return nil
	}

	// Calculate our end itme. If we're not looping or we have a <= 0 duration,
	// we will perform a single loop.
	var endTime time.Time
	if cfg.ProcessLoopDuration > 0 {
		endTime = clock.Now(c).Add(time.Duration(cfg.ProcessLoopDuration))
		logging.Debugf(c, "Process loop is configured to exit after [%s] at %s",
			cfg.ProcessLoopDuration.String(), endTime)
	}

	// Lock around the shard that we are trying to modify.
	//
	// Since memcache is namespaced, we don't need to include the namespace in our
	// lock name.
	task := makeProcessTask(timestamp, endTime, shard, loop)
	lockKey := fmt.Sprintf("%s.%d.lock", baseName, shard)
	clientID := fmt.Sprintf("%d_%d_%s", timestamp.Unix(), shard, info.RequestID(c))

	err := memlock.TryWithLock(c, lockKey, clientID, func(c context.Context) error {
		return task.process(c, cfg, q)
	})
	if err == memlock.ErrFailedToLock {
		logging.Infof(c, "Couldn't obtain lock (giving up): %s", err)
		return nil
	}
	return err
}

// processTask is a stateful processing task.
type processTask struct {
	timestamp time.Time
	endTime   time.Time
	lastKey   string
	banSets   map[string]stringset.Set
	loop      bool
}

func makeProcessTask(timestamp, endTime time.Time, shard uint64, loop bool) *processTask {
	return &processTask{
		timestamp: timestamp,
		endTime:   endTime,
		lastKey:   fmt.Sprintf("%s.%d.last", baseName, shard),
		banSets:   make(map[string]stringset.Set),
		loop:      loop,
	}
}

func (t *processTask) process(c context.Context, cfg *Config, q *ds.Query) error {
	// this last key allows buffered tasks to early exit if some other shard
	// processor has already processed past this task's target timestamp.
	lastItm, err := mc.GetKey(c, t.lastKey)
	if err != nil {
		if err != mc.ErrCacheMiss {
			logging.Warningf(c, "couldn't obtain last timestamp: %s", err)
		}
	} else {
		val := lastItm.Value()
		last, err := serialize.ReadTime(bytes.NewBuffer(val))
		if err != nil {
			logging.Warningf(c, "could not decode timestamp %v: %s", val, err)
		} else {
			last = last.Add(time.Duration(cfg.TemporalRoundFactor))
			if last.After(t.timestamp) {
				logging.Infof(c, "early exit, %s > %s", last, t.timestamp)
				return nil
			}
		}
	}
	err = nil

	// Loop until our shard processing session expires.
	prd := processRoundDelay{
		cfg: cfg,
	}
	prd.reset()

	for {
		var numProcessed, errCount, transientErrCount counter

		// Run our query against a work pool.
		//
		// NO work pool methods will return errors, so there is no need to collect
		// the result. Rather, any error that is encountered will atomically update
		// the "errCount" counter (for non-transient errors) or "transientErrCount"
		// counter (for transient errors).
		_ = parallel.WorkPool(int(cfg.NumGoroutines), func(ch chan<- func() error) {
			err := ds.Run(c, q, func(pm ds.PropertyMap) error {
				root := pm.Slice("TargetRoot")[0].Value().(*ds.Key)
				encRoot := root.Encode()

				// TODO(riannucci): make banSets remove keys from the banSet which
				// weren't hit. Once they stop showing up, they'll never show up
				// again.

				bs := t.banSets[encRoot]
				if bs == nil {
					bs = stringset.New(0)
					t.banSets[encRoot] = bs
				}
				ch <- func() error {
					switch err := processRoot(c, cfg, root, bs, &numProcessed); err {
					case nil:
						return nil

					case ds.ErrConcurrentTransaction:
						logging.Fields{
							logging.ErrorKey: err,
							"root":           root,
						}.Warningf(c, "Transient error encountered processing root.")
						transientErrCount.inc()
						return nil

					default:
						logging.Fields{
							logging.ErrorKey: err,
							"root":           root,
						}.Errorf(c, "Failed to process root.")
						errCount.inc()
						return nil
					}
				}

				if err := c.Err(); err != nil {
					logging.WithError(err).Warningf(c, "Context canceled (lost lock?).")
					return ds.Stop
				}
				return nil
			})
			if err != nil {
				var qstr string
				if fq, err := q.Finalize(); err == nil {
					qstr = fq.String()
				}

				logging.Fields{
					logging.ErrorKey: err,
					"query":          qstr,
				}.Errorf(c, "Failure to run shard query.")
				errCount.inc()
			}
		})

		logging.Infof(c, "cumulatively processed %d items with %d errors(s) and %d transient error(s)",
			numProcessed, errCount, transientErrCount)
		switch {
		case transientErrCount > 0:
			return errors.WrapTransient(errors.New("transient error during shard processing"))
		case errCount > 0:
			return errors.New("encountered non-transient error during shard processing")
		}

		now := clock.Now(c)
		didWork := numProcessed > 0
		if didWork {
			// Set our last key value for next round.
			err = mc.Set(c, mc.NewItem(c, t.lastKey).SetValue(serialize.ToBytes(now.UTC())))
			if err != nil {
				logging.Warningf(c, "could not update last process memcache key %s: %s", t.lastKey, err)
			}
		} else if t.endTime.IsZero() || !t.loop {
			// We didn't do any work this round, and we're configured for a single
			// loop, so we're done.
			logging.Debugf(c, "Configured for single loop.")
			return nil
		}

		// If we're past our end time, then we're done.
		if !t.endTime.IsZero() && now.After(t.endTime) {
			logging.Debugf(c, "Exceeded our process loop time by [%s]; terminating loop.", now.Sub(t.endTime))
			return nil
		}

		// Either we are looping, we did work last round, or both. Sleep in between
		// processing rounds for a duration based on whether or not we did work.
		delay := prd.next(didWork)
		if delay > 0 {
			// If we have an end time, and this delay would exceed that end time, then
			// don't bother sleeping; we're done.
			if !t.endTime.IsZero() && now.Add(delay).After(t.endTime) {
				logging.Debugf(c, "Delay (%s) exceeds process loop time (%s); terminating loop.",
					delay, t.endTime)
				return nil
			}

			logging.Debugf(c, "Sleeping %s in between rounds...", delay)
			if err := clock.Sleep(c, delay).Err; err != nil {
				logging.WithError(err).Warningf(c, "Sleep interrupted, terminating loop.")
				return nil
			}
		}
	}
}

func getBatchByRoot(c context.Context, cfg *Config, root *ds.Key, banSet stringset.Set) ([]*realMutation, error) {
	q := ds.NewQuery("tumble.Mutation").Eq("TargetRoot", root)
	if cfg.DelayedMutations {
		q = q.Lte("ProcessAfter", clock.Now(c).UTC())
	}

	fetchAllocSize := cfg.ProcessMaxBatchSize
	if fetchAllocSize < 0 {
		fetchAllocSize = 0
	}
	toFetch := make([]*realMutation, 0, fetchAllocSize)
	err := ds.Run(c, q, func(k *ds.Key) error {
		if !banSet.Has(k.Encode()) {
			toFetch = append(toFetch, &realMutation{
				ID:     k.StringID(),
				Parent: k.Parent(),
			})
		}
		if len(toFetch) < cap(toFetch) {
			return nil
		}
		return ds.Stop
	})
	return toFetch, err
}

func loadFilteredMutations(c context.Context, rms []*realMutation) ([]*ds.Key, []Mutation, error) {
	mutKeys := make([]*ds.Key, 0, len(rms))
	muts := make([]Mutation, 0, len(rms))
	err := ds.Get(c, rms)
	me, ok := err.(errors.MultiError)
	if !ok && err != nil {
		return nil, nil, err
	}

	for i, rm := range rms {
		err = nil
		if me != nil {
			err = me[i]
		}
		if err == nil {
			if rm.Version != getAppVersion(c) {
				logging.Fields{
					"mut_version": rm.Version,
					"cur_version": getAppVersion(c),
				}.Warningf(c, "loading mutation with different code version")
			}
			m, err := rm.GetMutation()
			if err != nil {
				logging.Errorf(c, "couldn't load mutation: %s", err)
				continue
			}
			muts = append(muts, m)
			mutKeys = append(mutKeys, ds.KeyForObj(c, rm))
		} else if err != ds.ErrNoSuchEntity {
			return nil, nil, me
		}
	}

	return mutKeys, muts, nil
}

type overrideRoot struct {
	Mutation

	root *ds.Key
}

func (o overrideRoot) Root(context.Context) *ds.Key {
	return o.root
}

func processRoot(c context.Context, cfg *Config, root *ds.Key, banSet stringset.Set, cnt *counter) error {
	l := logging.Get(c)

	toFetch, err := getBatchByRoot(c, cfg, root, banSet)
	switch {
	case err != nil:
		l.Errorf("Failed to get batch for root [%s]: %s", root, err)
		return err
	case len(toFetch) == 0:
		return nil
	}

	mutKeys, muts, err := loadFilteredMutations(c, toFetch)
	if err != nil {
		return err
	}

	if c.Err() != nil {
		l.Warningf("Lost lock during processRoot")
		return nil
	}

	allShards := map[taskShard]struct{}{}

	toDel := make([]*ds.Key, 0, len(muts))
	var numMuts, deletedMuts, processedMuts int
	err = ds.RunInTransaction(txnBuf.FilterRDS(c), func(c context.Context) error {
		toDel = toDel[:0]
		numMuts = 0
		deletedMuts = 0
		processedMuts = 0

		iterMuts := muts
		iterMutKeys := mutKeys

		for i := 0; i < len(iterMuts); i++ {
			m := iterMuts[i]

			logging.Fields{"m": m}.Infof(c, "running RollForward")
			shards, newMuts, newMutKeys, err := enterTransactionMutation(c, cfg, overrideRoot{m, root}, uint64(i))
			if err != nil {
				l.Errorf("Executing decoded gob(%T) failed: %q: %+v", m, err, m)
				continue
			}
			processedMuts++

			for j, nm := range newMuts {
				if nm.Root(c).HasAncestor(root) {
					runNow := !cfg.DelayedMutations
					if !runNow {
						dm, isDelayedMutation := nm.(DelayedMutation)
						runNow = !isDelayedMutation || clock.Now(c).UTC().After(dm.ProcessAfter())
					}
					if runNow {
						iterMuts = append(iterMuts, nm)
						iterMutKeys = append(iterMutKeys, newMutKeys[j])
					}
				}
			}

			// Finished processing this Mutation.
			key := iterMutKeys[i]
			if key.HasAncestor(root) {
				// try to delete it as part of the same transaction.
				if err := ds.Delete(c, key); err == nil {
					deletedMuts++
				} else {
					cnt.add(len(toDel))
					toDel = append(toDel, key)
				}
			} else {
				toDel = append(toDel, key)
			}

			numMuts += len(newMuts)
			for shard := range shards {
				allShards[shard] = struct{}{}
			}
		}

		return nil
	}, nil)
	if err != nil {
		l.Errorf("failed running transaction: %s", err)
		return err
	}

	fireTasks(c, cfg, allShards, true)
	l.Debugf("successfully processed %d mutations (%d tail-call), delta %d",
		processedMuts, deletedMuts, (numMuts - deletedMuts))

	if len(toDel) > 0 {
		cnt.add(len(toDel))
		for _, k := range toDel {
			banSet.Add(k.Encode())
		}
		if err := ds.Delete(c, toDel); err != nil {
			l.Warningf("error deleting finished mutations: %s", err)
		}
	}

	return nil
}

// counter is an atomic integer counter.
//
// When concurrent access is possible, a counter must only be manipulated with
// its "inc" and "add" methods, and must not be read.
//
// We use an int32 because that is atomically safe across all architectures.
type counter int32

func (c *counter) inc() int      { return c.add(1) }
func (c *counter) add(n int) int { return int(atomic.AddInt32((*int32)(c), int32(n))) }

// processRoundDelay calculates the delay to impose in between processing
// rounds.
type processRoundDelay struct {
	cfg       *Config
	nextDelay time.Duration
}

func (prd *processRoundDelay) reset() {
	// Reset our delay to DustSettleTimeout.
	prd.nextDelay = time.Duration(prd.cfg.DustSettleTimeout)
}

func (prd *processRoundDelay) next(didWork bool) time.Duration {
	if didWork {
		// Reset our delay to DustSettleTimeout.
		prd.reset()
		return prd.nextDelay
	}

	delay := prd.nextDelay
	if growth := prd.cfg.NoWorkDelayGrowth; growth > 1 {
		prd.nextDelay *= time.Duration(growth)
	}

	if max := time.Duration(prd.cfg.MaxNoWorkDelay); max > 0 && delay > max {
		delay = max

		// Cap our "next delay" so it doesn't grow unbounded in the background.
		prd.nextDelay = delay
	}

	// Enforce a no work lower bound.
	if delay < minNoWorkDelay {
		delay = minNoWorkDelay
	}

	return delay
}
