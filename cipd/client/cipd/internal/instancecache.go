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

package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

const (
	// instanceCacheMaxAge defines when to purge a cached package that is not
	// being used.
	//
	// Works in parallel with 'instanceCacheMaxSize' limit.
	instanceCacheMaxAge = 48 * time.Hour

	// instanceCacheSyncInterval determines the frequency of synchronization of
	// state.db with instance files in the cache dir.
	instanceCacheSyncInterval = 8 * time.Hour

	// instanceCacheStateFilename is a name of the file with InstanceCache proto.
	instanceCacheStateFilename = "state.db"
)

// Fetcher knows how to populate a file in the cache given its pin.
//
// It must check the hash of the downloaded package matches the pin.
type Fetcher func(ctx context.Context, pin common.Pin, f io.WriteSeeker) error

// ErrNoFetcher is the error returned when an InstanceCache is configured with
// a nil Fetcher but is missing data.
var ErrNoFetcher = cipderr.OfflineClient.Apply(
	errors.New("no fetcher configured"))

// CorruptFileDeletionFailure indicates that we had a corrupt cache file,
// attempted to delete it, but this deletion failed.
//
// This allows distinguishing 'corrupt file' (which was cleaned) vs 'corrupt
// file' (which may still be on disk).
//
// This will be set on [InstanceResponse].Err if it occurred.
var CorruptFileDeletionFailure = errtag.Make("failed to remove corrupt file", true)

// InstanceOpenMode describes how an InstanceRequest wants to open the instance.
type InstanceOpenMode byte

const (
	// Source (the default) indicates that the cache should just return the
	// pkg.Source without opening the file.
	Source InstanceOpenMode = iota

	// Instance indicates that the cache should open the pkg.Source as a
	// pkg.Instance.
	Instance

	// VerifiedInstance indicates that the cache should open the pkg.Source as a
	// pkg.Instance, but only after checking that it's hash matches the
	// InstanceID
	VerifiedInstance
)

// InstanceRequest is passed to RequestInstances.
type InstanceRequest struct {
	Context context.Context    // carries the cancellation signal
	Done    context.CancelFunc // called right before the result is enqueued
	Pin     common.Pin         // identifies the instance to fetch
	OpenAs  InstanceOpenMode   // indicates how the cache should open the instance file.
	State   any                // passed to the InstanceResult as is
}

// InstanceResult is a result of a completed InstanceRequest.
type InstanceResult struct {
	Context  context.Context // copied from the InstanceRequest
	Err      error           // non-nil if failed to obtain the instance
	Source   pkg.Source      // set only if OpenAs was Source, must be closed by the caller
	Instance pkg.Instance    // set only if OpenAs was not Source, must be closed by the caller
	State    any             // copied from the InstanceRequest
}

// InstanceCache is a file-system-based, thread-safe, LRU cache of instances.
//
// It also knows how to populate the cache using the given Fetcher callback,
// fetching multiple instances concurrently.
//
// Does not validate instance hashes; it is the responsibility of the supplied
// Fetcher callback.
type InstanceCache struct {
	// FS is a root of the cache directory.
	FS fs.FileSystem

	// Tmp, if true, indicates this is a temporary instance cache.
	//
	// It is a cache of downloaded, but not yet installed packages. It has a
	// property that cached files self-destruct after being closed and the entire
	// cache directory is removed in Close().
	Tmp bool

	// Fetcher is used to fetch files into the cache on cache misses.
	//
	// It must check the hash of the downloaded package matches the pin.
	//
	// If this is `nil`, then any attempts to fetch missing instances will
	// fail with an error wrapping [ErrNoFetcher] (accessing cached instances
	// still works).
	Fetcher Fetcher

	// ParallelDownloads limits how many parallel fetches can happen at once.
	//
	// The zero value means to do fetches in a blocking way in WaitInstance.
	ParallelDownloads int

	// ExactGC causes the garbage collection to collect all instances which were
	// not requested by this cache, and disables automatic collection as part of
	// state synchronization (so: the caller setting ExactGC must explicitly call
	// GC() when all done with this cache). This is internally modeled as setting
	// the maxAge to zero.
	ExactGC bool

	stateLock sync.Mutex // synchronizes access to the state file.

	// launchTime is the time at which RequestInstances was first called; this is
	// used to evaluate if instances are too old during garbage collection. Zero
	// if RequestInstances has not been called yet.
	launchTime time.Time

	fetchPending atomic.Int32
	fetchReq     chan *InstanceRequest
	fetchRes     chan *InstanceResult // non-nil only if ParallelDownloads > 0
	fetchWG      sync.WaitGroup

	// Defaults to instanceCacheMaxAge, mocked in tests.
	maxAge time.Duration
}

