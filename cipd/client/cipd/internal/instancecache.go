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
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
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
	instanceCacheMaxSize = 500

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
	Context context.Context // carries the cancellation signal
	Pin     common.Pin      // identifies the instance to fetch
	Open    bool            // true to return it as a pkg.Instance as opposed to pkg.Source
	State   interface{}     // passed to the InstanceResult as is
}

// InstanceResult is a result of a completed InstanceRequest.
type InstanceResult struct {
	Context  context.Context // copied from the InstanceRequest
	Err      error           // non-nil if failed to obtain the instance
	Source   pkg.Source      // set only if Open was false, must be closed by the caller
	Instance pkg.Instance    // set only if Open was true, must be closed by the caller
	State    interface{}     // copied from the InstanceRequest
}

// InstanceCache is a file-system-based, thread-safe, LRU cache of instances.
//
// It also knowns how to populate the cache using the given Fetcher callback,
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
	// Currently ignored.
	ParallelDownloads int

	stateLock sync.Mutex // synchronizes access to the state file.

	allocsLock sync.Mutex
	allocs     map[string]*Allocation // keyed by instance ID

	fetchQueue   chan func() *InstanceResult
	fetchPending int32

	// Defaults to instanceCacheMaxSize, mocked in tests.
	maxSize int
	// Defaults to instanceCacheMaxAge, mocked in tests.
	maxAge time.Duration
}

// Allocation is used to concurrently fetch files into the cache.
//
// It is a future/promise-like object keyed by instance ID that resolves into
// a pkg.Source with that instance's data in the cache.
//
// Multiple callers can get a reference to the same allocation and call
// `Realize` on it concurrently to get the cached file. The work will be done
// only once, all pending `Realize` calls will get the same result.
type Allocation struct {
	cache *InstanceCache // the parent
	pin   common.Pin     // the key of this allocation in the parent's map

	m        sync.Mutex
	refs     int           // the reference counter
	corrupt  bool          // true if the file was closed as corrupted
	realized bool          // true if `Realize` was already called
	resolved chan struct{} // closed after `resolve` is completed
	file     *os.File      // set only after `resolve` is completed
	err      error         // set only after `resolve` is completed
	size     int64         // set only after `resolve` is completed
}

// Realize blocks until the cached file is available.
func (a *Allocation) Realize(ctx context.Context) (pkg.Source, error) {
	a.m.Lock()
	owner := !a.realized
	a.realized = true
	a.m.Unlock()

	var err error
	if owner {
		err = a.resolve(a.cache.openOrFetch(ctx, a.pin))
	} else {
		err = a.wait(ctx)
	}

	if err != nil {
		return nil, err
	}

	a.m.Lock()
	defer a.m.Unlock()

	// Will be decremented when the cacheFile below is closed.
	a.refs++

	return &cacheFile{
		alloc: a,
		file:  a.file,
		size:  a.size,
	}, nil
}

// Release decrements the reference counter on this allocation.
//
// Once it reaches zero the allocation is "forgotten" and creating it again
// may result in a refetch of the file into the cache.
func (a *Allocation) Release(ctx context.Context) {
	_ = a.releaseAndClose(ctx, false)
}

// addRef increments the reference counter.
func (a *Allocation) addRef() {
	a.m.Lock()
	a.refs++
	a.m.Unlock()
}

// resolve is called once the file is downloaded (or failed to be downloaded).
//
// Unblocks all pending and future `wait` calls.
func (a *Allocation) resolve(f *os.File, err error) error {
	var stat os.FileInfo
	if err == nil {
		if stat, err = f.Stat(); err != nil {
			f.Close()
			f = nil
		}
	}

	a.m.Lock()
	defer a.m.Unlock()

	a.file = f
	a.err = err
	if err == nil {
		a.size = stat.Size()
	}
	close(a.resolved)

	return err
}

