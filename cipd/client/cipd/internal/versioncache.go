// Copyright 2016 The LUCI Authors.
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
	"context"
	"sync"

	"go.chromium.org/luci/common/clock"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/common"
)

const (
	maxCachedTags          = 300
	maxCachedExecutables   = 20
	maxCachedRefs          = 300
	legacyVersionCacheName = "tagcache.db"
	versionCacheName       = "versioncache.db"
)

// VersionCache caches several types of version information:
//   - 'immutable' tags in the form (service, package, tag) -> instance ID.
//   - mutable refs in the form (service, package, ref) -> (instance ID,
//     capture time)
//   - hashes of files-in-instaance in the form (service, package, instance ID)
//     -> file name -> file hash.
//
// Instance tags should not be detached, so the client keeps them indefinitely.
// Service admins can detach them, but they are aware of the fact that the
// clients will keep them indefinitely and should only detach them if no
// clients have yet used this tag.
//
// The primary purpose of this cache is to avoid round trips to the service to
// increase reliability of 'cipd ensure' calls that use only tags to specify
// versions. It happens to be the most common case of 'cipd ensure' usage by
// far.
//
// Refs are inherently mutable. However, for *offline* caches it makes sense to
// serve them when generating the offline cache from an ensurefile using refs.
// We retain the capture timestamp for debugging, but also to potentially allow
// setting some short retention period for ref caching in the future.
//
// The VersionCache's mapping for file hashes is primarially to assist in the
// `selfupdate` flow where pre-cipd scripts need to check the hash of the
// actual cipd binary hash, not the instance id which the binary was extracted
// from. Whenever selfupdate resolves what CIPD package instance ID (pin) it
// should be at, it looks at '(pin, 'cipd') => binary hash' map to figure out
// what hash the binary itself should have for this instance ID. The client
// then calculates the hash of itself to see if it's actually already at that
// instance ID, and can make a decision to fetch a new client.
type VersionCache struct {
	// FS is a filesystem rooted on the cache directory.
	FS fs.FileSystem

	// UseLegacyName indicates that this cache should use the legacy cache name
	// when saving the version cache.
	SaveName LegacyVersionCache

	// LoadRefs indicates that we should load refs when reading the version cache
	// from disk.
	LoadRefs bool

	// SaveRefs indicates that refs added via AddRef should be persisted to disk
	// when saving.
	SaveRefs bool

	mu sync.Mutex

	// The last-loaded state.
	cache       memoryVersionCache
	cacheLoaded bool

	// These are added values which have not been flushed to disk yet.
	added memoryVersionCache
}

// lazyLoadLocked loads the cache from disk.
//
// By the time this returns nil, `c.cache` is guaranteed to be non-nil.
func (c *VersionCache) lazyLoadLocked(ctx context.Context) error {
	if c.cacheLoaded {
		return nil
	}

	rawCache, err := ReadVersionCache(ctx, c.FS)
	if err != nil {
		return err
	}

	c.cache.load(ctx, rawCache, c.LoadRefs)
	c.cacheLoaded = true
	return nil
}

// ResolveTag returns cached tag or empty Pin{} if such tag is not in the cache.
//
// Returns error if the cache can't be read.
func (c *VersionCache) ResolveTag(ctx context.Context, service, pkg, tag string) (pin common.Pin, err error) {
	if err = common.ValidatePackageName(pkg); err != nil {
		return pin, err
	}
	if err = common.ValidateInstanceTag(tag); err != nil {
		return pin, err
	}

	key := tagKey{service, pkg, tag}
	entry, err := findEntry(ctx, c, key, (*memoryVersionCache).getTag)
	if entry != nil && err == nil {
		pin.PackageName = entry.Package
		pin.InstanceID = entry.InstanceId
	}
	return pin, err
}