// cacheFile is a downloaded instance file, implements pkg.Source.
type cacheFile struct {
	cache *InstanceCache
	pin   common.Pin
	file  *os.File
	size  int64
}

func (f *cacheFile) ReadAt(p []byte, off int64) (int, error) { return f.file.ReadAt(p, off) }
func (f *cacheFile) Size() int64                             { return f.size }

func (f *cacheFile) Close(ctx context.Context, corrupt bool) error {
	corruptText := ""
	if corrupt {
		corruptText = " corrupt"
	}

	var err error
	if err = f.file.Close(); err != nil && err != os.ErrClosed {
		logging.Warningf(ctx, "Failed to close%s cache file: %s", corruptText, err)
	} else {
		err = nil
	}

	if corrupt || f.cache.Tmp {
		if err2 := f.cache.delete(ctx, f.pin); err2 != nil {
			logging.Warningf(ctx, "Failed to delete%s cache file: %s", corruptText, err2)
			if err == nil {
				err = err2
			}
			err = CorruptFileDeletionFailure.Apply(err)
		}
	}

	return err
}

// maybeLaunch prepares the cache for fetching packages in background.
//
// called implicitly by RequestInstances.
func (c *InstanceCache) maybeLaunch(ctx context.Context) {
	if c.ParallelDownloads < 0 {
		panic("ParallelDownloads must be non-negative")
	}

	if !c.launchTime.IsZero() {
		return
	}
	c.launchTime = clock.Now(ctx)

	tmp := ""
	if c.Tmp {
		tmp = " temporary"
	}
	logging.Infof(ctx, "Using%s instance cache at %s", tmp, c.FS.Root())

	// This is an effectively an unlimited buffer. At very least it should be
	// large enough to hold all requests submitted by RequestInstances(...) before
	// the first WaitInstance() call when ParallelDownloads == 0 and the channel
	// is drained only in WaitInstance(). When ParallelDownloads > 0 the buffer
	// size doesn't matter much.
	c.fetchReq = make(chan *InstanceRequest, 1000000)

	// If allowed to fetch in parallel, run the work pool that does the job.
	if c.ParallelDownloads > 0 {
		// The buffer size here controls how many packages are allowed to sit in the
		// "downloaded, but not yet installed" queue before the pipeline starts
		// blocking new downloads. Each such pending file takes some resources (at
		// least an open file descriptor and some memory), so it's better to limit
		// the buffer size by some reasonable number.
		c.fetchRes = make(chan *InstanceResult, 50)

		// Start with 1 allowed concurrent download. It will eventually ramp up to
		// ParallelDownloads with each finished download.
		//
		// This is a heuristic for `ensure`-type flows to make sure we download the
		// first package ASAP, so the installer has something to install while more
		// packages are being downloaded.
		//
		// NOTE: workersLeft can become up to c.ParallelDownloads negative, but
		// workers will only launch a new worker if they observe it as >= 0 after
		// decrementing it so we should only launch a max of c.ParallelDownloads
		// workers.
		var workersLeft atomic.Int32
		workersLeft.Store(int32(c.ParallelDownloads - 1))

		var worker func()
		worker = func() {
			for req := range c.fetchReq {
				c.fetchRes <- c.handleRequest(req)

				if workersLeft.Load() > 0 {
					if workersLeft.Add(-1) >= 0 {
						c.fetchWG.Go(worker)
					}
				}
			}
		}

		c.fetchWG.Go(worker)
	}
}

