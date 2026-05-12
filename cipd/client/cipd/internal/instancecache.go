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
	"cmp"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
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
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

const (
	// DefaultInstanceCacheMaxAge defines when to purge a cached package that is
	// not being used.
	//
	// Works in parallel with 'instanceCacheMaxSize' limit.
	DefaultInstanceCacheMaxAge = 48 * time.Hour

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

// PassiveWritePolicy describes how this InstanceCache can passively write to
// disk. By default, reading from the instance cache is allowed to write
// under certain circumstances.
type PassiveWritePolicy int

const (
	// DisableGC causes the InstanceCache to never do garbage collection.
	DisableGC PassiveWritePolicy = 1 << iota

	// DisableCorruptDeletion causes the InstanceCache to never delete corrupted
	// instance files.
	DisableCorruptDeletion

	// DisableStateWrite causes the InstanceCache to never write the state.db
	// file (even after sync'ing state)
	DisableStateWrite

	// DisablePassiveWrites disables all passive writing capabilities of the
	// InstanceCache.
	DisablePassiveWrites = DisableGC | DisableCorruptDeletion | DisableStateWrite
)

func (p PassiveWritePolicy) has(item PassiveWritePolicy) bool {
	return p&item == item
}

func (p PassiveWritePolicy) String() string {
	if p == 0 {
		return ""
	}

	toks := make([]string, 0, 3)
	if p.has(DisableGC) {
		toks = append(toks, "no-gc")
	}
	if p.has(DisableCorruptDeletion) {
		toks = append(toks, "no-corrupt-cleanup")
	}
	if p.has(DisableStateWrite) {
		toks = append(toks, "no-state-write")
	}
	if leftover := p &^ DisablePassiveWrites; leftover != 0 {
		toks = append(toks, fmt.Sprintf("UNKNOWN[%b]", leftover))
	}
	return strings.Join(toks, "|")
}

// GCOlderThanLaunch indicates that the InstanceCache should garbage
// collect all instances which were older than the time that this InstanceCache
// was Launch'd.
//
// Typically this is used when the InstanceCache is the only one open for a
// given FS, and thus this has the effect of removing all instances which are
// not requested from this cache.
const GCOlderThanLaunch time.Duration = -1

// InstanceCache is a file-system-based cache of instances.
//
// It optionally knows how to populate the cache using the given Fetcher
// callback.
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

	// ExactGC causes the garbage collection to collect all instances which were
	// not requested by this cache, and disables automatic collection as part of
	// state synchronization (so: the caller setting ExactGC must explicitly call
	// GC() when all done with this cache). This is internally modeled as setting
	// the maxAge to zero.
	ExactGC bool

	// GCMaxAge causes the garbage collection to collect all instances which were
	// not requested from this cache within this time period.
	//
	// If this is exactly zero, it defaults to [DefaultInstanceCacheMaxAge].
	//
	// If this is [GCOlderThanLaunch], the GC will remove all instances
	// added before this cache was Launch'd.
	GCMaxAge time.Duration

	// GCLaunchTime, if set, will override the launch time for this
	// InstanceCache.
	//
	// This will have the effect of only GC'ing instances before this timestamp
	// (which may be substantially before the cache is actually launched).
	GCLaunchTime time.Time

	// PassiveWritePolicy adjusts how this InstanceCache is allowed to write
	// passively to disk. If omitted, the InstanceCache assumes full ability to
	// write passively.
	PassiveWritePolicy PassiveWritePolicy

	stateLock sync.Mutex // synchronizes access to the state file.

	// launchTime is the time at which OpenAsSource or Touch was first called;
	// this is used to evaluate if instances are too old during garbage
	// collection.
	//
	// If GCLaunchTime is set, this will be a copy of GCLaunchTime.
	launchTime time.Time
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
	if !c.launchTime.IsZero() {
		return
	}
	if c.GCLaunchTime.IsZero() {
		c.launchTime = clock.Now(ctx)
	} else {
		c.launchTime = c.GCLaunchTime
	}

	tmp := ""
	if c.Tmp {
		tmp = " temporary"
	}
	policy := ""
	if c.PassiveWritePolicy != 0 {
		policy = fmt.Sprintf(" (%s)", c.PassiveWritePolicy.String())
	}
	logging.Infof(ctx, "Using%s%s instance cache at %s", tmp, policy, c.FS.Root())
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

// Close cleans up temp files and does a garbage collection pass.
//
// Resets this InstanceCache to its initial state.
func (c *InstanceCache) Close(ctx context.Context) {
	if c.launchTime.IsZero() {
		return
	}

	// Self-destruct if the cache was temporary.
	if c.Tmp {
		if err := os.RemoveAll(c.FS.Root()); err != nil {
			logging.Warningf(ctx, "Leaking temp directory %s: %s", c.FS.Root(), err)
		}
	} else {
		// Do a garbage collection pass.
		c.withState(ctx, c.now(ctx), false, func(state *messages.InstanceCache) (save bool) {
			c.gc(ctx, state)
			return true
		})
	}

	// Finally, reset state.
	c.launchTime = time.Time{}
}

// OpenAsSource returns the instance file as a pkg.Source.
//
// If the on-disk cache does not contain this instance, it will be fetched with
// Fetcher.
//
// This is fully synchronous - fetching (if needed) will happen inline with
// this call.
//
// If you need to access multiple instances concurrently, see
// [ManagedInstanceCache].
func (c *InstanceCache) OpenAsSource(ctx context.Context, pin common.Pin) (pkg.Source, error) {
	c.maybeLaunch(ctx)

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
			c.Touch(ctx, pin.InstanceID) // Bump its last access time.
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
			c.Touch(ctx, pin.InstanceID) // Mark it as accessed.
			return file, nil
		}
	}
}

