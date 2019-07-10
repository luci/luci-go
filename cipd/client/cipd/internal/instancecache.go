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
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
)

const (
	// instanceCacheMaxSize defines how many packages to keep in the cache.
	//
	// When this limit is reached, oldest touched packages are purged.
	instanceCacheMaxSize = 100

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

// InstanceCache is a file-system-based, thread-safe, LRU cache of instances.
//
// Does not validate instance hashes; it is caller's responsibility.
type InstanceCache struct {
	fs        fs.FileSystem
	stateLock sync.Mutex // synchronizes access to the state file.

	// Defaults to instanceCacheMaxSize, mocked in tests.
	maxSize int
	// Defaults to instanceCacheMaxAge, mocked in tests.
	maxAge time.Duration
}

// NewInstanceCache initializes InstanceCache.
//
// fs will be the root of the cache.
func NewInstanceCache(fs fs.FileSystem) *InstanceCache {
	return &InstanceCache{
		fs:      fs,
		maxSize: instanceCacheMaxSize,
		maxAge:  instanceCacheMaxAge,
	}
}

type cacheFile struct {
	*os.File
	fs fs.FileSystem
}

// Close removes this file from the cache if corrupt is true.
//
// This will be true if the cached file is determined to be broken at a higher
// level.
//
// This implements pkg.Source.
func (f *cacheFile) Close(ctx context.Context, corrupt bool) error {
	var err error
	if err = f.File.Close(); err != nil && err != os.ErrClosed {
		corruptText := ""
		if corrupt {
			corruptText = " corrupt"
		}
		logging.WithError(err).Warningf(ctx, "failed to close%s cache file", corruptText)
	} else {
		err = nil
	}
	if corrupt {
		var err2 error
		if err2 = f.fs.EnsureFileGone(ctx, f.Name()); err2 != nil {
			logging.WithError(err2).Warningf(ctx, "failed to delete corrupt cache file")
		}
		// only return err2 if err was already nil
		if err == nil {
			err = err2
		}
	}
	return err
}

// UnderlyingFile is only used by tests and shouldn't be used directly.
func (f *cacheFile) UnderlyingFile() *os.File {
	return f.File
}

// Get searches for the instance in the cache and opens it for reading.
//
// If the instance is not found, returns an os.IsNotExists error.
func (c *InstanceCache) Get(ctx context.Context, pin common.Pin, now time.Time) (pkg.Source, error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return nil, err
	}

	path, err := c.fs.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("invalid instance ID %q", pin.InstanceID)
	}

	f, err := c.fs.OpenFile(path)
	if err != nil {
		return nil, err
	}

	c.withState(ctx, now, func(s *messages.InstanceCache) {
		touch(s, pin.InstanceID, now)
	})

	return &cacheFile{f, c.fs}, nil
}

// Put caches an instance.
//
// write must write the instance contents. May remove some instances from the
// cache that were not accessed for a long time.
func (c *InstanceCache) Put(ctx context.Context, pin common.Pin, now time.Time, write func(*os.File) error) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	path, err := c.fs.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return fmt.Errorf("invalid instance ID %q", pin.InstanceID)
	}

	if err := c.fs.EnsureFile(ctx, path, write); err != nil {
		return err
	}

	c.withState(ctx, now, func(s *messages.InstanceCache) {
		touch(s, pin.InstanceID, now)
		c.gc(ctx, s, now)
	})
	return nil
}

// GC opportunistically purges entries that haven't been touched for too long.
func (c *InstanceCache) GC(ctx context.Context, now time.Time) {
	c.withState(ctx, now, func(s *messages.InstanceCache) {
		c.gc(ctx, s, now)
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
	// Kick out entries older than some threshold first.
	garbage := stringset.New(0)
	for instanceID, e := range state.Entries {
		age := now.Sub(google.TimeFromProto(e.LastAccess))
		if age > c.maxAge {
			garbage.Add(instanceID)
			logging.Infof(ctx, "cipd: purging cached instance %s (age %s)", instanceID, age)
		}
	}

	// If still have too many entries, kick out oldest. Ignore entries already
	// designated as garbage.
	moreGarbage := len(state.Entries) - garbage.Len() - c.maxSize
	if moreGarbage > 0 {
		logging.Infof(ctx, "cipd: still need to purge %d cached instance(s)", moreGarbage)
		g := make(garbageHeap, 0, len(state.Entries)-garbage.Len())
		for instanceID, e := range state.Entries {
			if !garbage.Has(instanceID) {
				g = append(g, &garbageCandidate{
					instanceID:     instanceID,
					lastAccessTime: google.TimeFromProto(e.LastAccess),
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
		path, err := c.fs.RootRelToAbs(instanceID)
		if err != nil {
			panic("impossible")
		}
		// EnsureFileGone logs errors already.
		if c.fs.EnsureFileGone(ctx, path) == nil {
			delete(state.Entries, instanceID)
		}
		return true
	})

	c.fs.CleanupTrash(ctx)
}

// readState loads cache state from the state file.
// If the file does not exist, corrupted or its state was not synchronized
// with the instance files for a long time, synchronizes it.
// Newly discovered files are considered last accessed at zero time.
// If synchronization fails, then the state is considered empty.
func (c *InstanceCache) readState(ctx context.Context, state *messages.InstanceCache, now time.Time) {
	statePath, err := c.fs.RootRelToAbs(instanceCacheStateFilename)
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
			sync = google.TimeFromProto(state.LastSynced).Before(cutOff)
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
	root, err := os.Open(c.fs.Root())
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
					LastAccess: google.NewTimestamp(now),
				}
			}
		}

		for id := range state.Entries {
			if !existingIDs.Has(id) {
				delete(state.Entries, id)
			}
		}
	}

	state.LastSynced = google.NewTimestamp(now)
	logging.Infof(ctx, "cipd: synchronized instance cache with instance files")
	return nil
}

// saveState persists the cache state.
func (c *InstanceCache) saveState(ctx context.Context, state *messages.InstanceCache) error {
	stateBytes, err := MarshalWithSHA256(state)
	if err != nil {
		return err
	}

	statePath, err := c.fs.RootRelToAbs(instanceCacheStateFilename)
	if err != nil {
		panic("impossible")
	}

	return fs.EnsureFile(ctx, c.fs, statePath, bytes.NewReader(stateBytes))
}

// withState loads cache state from the state file, calls f and saves it back.
// See also readState.
func (c *InstanceCache) withState(ctx context.Context, now time.Time, f func(*messages.InstanceCache)) {
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
	f(state)
	if err := c.saveState(ctx, state); err != nil {
		logging.Warningf(ctx, "cipd: could not save instance cache - %s", err)
	}
}

// getAccessTime returns last access time of an instance.
// Used for testing.
func (c *InstanceCache) getAccessTime(ctx context.Context, now time.Time, pin common.Pin) (lastAccess time.Time, ok bool) {
	c.withState(ctx, now, func(s *messages.InstanceCache) {
		var entry *messages.InstanceCache_Entry
		if entry, ok = s.Entries[pin.InstanceID]; ok {
			lastAccess = google.TimeFromProto(entry.LastAccess)
		}
	})
	return
}

// touch updates/adds last access time for an instance.
func touch(state *messages.InstanceCache, instanceID string, now time.Time) {
	entry := state.Entries[instanceID]
	if entry == nil {
		entry = &messages.InstanceCache_Entry{}
		if state.Entries == nil {
			state.Entries = map[string]*messages.InstanceCache_Entry{}
		}
		state.Entries[instanceID] = entry
	}
	entry.LastAccess = google.NewTimestamp(now)
}