// `now` implements a gated version of clock.Now(ctx) which is resistant to
// shifts in system clock time.
//
// Normally `clock.Now()` (a.k.a. `time.Now`) would be resistant to such things
// when comparing `time.Time` objects. However in the case of the
// InstanceCache, we round-trip `time.Time` through `google.protobuf.Timestamp`
// messages, which do not contain the secret monotonic clock bits, and thus are
// vulnerable to e.g. `ntpd` adjusting the system clock.
//
// We want to preserve the property that all instances `touch`'d by this
// InstanceCache (either by fetching them, or using them off the disk) have a
// timestamp which is > c.launchTime. This is especially important when the GC
// maxAge is zero (e.g. ExactGC).
//
// This will return `clock.Now(ctx)` except if this would be before
// c.launchTime, in which case it will return c.launchTime + 1ns.
func (c *InstanceCache) now(ctx context.Context) time.Time {
	ret := clock.Now(ctx)
	if ret.After(c.launchTime) {
		return ret
	}
	return c.launchTime.Add(time.Nanosecond)
}

// Close shuts down goroutines started, cleans up temp files and does a garbage
// collection pass.
//
// The caller should ensure there's no pending fetches before making this call.
//
// Resets this InstanceCache to its initial state.
func (c *InstanceCache) Close(ctx context.Context) {
	if c.HasPendingFetches() {
		panic("closing an InstanceCache with some fetches still pending")
	}

	if c.launchTime.IsZero() {
		return
	}

	close(c.fetchReq)
	if c.ParallelDownloads > 0 {
		c.fetchWG.Wait()
	}

	// Self-destruct if the cache was temporary.
	if c.Tmp {
		if err := os.RemoveAll(c.FS.Root()); err != nil {
			logging.Warningf(ctx, "Leaking temp directory %s: %s", c.FS.Root(), err)
		}
	} else {
		// Do a garbage collection pass.
		c.withState(ctx, c.now(ctx), func(state *messages.InstanceCache) (save bool) {
			c.gc(ctx, state)
			return true
		})
	}

	// Finally, reset state.
	c.fetchPending.Store(0)
	c.fetchReq = nil
	c.fetchRes = nil
	c.launchTime = time.Time{}
}

// RequestInstances enqueues requests to fetch instances into the cache.
//
// The results can be waited upon using WaitInstance() method. Each enqueued
// InstanceRequest will get an exactly one InstanceResponse (perhaps with an
// error inside). The order of responses may not match the order of requests.
func (c *InstanceCache) RequestInstances(ctx context.Context, reqs []*InstanceRequest) {
	// Ensure we are launched.
	c.maybeLaunch(ctx)

	now := c.now(ctx)
	c.withState(ctx, now, func(s *messages.InstanceCache) (save bool) {
		for _, r := range reqs {
			// Mark any existing instances as used so they
			// won't be GCd.
			if e, ok := s.Entries[r.Pin.InstanceID]; ok {
				e.LastAccess = timestamppb.New(now)
			}
		}
		return true
	})

	c.fetchPending.Add(int32(len(reqs)))
	for _, r := range reqs {
		c.fetchReq <- r
	}
}

// HasPendingFetches is true if there's some uncompleted InstanceRequest whose
// completion notification will (eventually) unblock WaitInstance.
func (c *InstanceCache) HasPendingFetches() bool {
	return c.fetchPending.Load() > 0
}

// WaitInstance blocks until some InstanceRequest is available.
func (c *InstanceCache) WaitInstance() (res *InstanceResult) {
	if c.ParallelDownloads == 0 {
		// No parallelism allowed, fetch inline.
		res = c.handleRequest(<-c.fetchReq)
	} else {
		// Otherwise wait for the work pool to finish a request.
		res = <-c.fetchRes
	}
	c.fetchPending.Add(-1)
	return
}