// wait waits until `resolve` is called elsewhere (or the context deadlines).
//
// Either returns the same error as `resolve` or a context error.
func (a *Allocation) wait(ctx context.Context) error {
	select {
	case <-a.resolved:
		a.m.Lock()
		defer a.m.Unlock()
		return a.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseAndClose decrements the reference counter and closes the file.
func (a *Allocation) releaseAndClose(ctx context.Context, corrupt bool) error {
	a.m.Lock()
	a.refs--
	done := a.refs == 0
	if done {
		a.cache.forgetAlloc(a)
	}
	a.corrupt = a.corrupt || corrupt
	corrupt = a.corrupt
	file := a.file
	a.m.Unlock()

	if !done || file == nil {
		return nil
	}

	corruptText := ""
	if corrupt {
		corruptText = " corrupt"
	}

	var err error
	if err = file.Close(); err != nil && err != os.ErrClosed {
		logging.Warningf(ctx, "cipd: failed to close%s cache file: %s", corruptText, err)
	} else {
		err = nil
	}

	if corrupt || a.cache.Tmp {
		if err2 := a.cache.delete(ctx, a.pin); err2 != nil {
			logging.Warningf(ctx, "cipd: failed to delete%s cache file: %s", corruptText, err2)
			if err == nil {
				err = err2
			}
		}
	}

	return err
}

// cacheFile is a downloaded instance file, implements pkg.Source.
//
// Multiple cacheFile instances may all point to the same *os.File (owned by
// some single `alloc`).
type cacheFile struct {
	closed int32       // 1 if was already closed
	alloc  *Allocation // the parent allocation
	file   *os.File    // the underlying file owned by `alloc`
	size   int64       // the size of `file`
}

func (f *cacheFile) ReadAt(p []byte, off int64) (int, error) { return f.file.ReadAt(p, off) }
func (f *cacheFile) Size() int64                             { return f.size }

func (f *cacheFile) Close(ctx context.Context, corrupt bool) error {
	if atomic.CompareAndSwapInt32(&f.closed, 0, 1) {
		return f.alloc.releaseAndClose(ctx, corrupt)
	}
	return os.ErrClosed
}

// Launch prepares the cache for fetching packages in background.
//
// Must be called before RequestInstance.
func (c *InstanceCache) Launch(ctx context.Context) {
	tmp := ""
	if c.Tmp {
		tmp = " temporary"
	}
	logging.Infof(ctx, "cipd: using%s instance cache at %q", tmp, c.FS.Root())

	c.fetchQueue = make(chan func() *InstanceResult, 100000)
}

// Close shuts down goroutines started in Launch and cleans up temp files.
func (c *InstanceCache) Close(ctx context.Context) {
	close(c.fetchQueue)

	if c.Tmp {
		if err := os.RemoveAll(c.FS.Root()); err != nil {
			logging.Warningf(ctx, "cipd: leaking temp directory %s: %s", c.FS.Root(), err)
		}
	}
}

// RequestInstances enqueues requests to fetch instances into the cache.
//
// The results can be waited upon using WaitInstance() method. Each enqueued
// InstanceRequest will get an exactly one InstanceResponse (perhaps with an
// error inside). The order of responses may not match order of requests.
func (c *InstanceCache) RequestInstances(reqs []*InstanceRequest) {
	// TODO(vadimsh): Rewrite not to use Allocation, use a worker pool to control
	// concurrency, etc. This is just a MVP implementation of the interface to
	// make sure cipd.Client is comfortable using it.

	atomic.AddInt32(&c.fetchPending, int32(len(reqs)))

	for _, req := range reqs {
		ctx := req.Context
		pin := req.Pin
		open := req.Open
		state := req.State

		c.fetchQueue <- func() *InstanceResult {
			res := &InstanceResult{Context: ctx, State: state}

			alloc := c.Allocate(ctx, pin)
			defer alloc.Release(ctx)

			switch src, err := alloc.Realize(ctx); {
			case err != nil:
				res.Err = err
			case !open:
				res.Source = src
			default:
				instance, err := reader.OpenInstance(ctx, src, reader.OpenInstanceOpts{
					VerificationMode: reader.SkipHashVerification, // the cache did it already
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
	}
}

// HasPendingFetches is true if there's some uncompleted InstanceRequest whose
// completion notification will (eventually) unblock WaitInstance.
func (c *InstanceCache) HasPendingFetches() bool {
	return atomic.LoadInt32(&c.fetchPending) > 0
}

// WaitInstance blocks until some InstanceRequest is available.
func (c *InstanceCache) WaitInstance() *InstanceResult {
	// TODO(vadimsh): This is really just a serial execution for now to reproduce
	// the previous non-parallel behavior.
	r := (<-c.fetchQueue)()
	atomic.AddInt32(&c.fetchPending, -1)
	return r
}

// Allocate returns a reference to a current or future cached instance.
//
// Use `Realize` to get the actual package file. Call `Release` once the
// allocation is no longer needed.
func (c *InstanceCache) Allocate(ctx context.Context, pin common.Pin) *Allocation {
	c.allocsLock.Lock()
	defer c.allocsLock.Unlock()

	alloc := c.allocs[pin.InstanceID]
	if alloc != nil {
		alloc.addRef()
		return alloc
	}

	if c.allocs == nil {
		c.allocs = make(map[string]*Allocation, 1)
	}

	alloc = &Allocation{
		cache:    c,
		pin:      pin,
		refs:     1,
		resolved: make(chan struct{}),
	}
	c.allocs[pin.InstanceID] = alloc

	return alloc
}

// forgetAlloc removes `a` from `allocs`.
func (c *InstanceCache) forgetAlloc(a *Allocation) {
	c.allocsLock.Lock()
	defer c.allocsLock.Unlock()
	if c.allocs[a.pin.InstanceID] == a {
		delete(c.allocs, a.pin.InstanceID)
	}
}

// openOrFetch either opens an existing instance file or writes a new one.
func (c *InstanceCache) openOrFetch(ctx context.Context, pin common.Pin) (*os.File, error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return nil, err
	}

	path, err := c.FS.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("invalid instance ID %q", pin.InstanceID)
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
			logging.Warningf(ctx, "cipd: could not get %s from cache - %s", pin, err)
		default:
			logging.Infof(ctx, "cipd: instance cache hit for %s", pin)
			c.touch(ctx, pin.InstanceID, false) // bump its last access time
			return file, nil
		}

		// No such cached instance, download it.
		err := c.FS.EnsureFile(ctx, path, func(f *os.File) error {
			return c.Fetcher(ctx, pin, f)
		})
		if err != nil {
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
		file, err := c.FS.OpenFile(path)
		if err != nil {
			logging.Errorf(ctx, "cipd: %s is unexpectedly missing from the cache: %s", pin, err)
			if attempt == 1 {
				logging.Infof(ctx, "cipd: retrying...")
				continue
			}
			logging.Errorf(ctx, "cipd: giving up")
			return nil, err
		}
		c.touch(ctx, pin.InstanceID, true) // mark it as accessed, run the GC
		return file, nil
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
		return fmt.Errorf("invalid instance ID %q", pin.InstanceID)
	}

	if err := c.FS.EnsureFileGone(ctx, path); err != nil {
		return err
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
func (h *garbageHeap) Push(x interface{}) {
	*h = append(*h, x.(*garbageCandidate))
}
func (h *garbageHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// gc cleans up the old instances.
//
// There are two cleanup polices acting at the same time:
//   1. Instances that haven't been touched for too long are removed.
//   2. If the number of instances in the state is greater than maximum, oldest
//      instances are removed.
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
			logging.Infof(ctx, "cipd: purging cached instance %s (age %s)", instanceID, age)
		}
	}

	// If still have too many entries, kick out oldest. Ignore entries already
	// designated as garbage.
	moreGarbage := len(state.Entries) - garbage.Len() - maxSize
	if moreGarbage > 0 {
		logging.Infof(ctx, "cipd: still need to purge %d cached instance(s)", moreGarbage)
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
			logging.Infof(ctx, "cipd: purging cached instance %s (age %s)", item.instanceID, now.Sub(item.lastAccessTime))
		}
	}

	garbage.Iter(func(instanceID string) bool {
		path, err := c.FS.RootRelToAbs(instanceID)
		if err != nil {
			panic("impossible")
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
		panic("impossible")
	}

	stateBytes, err := ioutil.ReadFile(statePath)
	sync := false
	switch {
	case os.IsNotExist(err):
		sync = true

	case err != nil:
		logging.Warningf(ctx, "cipd: could not read instance cache - %s", err)
		sync = true

	default:
		if err := UnmarshalWithSHA256(stateBytes, state); err != nil {
			if err == ErrUnknownSHA256 {
				logging.Warningf(ctx, "cipd: need to rebuild instance cache index file")
			} else {
				logging.Warningf(ctx, "cipd: instance cache file is corrupted - %s", err)
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
			logging.Warningf(ctx, "cipd: failed to sync instance cache - %s", err)
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
	logging.Infof(ctx, "cipd: synchronized instance cache with instance files")
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
		panic("impossible")
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
		if totalTime > 2*time.Second {
			logging.Warningf(
				ctx, "cipd: instance cache state update took %s (%d entries)",
				totalTime, len(state.Entries))
		}
	}()

	c.stateLock.Lock()
	defer c.stateLock.Unlock()

	c.readState(ctx, state, now)
	if f(state) {
		if err := c.saveState(ctx, state); err != nil {
			logging.Warningf(ctx, "cipd: could not save instance cache - %s", err)
		}
	}
}
