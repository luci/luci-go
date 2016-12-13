// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastorecache

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/appengine/memlock"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/sync/parallel"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/gae/filter/dsQueryBatch"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
)

const (
	// managerQueryBatchSize is the query batch size for cache entries that are
	// processed by a manager shard.
	managerQueryBatchSize = 200
)

func errHTTPHandler(fn func(c context.Context, req *http.Request, params httprouter.Params) error) router.Handler {
	return func(ctx *router.Context) {
		err := fn(ctx.Context, ctx.Request, ctx.Params)
		if err == nil {
			// Handler returned no error, everything is good.
			return
		}

		// Handler returned an error, dump it to output.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)

		// Log all of our stack lines individually, so we don't overflow the
		// maximum log message size with a full stack.
		stk := errors.RenderStack(err)
		log.WithError(err).Errorf(ctx.Context, "Handler returned error.")
		for _, line := range stk.ToLines() {
			log.Errorf(ctx.Context, ">> %s", line)
		}

		dumpErr := func() error {
			var buf bytes.Buffer
			if _, err := stk.DumpTo(&buf); err != nil {
				return err
			}
			if _, err := buf.WriteTo(ctx.Writer); err != nil {
				return err
			}
			return nil
		}()
		if dumpErr != nil {
			log.WithError(dumpErr).Errorf(ctx.Context, "Failed to dump error stack.")
		}
	}
}

// manager is initialized to perform the management cron task.
type manager struct {
	cache *Cache

	queryBatchSize int
}

// installCronRoute installs a handler for this manager's cron task into the
// supplied Router at the specified path.
//
// It is recommended to assert in the middleware that this endpoint is only
// accessible from a cron task.
func (m *manager) installCronRoute(path string, r *router.Router, base router.MiddlewareChain) {
	r.GET(path, base, errHTTPHandler(m.handleManageCronGET))
}

func (m *manager) handleManageCronGET(c context.Context, req *http.Request, params httprouter.Params) error {
	var h Handler
	if hf := m.cache.HandlerFunc; hf != nil {
		h = hf(c)
	}

	// NOTE: All manager runs currently have exactly one shard, #0.
	const shardID = 0
	shard := managerShard{
		manager: m,
		h:       h,
		now:     clock.Now(c).UTC(),
		st: managerShardStats{
			Shard: shardID + 1, // +1 b/c 0 is invalid ID.
		},
		clientID: strings.Join([]string{
			"datastore_cache_manager",
			info.RequestID(c),
		}, "\x00"),
		shard:    0,
		shardKey: fmt.Sprintf("datastore_cache_manager_shard_%d", shardID),
	}
	return shard.run(c)
}

type managerShard struct {
	*manager

	// h is this manager's Handler. Note this can be nil, in which case the cached
	// entries will be pruned eventually.
	h Handler

	clientID string
	now      time.Time

	st managerShardStats

	shard    int
	shardKey string

	// entries is the number of observed cache entries.
	entries int32
	// errors is the number of errors encountered. If this is non-zero, our
	// handler will return an error.
	errors int32
}

func (ms *managerShard) observeEntry()         { atomic.AddInt32(&ms.entries, 1) }
func (ms *managerShard) observeErrors(c int32) { atomic.AddInt32(&ms.errors, c) }

func (ms *managerShard) run(c context.Context) error {
	// Enter our cache cacheNamespace. We'll leave this when calling Handler
	// functions.
	c = ms.cache.withNamespace(c)
	c = log.SetField(c, "shard", ms.shard)

	// Take out a memlock on our cache shard.
	return memlock.TryWithLock(c, ms.shardKey, ms.clientID, func(c context.Context) (rv error) {
		// Output our stats on completion.
		defer func() {
			ms.st.LastEntryCount = int(ms.entries)
			if rv == nil {
				ms.st.LastSuccessfulRun = ms.now
			}

			if rv := datastore.Put(c, &ms.st); rv != nil {
				log.WithError(rv).Errorf(c, "Failed to Put() stats on completion.")
			}
		}()

		if err := ms.runLocked(c); err != nil {
			return errors.Annotate(err).Err()
		}

		// If we observed errors during processing, note this.
		if ms.errors > 0 {
			return errors.Reason("%(count)d error(s) encountered during processing").D("count", ms.errors).Err()
		}
		return nil
	})
}