// handleRequest handles an *InstanceRequest producing an *InstanceResult.
func (c *InstanceCache) handleRequest(req *InstanceRequest) *InstanceResult {
	if req.Done != nil {
		defer req.Done()
	}

	res := &InstanceResult{
		Context: req.Context,
		Err:     req.Context.Err(),
		State:   req.State,
	}
	if res.Err != nil {
		return res
	}

	ctx := req.Context
	pin := req.Pin

	switch src, err := c.openAsSource(ctx, pin); {
	case err != nil:
		res.Err = err
	case req.OpenAs == Source:
		res.Source = src
	case req.OpenAs == Instance, req.OpenAs == VerifiedInstance:
		// Normally, the Fetcher will verify the hash when fetching from the
		// network. However, in the case where the on-disk state can't be trusted
		// (e.g. it is a prepared offline cache which doesn't have a trusted chain
		// of custody from preparation to usage), the caller may require that we
		// verify the hash when opening the instance.
		//
		// We don't do this by default because on slow systems (e.g. tiny ARM SoC,
		// like raspberry pi) the hash verification can take significant time.
		// Modern systems with nvme storage will see a couple GBps here.
		vm := reader.SkipHashVerification
		if req.OpenAs == VerifiedInstance {
			vm = reader.VerifyHash
		}
		instance, err := reader.OpenInstance(ctx, src, reader.OpenInstanceOpts{
			VerificationMode: vm,
			InstanceID:       pin.InstanceID,
		})

		if err != nil {
			// We join the src.Close error so the caller can detect
			// CorruptFileDeletionFailure, if it occurred.
			res.Err = errors.Join(err, src.Close(ctx, reader.IsCorruptionError(err)))
		} else {
			res.Instance = instance
		}
	default:
		panic(fmt.Sprintf("impossible: InstanceRequest.OpenAs: %x", req.OpenAs))
	}
	return res
}

// openAsSource fetches the instance file and returns it as pkg.Source.
func (c *InstanceCache) openAsSource(ctx context.Context, pin common.Pin) (pkg.Source, error) {
	f, err := c.openOrFetch(ctx, pin)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, cipderr.IO.Apply(errors.Fmt("checking size: %w", err))
	}

	return &cacheFile{
		cache: c,
		pin:   pin,
		file:  f,
		size:  stat.Size(),
	}, nil
}

// openOrFetch either opens an existing instance file or writes a new one.
func (c *InstanceCache) openOrFetch(ctx context.Context, pin common.Pin) (*os.File, error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return nil, err
	}

	path, err := c.FS.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("invalid instance ID %q", pin.InstanceID))
	}

	attempt := 0
	for {
		attempt++

		// Try to see if we have the file in the cache already.
		switch file, err := c.FS.OpenFile(path); {
		case os.IsNotExist(err):
			// No such file in the cache. This is fine.
		case err != nil:
			// Some unexpected error. Log and carry on, as if it is a cache miss.
			logging.Warningf(ctx, "Could not get %s from the cache: %s", pin, err)
		default:
			logging.Infof(ctx, "Cache hit for %s", pin)
			c.touch(ctx, pin.InstanceID) // Bump its last access time.
			return file, nil
		}

		if c.Fetcher == nil {
			return nil, errors.Fmt("missing %s: %w", pin, ErrNoFetcher)
		}

		// No such cached instance, download it.
		err := c.FS.EnsureFile(ctx, path, func(f *os.File) error {
			return c.Fetcher(ctx, pin, f)
		})
		if err != nil {
			if cipderr.ToCode(err) == cipderr.Unknown {
				err = cipderr.IO.Apply(errors.Fmt("writing to instance cache: %w", err))
			}
			return nil, err
		}

		// Try to open it now. There's a (very) small chance that it has been
		// evicted from the cache already. If this happens, try again, but do it
		// only once.
		//
		// Note that theoretically we could keep open the handle to the file used in
		// EnsureFile above, but this file gets renamed at some point, and renaming
		// files with open handles on Windows is moot. So instead we close it,
		// rename the file (this happens inside EnsureFile), and reopen it again
		// under the new name.
		switch file, err := c.FS.OpenFile(path); {
		case os.IsNotExist(err) && attempt == 1:
			logging.Infof(ctx, "Pin %s is still not in the cache, retrying...", pin)
		case os.IsNotExist(err) && attempt > 1:
			logging.Errorf(ctx, "Pin %s is unexpectedly missing from the cache", pin)
			return nil, cipderr.IO.Apply(errors.Fmt("pin %s is unexpectedly missing from the cache: %w", pin, err))
		case err != nil:
			logging.Errorf(ctx, "Failed to open the instance %s: %s", pin, err)
			return nil, cipderr.IO.Apply(errors.Fmt("opening the cached instance %s: %w", pin, err))
		default:
			c.touch(ctx, pin.InstanceID) // Mark it as accessed.
			return file, nil
		}
	}
}

