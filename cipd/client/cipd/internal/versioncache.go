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

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/common"
)

const (
	maxCachedTags          = 300
	maxCachedExecutables   = 20
	legacyVersionCacheName = "tagcache.db"
	versionCacheName       = "versioncache.db"
)

// VersionCache provides a mapping (package name, tag) -> instance ID.
//
// This mapping is safe to cache because tags are not detachable: once a tag is
// successfully resolved to an instance ID it is guaranteed to resolve to same
// instance ID later or not resolve at all (e.g. if one tag is attached to
// multiple instances, in which case the tag is misused anyway). In any case,
// returning a cached instance ID does make sense. The primary purpose of this
// cache is to avoid round trips to the service to increase reliability of
// 'cipd ensure' calls that use only tags to specify versions. It happens to be
// the most common case of 'cipd ensure' usage by far.
//
// Additionally, this VersionCache stores a mapping of (pin, file_name) ->
// encode(ObjectRef of extracted file) to assist in the `selfupdate` flow.
//
// Whenever selfupdate resolves what CIPD package instance ID (pin) it SHOULD be
// at, it looks at '(pin, 'cipd') => binary hash' map to figure out what hash
// the client itself SHOULD have for this instance ID. The client then
// calculates the hash of itself to see if it's actually already at that
// instance ID.
type VersionCache struct {
	fs      fs.FileSystem
	service string
	useName LegacyVersionCache

	lock sync.Mutex

	// The last-loaded state.
	cache       memoryVersionCache
	cacheLoaded bool

	// These are added values which have not been flushed to disk yet.
	added memoryVersionCache
}

// NewVersionCache initializes VersionCache.
//
// fs will be the root of the cache. It will be searched for tagcache.db file.
func NewVersionCache(fs fs.FileSystem, service string, useName LegacyVersionCache) *VersionCache {
	return &VersionCache{
		fs:      fs,
		service: service,
		useName: useName,
		added:   memoryVersionCache{service: service},
	}
}

// lazyLoadLocked loads the cache from disk.
//
// By the time this returns nil, `c.cache` is guaranteed to be non-nil.
func (c *VersionCache) lazyLoadLocked(ctx context.Context) error {
	if c.cacheLoaded {
		return nil
	}

	rawCache, err := ReadVersionCache(ctx, c.fs)
	if err != nil {
		return err
	}

	c.cache.service = c.service
	c.cache.load(ctx, rawCache)
	c.cacheLoaded = true
	return nil
}

// ResolveTag returns cached tag or empty Pin{} if such tag is not in the cache.
//
// Returns error if the cache can't be read.
func (c *VersionCache) ResolveTag(ctx context.Context, pkg, tag string) (pin common.Pin, err error) {
	if err = common.ValidatePackageName(pkg); err != nil {
		return pin, err
	}
	if err = common.ValidateInstanceTag(tag); err != nil {
		return pin, err
	}

	key := tagKey{pkg, tag}
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
func (c *VersionCache) ResolveExtractedObjectRef(ctx context.Context, pin common.Pin, fileName string) (ref *caspb.ObjectRef, err error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return ref, err
	}

	key := fileKey{pin.PackageName, pin.InstanceID, fileName}
	entry, err := findEntry(ctx, c, key, (*memoryVersionCache).getFile)
	if entry != nil && err == nil {
		ref = common.InstanceIDToObjectRef(entry.ObjectRef)
	}
	return ref, err
}

// AddTag records that (pin.PackageName, tag) maps to pin.InstanceID.
//
// Call [VersionCache.Flush] later to persist these changes to the cache file
// on disk.
func (c *VersionCache) AddTag(ctx context.Context, pin common.Pin, tag string) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidateInstanceTag(tag); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.added.addTag(tagKey{pin.PackageName, tag}, pin.InstanceID)
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
func (c *VersionCache) AddExtractedObjectRef(ctx context.Context, pin common.Pin, fileName string, ref *caspb.ObjectRef) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidateObjectRef(ref, common.AnyHash); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.added.addFile(fileKey{pin.PackageName, pin.InstanceID, fileName}, ref)
	return nil
}

// Flush flushes all pending cache updates to the file system.
//
// It effectively resets the object to the initial state (except that all saved
// versions are treated as if they were just loaded from the disk).
func (c *VersionCache) Flush(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Nothing to store? Just clean the state, so that ResolveTag can fetch the
	// up-to-date cache from disk later if needed.
	if c.added.count() == 0 {
		c.cacheLoaded = false
		return nil
	}

	// Load the most recent data to avoid overwriting it. Load ALL entries, even
	// if they belong to different service: we have one global cache file and
	// should preserve all entries there.
	recent, err := ReadVersionCache(ctx, c.fs)
	if err != nil {
		return err
	}

	// Serialize and write to disk. We still can accidentally replace someone
	// else's changes, but the probability should be relatively low. It can
	// happen only if two processes call [VersionCache.Flush] at the exact same
	// time.
	updated := &messages.VersionCache{
		Entries:     pruneEntries(recent.GetEntries(), c.added.tags, c.service, maxCachedTags),
		FileEntries: pruneEntries(recent.GetFileEntries(), c.added.files, c.service, maxCachedExecutables),
	}
	if err := WriteVersionCache(ctx, c.fs, updated, c.useName); err != nil {
		return err
	}

	// The state is persisted now; load the updated proto back into c.cache as if
	// we just read it form disk.
	c.cache.load(ctx, updated)
	c.added.reset()

	return nil
}