// runLocked runs the main main maintenance loop.
//
// As the run is executed, stats can be collected in ms.st. These will be output
// to datastore on completion.
func (ms *managerShard) runLocked(c context.Context) error {
	workers := ms.cache.Parallel
	if workers <= 0 {
		workers = 1
	}

	// NOTE: This does not currently restrain itself to the current shard. This
	// can be done using a "__key__" inequality filter to bound the query.
	prototype := entry{
		CacheName: ms.cache.Name,
	}
	q := datastore.NewQuery(prototype.kind())

	var (
		totalEntries   = 0
		totalRefreshed = 0
		totalPruned    = 0
		totalErrors    = int32(0)

		entries   = make([]*entry, 0, ms.queryBatchSize)
		putBuf    = make([]*entry, ms.queryBatchSize)
		deleteBuf = make([]*entry, ms.queryBatchSize)
	)

	// Calculate our pruning threshold. If an entry's "LastAccessed" is <= this
	// threshold, it is candidate for pruning.
	var pruneThreshold time.Time
	if pi := ms.cache.pruneInterval(); pi > 0 {
		pruneThreshold = ms.now.Add(-pi)
	}

	// handleEntries refreshes the accumulated entries. It is used as a callback
	// in between query batches, as well as a finalizer after the queries
	// complete.
	//
	// handleEntries is, itself, not goroutine-safe.
	//
	// It will refresh in parallel, adding entries to "putBuf" or "deleteBuf" as
	// appropriate. At the end of its operation, all deferred datastore
	// operations will execute, and the accumulated entries list will be purged
	// for next round.
	handleEntries := func(c context.Context) error {
		if len(entries) == 0 {
			return nil
		}

		// Use atomic-friendly int32 values to index the put/delete buffers. This
		// will let our parallel goroutines safely add entries with very low
		// overhead.
		var putIdx, deleteIdx int32
		putEntry := func(e *entry) { putBuf[atomic.AddInt32(&putIdx, 1)-1] = e }
		deleteEntry := func(e *entry) { deleteBuf[atomic.AddInt32(&deleteIdx, 1)-1] = e }

		// Process each entry in parallel.
		//
		// Each task will return a nil error, so the error result does not need to
		// be observed.
		_ = parallel.WorkPool(workers, func(taskC chan<- func() error) {
			for _, e := range entries {
				e := e

				taskC <- func() error {
					// Is this entry candidate for pruning?
					if !(pruneThreshold.IsZero() || e.LastAccessed.After(pruneThreshold)) {
						log.Fields{
							"key":            e.keyHash(),
							"lastRefresh":    e.LastRefreshed,
							"lastAccessed":   e.LastAccessed,
							"pruneThreshold": pruneThreshold,
						}.Infof(c, "Pruning expired cache entry.")
						deleteEntry(e)
						return nil
					}

					// Is this cache entry candidate for refresh?
					if ms.h != nil {
						refreshInterval := ms.h.RefreshInterval(e.Key)
						if refreshInterval > 0 && !e.LastRefreshed.After(ms.now.Add(-refreshInterval)) {
							// Call our Handler's Refresh function. We leave our cache namespace
							// first.
							switch value, delta, err := doRefresh(c, ms.h, e, ""); err {
							case nil:
								// Refresh successful! Update the entry.
								//
								// Even if he data hasn't changed, the LastRefreshed time has, and
								// the cost of the "Put" is the same either way.
								e.LastRefreshed = ms.now
								e.LastRefreshDelta = int64(delta)
								e.loadValue(value)
								putEntry(e)
								return nil

							case ErrDeleteCacheEntry:
								log.Fields{
									"key": e.keyHash(),
								}.Debugf(c, "Refresh requested entry deletion.")
								deleteEntry(e)
								return nil

							default:
								log.Fields{
									log.ErrorKey:  err,
									"key":         e.keyHash(),
									"lastRefresh": e.LastRefreshed,
								}.Errorf(c, "Failed to refresh cache entry.")
								atomic.AddInt32(&totalErrors, 1)
							}
						}
					}
					return nil
				}
			}
		})

		// Clear our entries buffer for next round.
		entries = entries[:0]

		// Flush our put/delete buffers. Accumulate errors (best effort) and return
		// them as a batch.
		//
		// A failure here is a datastore failure, and will halt processing by
		// propagating from the callback to the query return value.
		_ = parallel.FanOutIn(func(taskC chan<- func() error) {
			if putIdx > 0 {
				taskC <- func() error {
					if err := datastore.Put(c, putBuf[:putIdx]); err != nil {
						log.Fields{
							log.ErrorKey: err,
							"size":       putIdx,
						}.Errorf(c, "Failed to Put batch.")
						atomic.AddInt32(&totalErrors, 1)
					} else {
						totalRefreshed += int(putIdx)
					}
					return nil
				}
			}

			if deleteIdx > 0 {
				taskC <- func() error {
					if err := datastore.Delete(c, deleteBuf[:deleteIdx]); err != nil {
						log.Fields{
							log.ErrorKey: err,
							"size":       deleteIdx,
						}.Errorf(c, "Failed to Delete batch.")
						atomic.AddInt32(&totalErrors, 1)
					} else {
						totalPruned += int(deleteIdx)
					}
					return nil
				}
			}
		})
		return nil
	}

	err := datastore.Run(dsQueryBatch.BatchQueries(c, int32(ms.queryBatchSize), handleEntries), q, func(e *entry) error {
		totalEntries++
		ms.observeEntry()
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return errors.Annotate(err).Reason("failed to run entry query").Err()
	}

	// Flush any outstanding entries (ignore error, will always be nil).
	_ = handleEntries(c)
	if totalErrors > 0 {
		ms.observeErrors(totalErrors)
	}

	log.Fields{
		"entries":   totalEntries,
		"errors":    totalErrors,
		"refreshed": totalRefreshed,
		"pruned":    totalPruned,
	}.Infof(c, "Successfully updated cache entries.")
	return nil
}