// ResolveExtractedObjectRef returns ObjectRef or nil if that file is not in the
// cache.
//
// Returns error if the cache can't be read.
func (c *VersionCache) ResolveExtractedObjectRef(ctx context.Context, service string, pin common.Pin, fileName string) (ref *caspb.ObjectRef, err error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return ref, err
	}

	key := fileKey{service, pin.PackageName, pin.InstanceID, fileName}
	entry, err := findEntry(ctx, c, key, (*memoryVersionCache).getFile)
	if entry != nil && err == nil {
		ref = common.InstanceIDToObjectRef(entry.ObjectRef)
	}
	return ref, err
}

// ResolveRef returns cached ref or empty Pin{} if such ref is not in the cache.
//
// Returns error if the cache can't be read.
func (c *VersionCache) ResolveRef(ctx context.Context, service, pkg, ref string) (pin common.Pin, err error) {
	if err = common.ValidatePackageName(pkg); err != nil {
		return pin, err
	}
	if err = common.ValidatePackageRef(ref); err != nil {
		return pin, err
	}

	key := refKey{service, pkg, ref}
	entry, err := findEntry(ctx, c, key, (*memoryVersionCache).getRef)
	if entry != nil && err == nil {
		pin.PackageName = entry.Package
		pin.InstanceID = entry.InstanceId
	}
	return pin, err
}

// AddTag records that (pin.PackageName, tag) maps to pin.InstanceID.
//
// Call [VersionCache.Flush] later to persist these changes to the cache file
// on disk.
func (c *VersionCache) AddTag(ctx context.Context, service string, pin common.Pin, tag string) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidateInstanceTag(tag); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.added.addTag(tagKey{service, pin.PackageName, tag}, pin.InstanceID)
	return nil
}

// AddExtractedObjectRef records that fileName extracted from the package at
// the given pin has the given hash.
//
// The hash is represented as ObjectRef, which is a tuple (hash algo, hex
// digest).
//
// Call [VersionCache.Flush] later to persist these changes to the cache file
// on disk.
func (c *VersionCache) AddExtractedObjectRef(ctx context.Context, service string, pin common.Pin, fileName string, ref *caspb.ObjectRef) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidateObjectRef(ref, common.AnyHash); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.added.addFile(fileKey{service, pin.PackageName, pin.InstanceID, fileName}, ref)
	return nil
}

// AddRef records that (pin.PackageName, ref) maps to pin.InstanceID.
//
// Records the capture time as [clock.Now].
//
// Call [VersionCache.Flush] later to persist these changes to the cache file
// on disk.
func (c *VersionCache) AddRef(ctx context.Context, service string, pin common.Pin, ref string) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidatePackageRef(ref); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.added.addRef(refKey{service, pin.PackageName, ref}, pin.InstanceID, clock.Now(ctx))
	return nil
}

// Flush flushes all pending cache updates to the file system.
//
// It effectively resets the object to the initial state (except that all saved
// versions are treated as if they were just loaded from the disk).
func (c *VersionCache) Flush(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Nothing to store? Just clean the state, so that ResolveTag can fetch the
	// up-to-date cache from disk later if needed.
	if c.added.count() == 0 {
		c.cacheLoaded = false
		return nil
	}

	// Load the most recent data to avoid overwriting it. Load ALL entries, even
	// if they belong to different service: we have one global cache file and
	// should preserve all entries there.
	recent, err := ReadVersionCache(ctx, c.FS)
	if err != nil {
		return err
	}

	// Serialize and write to disk. We still can accidentally replace someone
	// else's changes, but the probability should be relatively low. It can
	// happen only if two processes call [VersionCache.Flush] at the exact same
	// time.
	updated := &messages.VersionCache{
		Entries:     pruneEntries(recent.GetEntries(), c.added.tags, false, maxCachedTags),
		FileEntries: pruneEntries(recent.GetFileEntries(), c.added.files, false, maxCachedExecutables),
		RefEntries:  pruneEntries(recent.GetRefEntries(), c.added.refs, !c.SaveRefs, maxCachedRefs),
	}
	if err := WriteVersionCache(ctx, c.FS, updated, c.SaveName); err != nil {
		return err
	}

	// The state is persisted now; load the updated proto back into c.cache as if
	// we just read it from disk.
	c.cache.load(ctx, updated, c.LoadRefs)
	c.added.reset()

	return nil
}
