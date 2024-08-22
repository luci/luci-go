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
	"container/heap"
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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

const (
	// instanceCacheMaxSize defines how many packages to keep in the cache.
	//
	// When this limit is reached, oldest touched packages are purged.
	//
	// NOTE: There's a bad failure mode if this is too small. Because the cache is
	// LRU if a normal "ensure" scenario involves more than this number of
	// instances, it's possible to have a situation where EVERY ensure operation
	// causes rolling evictions through the cache. See crbug.com/1003774.
	instanceCacheMaxSize = 10_000

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

// InstanceRequest is passed to RequestInstances.
type InstanceRequest struct {
	Context context.Context    // carries the cancellation signal
	Done    context.CancelFunc // called right before the result is enqueued
	Pin     common.Pin         // identifies the instance to fetch
	Open    bool               // true to return it as a pkg.Instance as opposed to pkg.Source
	State   any                // passed to the InstanceResult as is
}

// InstanceResult is a result of a completed InstanceRequest.
type InstanceResult struct {
	Context  context.Context // copied from the InstanceRequest
	Err      error           // non-nil if failed to obtain the instance
	Source   pkg.Source      // set only if Open was false, must be closed by the caller
	Instance pkg.Instance    // set only if Open was true, must be closed by the caller
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
	Fetcher Fetcher

	// ParallelDownloads limits how many parallel fetches can happen at once.
	//
	// The zero value means to do fetches in a blocking way in WaitInstance.
	ParallelDownloads int

	stateLock sync.Mutex // synchronizes access to the state file.

	// true if Launch was called
	launched bool

	fetchPending int32
	fetchReq     chan *InstanceRequest
	fetchRes     chan *InstanceResult // non-nil only if ParallelDownloads > 0
	fetchWG      sync.WaitGroup

	// Fetch tokens implement a semaphore that gradually ramps up the download
	// concurrency.
	fetchTokensM    sync.Mutex
	fetchTokensC    int
	fetchTokensDone bool
	fetchTokens     chan struct{}

	// Defaults to instanceCacheMaxSize, mocked in tests.
	maxSize int
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
		}
	}

	return err
}

// Launch prepares the cache for fetching packages in background.
//
// Must be called before RequestInstances.
func (c *InstanceCache) Launch(ctx context.Context) {
	if c.ParallelDownloads < 0 {
		panic("ParallelDownloads must be non-negative")
	}

	if c.launched {
		panic("Called InstanceCache.Launch more than once")
	}
	c.launched = true

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
		// ParallelDownloads with each finished download, see grabConcurrencySlot.
		// This is a heuristic to make sure we download the first package ASAP, so
		// the installer has something to install while more packages are being
		// downloaded.
		c.fetchTokens = make(chan struct{}, c.ParallelDownloads)
		c.fetchTokens <- struct{}{}
		c.fetchTokensC = 1

		c.fetchWG.Add(c.ParallelDownloads)
		for i := 0; i < c.ParallelDownloads; i++ {
			go func() {
				defer c.fetchWG.Done()

				handleOne := func() bool {
					defer c.grabConcurrencySlot()()
					req, ok := <-c.fetchReq
					if !ok {
						return false
					}
					c.fetchRes <- c.handleRequest(req)
					return true
				}

				for handleOne() {
					// Keep spinning until c.fetchReq is closed.
				}
			}()
		}
	}
}

// Close shuts down goroutines started in Launch and cleans up temp files.
//
// The caller should ensure there's no pending fetches before making this call.
func (c *InstanceCache) Close(ctx context.Context) {
	if !c.launched {
		// no-op if InstanceCache never launched.
		return
	}

	if c.HasPendingFetches() {
		panic("closing an InstanceCache with some fetches still pending")
	}

	close(c.fetchReq)
	if c.ParallelDownloads > 0 {
		c.unblockAllConcurrencySlots()
		c.fetchWG.Wait()
	}

	// Self-destruct if the cache was temporary.
	if c.Tmp {
		if err := os.RemoveAll(c.FS.Root()); err != nil {
			logging.Warningf(ctx, "Leaking temp directory %s: %s", c.FS.Root(), err)
		}
	}
}