func (c *InstanceCache) touch(ctx context.Context, instanceID string) {
	now := c.now(ctx)
	c.withState(ctx, now, func(s *messages.InstanceCache) (save bool) {
		// Update LastAccess time.
		entry := s.Entries[instanceID]
		if entry == nil {
			entry = &messages.InstanceCache_Entry{}
			if s.Entries == nil {
				s.Entries = map[string]*messages.InstanceCache_Entry{}
			}
			s.Entries[instanceID] = entry
		}
		entry.LastAccess = timestamppb.New(now)
		return true
	})
}

// delete deletes an instance from the cache if it was there.
func (c *InstanceCache) delete(ctx context.Context, pin common.Pin) error {
	path, err := c.FS.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return cipderr.BadArgument.Apply(errors.Fmt("invalid instance ID %q", pin.InstanceID))
	}

	if err := c.FS.EnsureFileGone(ctx, path); err != nil {
		return cipderr.IO.Apply(errors.Fmt("deleting the cached instance file: %w", err))
	}

	c.withState(ctx, c.now(ctx), func(s *messages.InstanceCache) (save bool) {
		if s.Entries[pin.InstanceID] == nil {
			return false
		}
		delete(s.Entries, pin.InstanceID)
		return true
	})

	return nil
}

// gc cleans up the old instances.
//
// This relies purely on a policy which collects instances which haven't been
// touched for too long.
func (c *InstanceCache) gc(ctx context.Context, state *messages.InstanceCache) {
	maxAge := c.maxAge
	if maxAge == 0 && !c.ExactGC {
		maxAge = instanceCacheMaxAge
	}

	if c.launchTime.IsZero() {
		panic("cannot GC on never-launched cache.")
	}

	// Kick out entries older than the age threshold.
	garbage := stringset.New(0)
	for instanceID, e := range state.Entries {
		age := c.launchTime.Sub(e.LastAccess.AsTime())
		if age >= maxAge {
			garbage.Add(instanceID)
			logging.Debugf(ctx, "Purging cached instance %q (age %s as of %s)",
				instanceID, age, c.launchTime)
		}
	}

	for instanceID := range garbage.Iter {
		path, err := c.FS.RootRelToAbs(instanceID)
		if err != nil {
			panic(fmt.Sprintf("failed to convert path %q: %v", instanceID, err))
		}
		// EnsureFileGone logs errors already.
		if c.FS.EnsureFileGone(ctx, path) == nil {
			delete(state.Entries, instanceID)
		}
	}

	c.FS.CleanupTrash(ctx)
}