// Touch resets the LastAccess time of `instanceIDs` to `now`.
//
// This will touch instances regardless of the existence of the instanceID in
// the cache. This can be used prior to [InstanceCache.OpenAsSource] to
// speculatively reset the access time before checking to see if the instance
// is actually present on disk.
func (c *InstanceCache) Touch(ctx context.Context, instanceIDs ...string) {
	c.maybeLaunch(ctx)

	now := c.now(ctx)
	c.withState(ctx, now, false, func(s *messages.InstanceCache) (save bool) {
		for _, iid := range instanceIDs {
			// Update LastAccess time.
			entry := s.Entries[iid]
			if entry == nil {
				entry = &messages.InstanceCache_Entry{}
				if s.Entries == nil {
					s.Entries = map[string]*messages.InstanceCache_Entry{}
				}
				s.Entries[iid] = entry
			}
			entry.LastAccess = timestamppb.New(now)
		}
		return true
	})
}

// delete deletes an instance from the cache if it was there.
func (c *InstanceCache) delete(ctx context.Context, pin common.Pin) error {
	// If the cache is configured to skip corrupt cleanup, do so here.
	//
	// If the cache is temporary, then ignore this.
	if !c.Tmp && c.PassiveWritePolicy.has(DisableCorruptDeletion) {
		return nil
	}

	path, err := c.FS.RootRelToAbs(pin.InstanceID)
	if err != nil {
		return cipderr.BadArgument.Apply(errors.Fmt("invalid instance ID %q", pin.InstanceID))
	}

	if err := c.FS.EnsureFileGone(ctx, path); err != nil {
		return cipderr.IO.Apply(errors.Fmt("deleting the cached instance file: %w", err))
	}

	c.withState(ctx, c.now(ctx), false, func(s *messages.InstanceCache) (save bool) {
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
	if c.PassiveWritePolicy.has(DisableGC) {
		return
	}

	// Values less than 0, including GCOlderThanLaunch, become 0.
	var maxAge time.Duration
	if !c.ExactGC {
		maxAge = max(cmp.Or(c.GCMaxAge, DefaultInstanceCacheMaxAge), 0)
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
//
// If forceSync is given, it will force a resync with the state on disc, and
// any failure to sync will return an error.
func (c *InstanceCache) readState(ctx context.Context, state *messages.InstanceCache, now time.Time, forceSync bool) error {
	sync := forceSync
	if !sync {
		statePath, err := c.FS.RootRelToAbs(instanceCacheStateFilename)
		if err != nil {
			panic(fmt.Sprintf("failed to convert path %q: %v", instanceCacheStateFilename, err))
		}

		stateBytes, err := os.ReadFile(statePath)
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
	}

	if sync {
		if err := c.syncState(ctx, state, now); err != nil {
			if forceSync {
				return errors.Fmt("syncing instance cache: %w", err)
			}
			logging.Warningf(ctx, "Failed to sync instance cache: %s", err)
		}
	}

	return nil
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
	if c.PassiveWritePolicy.has(DisableStateWrite) {
		return nil
	}

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
//
// Only returns an error if forceSync is true.
func (c *InstanceCache) withState(ctx context.Context, now time.Time, forceSync bool, f func(*messages.InstanceCache) (save bool)) error {
	if c.Tmp {
		return nil // no need to keep state.db for temp caches
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

	if err := c.readState(ctx, state, now, forceSync); err != nil {
		return err
	}
	if f(state) {
		if err := c.saveState(ctx, state); err != nil {
			logging.Warningf(ctx, "Could not save instance cache: %s", err)
		}
	}
	return nil
}