// RequestInstances enqueues requests to fetch instances into the cache.
//
// The results can be waited upon using WaitInstance() method. Each enqueued
// InstanceRequest will get an exactly one InstanceResponse (perhaps with an
// error inside). The order of responses may not match the order of requests.
func (c *InstanceCache) RequestInstances(ctx context.Context, reqs []*InstanceRequest) {
	now := clock.Now(ctx)
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

	atomic.AddInt32(&c.fetchPending, int32(len(reqs)))
	for _, r := range reqs {
		c.fetchReq <- r
	}
}

// HasPendingFetches is true if there's some uncompleted InstanceRequest whose
// completion notification will (eventually) unblock WaitInstance.
func (c *InstanceCache) HasPendingFetches() bool {
	return atomic.LoadInt32(&c.fetchPending) > 0
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
	atomic.AddInt32(&c.fetchPending, -1)
	return
}

// grabConcurrencySlot is called right before processing a fetch request.
//
// It implements the gradual ramp up of download parallelism. It blocks until
// the request is allowed to run.
func (c *InstanceCache) grabConcurrencySlot() (done func()) {
	_, ok := <-c.fetchTokens
	if !ok {
		return func() {} // we are exiting
	}

	return func() {
		c.fetchTokensM.Lock()
		defer c.fetchTokensM.Unlock()
		if c.fetchTokensDone {
			return
		}

		// Return the token we grabbed.
		c.fetchTokens <- struct{}{}

		// If still ramping up, add one more token to increase the concurrency.
		if c.fetchTokensC < c.ParallelDownloads {
			c.fetchTokensC++
			c.fetchTokens <- struct{}{}
		}
	}
}

// unblockAllConcurrencySlots is called when closing.
func (c *InstanceCache) unblockAllConcurrencySlots() {
	c.fetchTokensM.Lock()
	defer c.fetchTokensM.Unlock()
	c.fetchTokensDone = true
	close(c.fetchTokens)
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
	case !req.Open:
		res.Source = src
	default:
		instance, err := reader.OpenInstance(ctx, src, reader.OpenInstanceOpts{
			VerificationMode: reader.SkipHashVerification, // the fetcher did it already
			InstanceID:       pin.InstanceID,
		})
		if err != nil {
			src.Close(ctx, reader.IsCorruptionError(err))
			res.Err = err
		} else {
			res.Instance = instance
		}
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
		return nil, errors.Annotate(err, "checking size").Tag(cipderr.IO).Err()
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
		return nil, errors.Reason("invalid instance ID %q", pin.InstanceID).Tag(cipderr.BadArgument).Err()
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
			c.touch(ctx, pin.InstanceID, false) // bump its last access time
			return file, nil
		}

		// No such cached instance, download it.
		err := c.FS.EnsureFile(ctx, path, func(f *os.File) error {
			return c.Fetcher(ctx, pin, f)
		})
		if err != nil {
			if cipderr.ToCode(err) == cipderr.Unknown {
				err = errors.Annotate(err, "writing to instance cache").Tag(cipderr.IO).Err()
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
			return nil, errors.Annotate(err, "pin %s is unexpectedly missing from the cache", pin).Tag(cipderr.IO).Err()
		case err != nil:
			logging.Errorf(ctx, "Failed to open the instance %s: %s", pin, err)
			return nil, errors.Annotate(err, "opening the cached instance %s", pin).Tag(cipderr.IO).Err()
		default:
			c.touch(ctx, pin.InstanceID, true) // mark it as accessed, run the GC
			return file, nil
		}
	}
}

// touch updates the cache state file (best effort).
func (c *InstanceCache) touch(ctx context.Context, instanceID string, gc bool) {
	now := clock.Now(ctx)
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
		// Optionally collect old entries.
		if gc {
			c.gc(ctx, s, now)
		}
		return true
	})
}