// readState loads cache state from the state file.
//
// If the file does not exist, corrupted or its state was not synchronized
// with the instance files for a long time, synchronizes it.
//
// Newly discovered files are considered last accessed at `c.launchTime`.
//
// If synchronization fails, then the state is considered empty.
func (c *InstanceCache) readState(ctx context.Context, state *messages.InstanceCache, now time.Time) {
	statePath, err := c.FS.RootRelToAbs(instanceCacheStateFilename)
	if err != nil {
		panic(fmt.Sprintf("failed to convert path %q: %v", instanceCacheStateFilename, err))
	}

	stateBytes, err := os.ReadFile(statePath)
	sync := false
	switch {
	case os.IsNotExist(err):
		sync = true

	case err != nil:
		logging.Warningf(ctx, "Could not read the instance cache state: %s", err)
		sync = true

	default:
		if err := UnmarshalWithSHA256(stateBytes, state); err != nil {
			if err == ErrUnknownSHA256 {
				logging.Warningf(ctx, "Need to rebuild instance cache index file")
			} else {
				logging.Warningf(ctx, "Instance cache file is corrupted: %s", err)
			}
			*state = messages.InstanceCache{}
			sync = true
		} else {
			cutOff := now.
				Add(-instanceCacheSyncInterval).
				Add(time.Duration(rand.Int63n(int64(5 * time.Minute))))
			sync = state.LastSynced.AsTime().Before(cutOff)
		}
	}

	if sync {
		if err := c.syncState(ctx, state, now); err != nil {
			logging.Warningf(ctx, "Failed to sync instance cache: %s", err)
		}
	}
}

// syncState synchronizes the list of instances in the state file with instance
// files.
//
// Preserves lastAccess of existing instances. Newly discovered files are
// considered last accessed now.
func (c *InstanceCache) syncState(ctx context.Context, state *messages.InstanceCache, now time.Time) error {
	root, err := os.Open(c.FS.Root())
	switch {
	case os.IsNotExist(err):
		state.Entries = nil

	case err != nil:
		return err

	default:
		instanceIDs, err := root.Readdirnames(0)
		if err != nil {
			return err
		}
		existingIDs := stringset.New(len(instanceIDs))
		for _, id := range instanceIDs {
			if common.ValidateInstanceID(id, common.AnyHash) != nil {
				continue
			}
			existingIDs.Add(id)

			if _, ok := state.Entries[id]; !ok {
				if state.Entries == nil {
					state.Entries = map[string]*messages.InstanceCache_Entry{}
				}
				state.Entries[id] = &messages.InstanceCache_Entry{
					LastAccess: timestamppb.New(c.launchTime),
				}
			}
		}

		for id := range state.Entries {
			if !existingIDs.Has(id) {
				delete(state.Entries, id)
			}
		}
	}

	state.LastSynced = timestamppb.New(now)
	logging.Debugf(ctx, "Synchronized instance cache with instance files")
	return nil
}

// saveState persists the cache state.
func (c *InstanceCache) saveState(ctx context.Context, state *messages.InstanceCache) error {
	stateBytes, err := MarshalWithSHA256(state)
	if err != nil {
		return err
	}

	statePath, err := c.FS.RootRelToAbs(instanceCacheStateFilename)
	if err != nil {
		panic(fmt.Sprintf("failed to convert path %q: %v", instanceCacheStateFilename, err))
	}

	return fs.EnsureFile(ctx, c.FS, statePath, bytes.NewReader(stateBytes))
}

// withState loads cache state from the state file, calls f and saves it back.
// See also readState.
func (c *InstanceCache) withState(ctx context.Context, now time.Time, f func(*messages.InstanceCache) (save bool)) {
	if c.Tmp {
		return // no need to keep state.db for temp caches
	}

	state := &messages.InstanceCache{}

	start := c.now(ctx)
	defer func() {
		totalTime := clock.Since(ctx, start)
		if totalTime > 5*time.Second {
			logging.Warningf(
				ctx, "Instance cache state update took %s (%d entries)",
				totalTime, len(state.Entries))
		}
	}()

	c.stateLock.Lock()
	defer c.stateLock.Unlock()

	c.readState(ctx, state, now)
	if f(state) {
		if err := c.saveState(ctx, state); err != nil {
			logging.Warningf(ctx, "Could not save instance cache: %s", err)
		}
	}
}
