// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/filter/txnBuf"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/appengine/memlock"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
)

// expandedShardBounds returns the boundary of the expandedShard query that
// currently corresponds to this shard number. If Shard is < 0 or > NumShards
// (the currently configured number of shards), this will return a low > high.
// Otherwise low < high.
func expandedShardBounds(c context.Context, shard uint64) (low, high int64) {
	cfg := GetConfig(c)

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

var dustSettleTimeout = 2 * time.Second

// ProcessShardHandler is a http handler suitable for installation into
// a httprouter. It expects `logging` and `luci/gae` services to be installed
// into the context.
//
// ProcessShardHandler verifies that its being run as a taskqueue task and that
// the following parameters exist and are well-formed:
//   * timestamp: decimal-encoded UNIX/UTC timestamp in seconds.
//   * shard_id: decimal-encoded shard identifier.
//
// ProcessShardHandler then invokes ProcessShard with the parsed parameters.
func ProcessShardHandler(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
	tstampStr := p.ByName("timestamp")
	sidStr := p.ByName("shard_id")

	tstamp, err := strconv.ParseInt(tstampStr, 10, 64)
	if err != nil {
		logging.Errorf(c, "bad timestamp %q", tstampStr)
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "bad timestamp")
		return
	}

	sid, err := strconv.ParseUint(sidStr, 10, 64)
	if err != nil {
		logging.Errorf(c, "bad shardID %q", tstampStr)
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "bad shardID")
		return
	}

	err = ProcessShard(c, time.Unix(tstamp, 0).UTC(), sid)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "error: %s", err)
	} else {
		rw.Write([]byte("ok"))
	}
}

// ProcessShard is the tumble backend endpoint. This accepts a shard number which
// is expected to be < GlobalConfig.NumShards.
func ProcessShard(c context.Context, timestamp time.Time, shard uint64) error {
	low, high := expandedShardBounds(c, shard)
	if low > high {
		return nil
	}

	l := logging.Get(logging.SetField(c, "shard", shard))

	cfg := GetConfig(c)

	lockKey := fmt.Sprintf("%s.%d.lock", cfg.Name, shard)
	clientID := fmt.Sprintf("%d_%d", timestamp.Unix(), shard)

	// this last key allows buffered tasks to early exit if some other shard
	// processor has already processed past this task's target timestamp.
	lastKey := fmt.Sprintf("%s.%d.last", cfg.Name, shard)
	mc := memcache.Get(c)
	lastItm, err := mc.Get(lastKey)
	if err != nil {
		if err != memcache.ErrCacheMiss {
			l.Warningf("couldn't obtain last timestamp: %s", err)
		}
	} else {
		val := lastItm.Value()
		last, err := serialize.ReadTime(bytes.NewBuffer(val))
		if err != nil {
			l.Warningf("could not decode timestamp %v: %s", val, err)
		} else {
			last = last.Add(cfg.TemporalRoundFactor)
			if timestamp.After(last) {
				l.Infof("early exit, %s > %s", timestamp, last)
				return nil
			}
		}
	}
	err = nil

	q := datastore.NewQuery("tumble.Mutation").
		Gte("ExpandedShard", low).Lte("ExpandedShard", high).
		Project("TargetRoot").Distinct(true).
		Limit(cfg.ProcessMaxBatchSize)

	banSets := map[string]stringset.Set{}

	limitSemaphore := make(chan struct{}, cfg.NumGoroutines)

	for try := 0; try < 2; try++ {
		err = memlock.TryWithLock(c, lockKey, clientID, func(c context.Context) error {
			l.Infof("Got lock (try %d)", try)

			for {
				processCounters := []*int64{}
				err := parallel.FanOutIn(func(ch chan<- func() error) {
					err := datastore.Get(c).Run(q, func(pm datastore.PropertyMap, _ datastore.CursorCB) bool {
						root := pm["TargetRoot"][0].Value().(*datastore.Key)
						encRoot := root.Encode()

						// TODO(riannucci): make banSets remove keys from the banSet which
						// weren't hit. Once they stop showing up, they'll never show up
						// again.

						bs := banSets[encRoot]
						if bs == nil {
							bs = stringset.New(0)
							banSets[encRoot] = bs
						}
						counter := new(int64)
						processCounters = append(processCounters, counter)

						ch <- func() error {
							limitSemaphore <- struct{}{}
							defer func() {
								<-limitSemaphore
							}()
							return processRoot(c, root, bs, counter)
						}

						select {
						case <-c.Done():
							l.Warningf("Lost lock!")
							return false
						default:
							return true
						}
					})
					if err != nil {
						l.Errorf("Failure to query: %s", err)
						ch <- func() error {
							return err
						}
					}
				})
				if err != nil {
					return err
				}
				numProcessed := int64(0)
				for _, n := range processCounters {
					numProcessed += *n
				}
				l.Infof("cumulatively processed %d items", numProcessed)
				if numProcessed == 0 {
					break
				}

				err = mc.Set(mc.NewItem(lastKey).SetValue(serialize.ToBytes(clock.Now(c))))
				if err != nil {
					l.Warningf("could not update last process memcache key %s: %s", lastKey, err)
				}

				clock.Sleep(c, dustSettleTimeout)
			}
			return nil
		})
		if err != memlock.ErrFailedToLock {
			break
		}
		l.Infof("Couldn't obtain lock (try %d) (sleeping 2s)", try+1)
		clock.Sleep(c, time.Second*2)
	}
	if err == memlock.ErrFailedToLock {
		l.Infof("Couldn't obtain lock (giving up): %s", err)
		err = nil
	}
	return err
}