// delete deletes an instance from the cache if it was there.
func (c *InstanceCache) delete(ctx context.Context, pin common.Pin) error {
	path, err := c.FS.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return errors.Reason("invalid instance ID %q", pin.InstanceID).Tag(cipderr.BadArgument).Err()
	}

	if err := c.FS.EnsureFileGone(ctx, path); err != nil {
		return errors.Annotate(err, "deleting the cached instance file").Tag(cipderr.IO).Err()
	}

	c.withState(ctx, clock.Now(ctx), func(s *messages.InstanceCache) (save bool) {
		if s.Entries[pin.InstanceID] == nil {
			return false
		}
		delete(s.Entries, pin.InstanceID)
		return true
	})

	return nil
}

// GC opportunistically purges entries that haven't been touched for too long.
func (c *InstanceCache) GC(ctx context.Context) {
	now := clock.Now(ctx)
	c.withState(ctx, now, func(s *messages.InstanceCache) (save bool) {
		c.gc(ctx, s, now)
		return true
	})
}

type garbageCandidate struct {
	instanceID     string
	lastAccessTime time.Time
}

type garbageHeap []*garbageCandidate

func (h garbageHeap) Len() int           { return len(h) }
func (h garbageHeap) Less(i, j int) bool { return h[i].lastAccessTime.Before(h[j].lastAccessTime) }
func (h garbageHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *garbageHeap) Push(x any) {
	*h = append(*h, x.(*garbageCandidate))
}
func (h *garbageHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil // For GC.
	*h = old[0 : n-1]
	return x
}

// gc cleans up the old instances.
//
// There are two cleanup polices acting at the same time:
//  1. Instances that haven't been touched for too long are removed.
//  2. If the number of instances in the state is greater than maximum, oldest
//     instances are removed.
func (c *InstanceCache) gc(ctx context.Context, state *messages.InstanceCache, now time.Time) {
	maxAge := c.maxAge
	if maxAge == 0 {
		maxAge = instanceCacheMaxAge
	}
	maxSize := c.maxSize
	if maxSize == 0 {
		maxSize = instanceCacheMaxSize
	}

	// Kick out entries older than some threshold first.
	garbage := stringset.New(0)
	for instanceID, e := range state.Entries {
		age := now.Sub(e.LastAccess.AsTime())
		if age > maxAge {
			garbage.Add(instanceID)
			logging.Debugf(ctx, "Purging cached instance %q (age %s)", instanceID, age)
		}
	}

	// If still have too many entries, kick out oldest. Ignore entries already
	// designated as garbage.
	moreGarbage := len(state.Entries) - garbage.Len() - maxSize
	if moreGarbage > 0 {
		logging.Debugf(ctx, "Still need to purge %d cached instance(s)", moreGarbage)
		g := make(garbageHeap, 0, len(state.Entries)-garbage.Len())
		for instanceID, e := range state.Entries {
			if !garbage.Has(instanceID) {
				g = append(g, &garbageCandidate{
					instanceID:     instanceID,
					lastAccessTime: e.LastAccess.AsTime(),
				})
			}
		}
		heap.Init(&g)
		for i := 0; i < moreGarbage; i++ {
			item := heap.Pop(&g).(*garbageCandidate)
			garbage.Add(item.instanceID)
			logging.Debugf(ctx, "Purging cached instance %q (age %s)", item.instanceID, now.Sub(item.lastAccessTime))
		}
	}

	garbage.Iter(func(instanceID string) bool {
		path, err := c.FS.RootRelToAbs(instanceID)
		if err != nil {
			panic(fmt.Sprintf("failed to convert path %q: %v", instanceID, err))
		}
		// EnsureFileGone logs errors already.
		if c.FS.EnsureFileGone(ctx, path) == nil {
			delete(state.Entries, instanceID)
		}
		return true
	})

	c.FS.CleanupTrash(ctx)
}

// readState loads cache state from the state file.
// If the file does not exist, corrupted or its state was not synchronized
// with the instance files for a long time, synchronizes it.
// Newly discovered files are considered last accessed at zero time.
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
		c.gc(ctx, state, now)
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
					LastAccess: timestamppb.New(now),
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

	start := clock.Now(ctx)
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