func getBatchByRoot(c context.Context, root *datastore.Key, banSet stringset.Set) ([]*realMutation, error) {
	cfg := GetConfig(c)
	ds := datastore.Get(c)
	q := datastore.NewQuery("tumble.Mutation").Eq("TargetRoot", root)
	toFetch := make([]*realMutation, 0, cfg.ProcessMaxBatchSize)
	err := ds.Run(q, func(k *datastore.Key, _ datastore.CursorCB) bool {
		if !banSet.Has(k.Encode()) {
			toFetch = append(toFetch, &realMutation{
				ID:     k.StringID(),
				Parent: k.Parent(),
			})
		}
		return len(toFetch) < cap(toFetch)
	})
	return toFetch, err
}

func loadFilteredMutations(c context.Context, rms []*realMutation) ([]*datastore.Key, []Mutation, error) {
	ds := datastore.Get(c)

	mutKeys := make([]*datastore.Key, 0, len(rms))
	muts := make([]Mutation, 0, len(rms))
	err := ds.GetMulti(rms)
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
			mutKeys = append(mutKeys, ds.KeyForObj(rm))
		} else if err != datastore.ErrNoSuchEntity {
			return nil, nil, me
		}
	}

	return mutKeys, muts, nil
}

type overrideRoot struct {
	Mutation

	root *datastore.Key
}

func (o overrideRoot) Root(context.Context) *datastore.Key {
	return o.root
}

func processRoot(c context.Context, root *datastore.Key, banSet stringset.Set, counter *int64) error {
	l := logging.Get(c)

	toFetch, err := getBatchByRoot(c, root, banSet)
	if err != nil || len(toFetch) == 0 {
		return err
	}

	mutKeys, muts, err := loadFilteredMutations(c, toFetch)
	if err != nil {
		return err
	}

	select {
	case <-c.Done():
		l.Warningf("Lost lock during processRoot")
	default:
	}

	allShards := map[uint64]struct{}{}

	toDel := make([]*datastore.Key, 0, len(muts))
	numMuts := uint64(0)
	err = datastore.Get(txnBuf.FilterRDS(c)).RunInTransaction(func(c context.Context) error {
		toDel = toDel[:0]
		numMuts = 0

		for i, m := range muts {
			shards, numNewMuts, err := enterTransactionInternal(c, overrideRoot{m, root})
			if err != nil {
				l.Errorf("Executing decoded gob(%T) failed: %q: %+v", m, err, m)
				continue
			}
			toDel = append(toDel, mutKeys[i])
			numMuts += uint64(numNewMuts)
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
	fireTasks(c, allShards)
	l.Infof("successfully processed %d mutations, adding %d more", len(toDel), numMuts)

	if len(toDel) > 0 {
		atomic.StoreInt64(counter, int64(len(toDel)))

		for _, k := range toDel {
			banSet.Add(k.Encode())
		}
		if err := datastore.Get(c).DeleteMulti(toDel); err != nil {
			l.Warningf("error deleting finished mutations: %s", err)
		}
	}

	return nil
}
