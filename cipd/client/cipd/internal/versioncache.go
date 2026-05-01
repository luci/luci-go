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
	"cmp"
	"context"
	"slices"
	"sync"

	"go.chromium.org/luci/common/logging"

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

	cache       *messages.VersionCache // the last loaded state, if not nil.
	cacheLoaded bool
	addedTags   map[tagKey]*messages.VersionCache_Entry      // entries added by AddTag
	addedFiles  map[fileKey]*messages.VersionCache_FileEntry // entries added by AddExtractedObjectRef
}

type tagKey struct {
	pkg string
	tag string
}

func (t tagKey) cmp(o tagKey) int {
	return cmp.Or(
		cmp.Compare(t.pkg, o.pkg),
		cmp.Compare(t.tag, o.tag),
	)
}

type fileKey struct {
	pkg      string
	instance string
	file     string
}

func (t fileKey) cmp(o fileKey) int {
	return cmp.Or(
		cmp.Compare(t.pkg, o.pkg),
		cmp.Compare(t.instance, o.instance),
		cmp.Compare(t.file, o.file),
	)
}

// NewVersionCache initializes VersionCache.
//
// fs will be the root of the cache. It will be searched for tagcache.db file.
func NewVersionCache(fs fs.FileSystem, service string, useName LegacyVersionCache) *VersionCache {
	return &VersionCache{
		fs:      fs,
		service: service,
		useName: useName,
	}
}

func (c *VersionCache) lazyLoadLocked(ctx context.Context) error {
	// Lazy-load the cache the first time it is used. We reload it again in Flush
	// right before overwriting the file. We don't reload it anywhere else
	// though, so the implementation essentially assumes Flush is called
	// relatively soon after ResolveTag call. If it's not the case, cache updates
	// made by other processes will be "invisible" to the VersionCache. It still
	// tries not to overwrite them in Flush, so it's fine.
	if c.cacheLoaded {
		return nil
	}
	rawCache, err := ReadVersionCache(ctx, c.fs)
	if err != nil {
		return err
	}
	if len(rawCache.GetEntries()) > 0 {
		rawCache.Entries = slices.DeleteFunc(rawCache.Entries, func(e *messages.VersionCache_Entry) bool {
			return e.Service != c.service
		})
	}
	if len(rawCache.GetFileEntries()) > 0 {
		rawCache.FileEntries = slices.DeleteFunc(rawCache.FileEntries, func(e *messages.VersionCache_FileEntry) bool {
			if e.Service != c.service {
				return true
			}

			if err := common.ValidateInstanceID(e.ObjectRef, common.AnyHash); err != nil {
				pin := common.Pin{
					PackageName: e.Package,
					InstanceID:  e.InstanceId,
				}
				logging.Errorf(ctx, "Stored object_ref %q for %q in %s is invalid, ignoring it: %s",
					e.ObjectRef, e.FileName, pin, err)
				return true
			}

			return false
		})
	}
	c.cache = rawCache
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

	c.lock.Lock()
	defer c.lock.Unlock()

	// Already added with AddTag recently?
	if e := c.addedTags[tagKey{pkg, tag}]; e != nil {
		return common.Pin{
			PackageName: e.Package,
			InstanceID:  e.InstanceId,
		}, nil
	}

	if err = c.lazyLoadLocked(ctx); err != nil {
		return pin, err
	}

	// Most recently used tags are usually at the end, search in reverse as a
	// silly optimization.
	for i := len(c.cache.GetEntries()) - 1; i >= 0; i-- {
		e := c.cache.Entries[i]
		if e.Package == pkg && e.Tag == tag {
			return common.Pin{
				PackageName: pkg,
				InstanceID:  e.InstanceId,
			}, nil
		}
	}
	return pin, err
}

// ResolveExtractedObjectRef returns ObjectRef or nil if that file is not in the
// cache.
//
// Returns error if the cache can't be read.
func (c *VersionCache) ResolveExtractedObjectRef(ctx context.Context, pin common.Pin, fileName string) (*caspb.ObjectRef, error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return nil, err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	key := fileKey{pin.PackageName, pin.InstanceID, fileName}
	if e := c.addedFiles[key]; e != nil {
		return common.InstanceIDToObjectRef(e.ObjectRef), nil
	}

	if err := c.lazyLoadLocked(ctx); err != nil {
		return nil, err
	}

	// Most recently used entries are usually at the end, search in reverse as a
	// silly optimization.
	for i := len(c.cache.GetFileEntries()) - 1; i >= 0; i-- {
		e := c.cache.FileEntries[i]
		if e.Package == pin.PackageName && e.InstanceId == pin.InstanceID && e.FileName == fileName {
			return common.InstanceIDToObjectRef(e.ObjectRef), nil
		}
	}
	return nil, nil
}

// AddTag records that (pin.PackageName, tag) maps to pin.InstanceID.
//
// Call 'Flush' later to persist these changes to the cache file on disk.
func (c *VersionCache) AddTag(ctx context.Context, pin common.Pin, tag string) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidateInstanceTag(tag); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.addedTags == nil {
		c.addedTags = make(map[tagKey]*messages.VersionCache_Entry, 1)
	}

	// 'Flush' will merge this into 'c.cache' before dumping to disk.
	c.addedTags[tagKey{pin.PackageName, tag}] = &messages.VersionCache_Entry{
		Service:    c.service,
		Package:    pin.PackageName,
		Tag:        tag,
		InstanceId: pin.InstanceID,
	}

	return nil
}

// AddExtractedObjectRef records that fileName extracted from the package at
// the given pin has the given hash.
//
// The hash is represented as ObjectRef, which is a tuple (hash algo, hex
// digest).
//
// Call 'Flush' later to persist these changes to the cache file on disk.
func (c *VersionCache) AddExtractedObjectRef(ctx context.Context, pin common.Pin, fileName string, ref *caspb.ObjectRef) error {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return err
	}
	if err := common.ValidateObjectRef(ref, common.AnyHash); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.addedFiles == nil {
		c.addedFiles = make(map[fileKey]*messages.VersionCache_FileEntry)
	}
	c.addedFiles[fileKey{pin.PackageName, pin.InstanceID, fileName}] = &messages.VersionCache_FileEntry{
		Service:    c.service,
		Package:    pin.PackageName,
		InstanceId: pin.InstanceID,
		FileName:   fileName,
		ObjectRef:  common.ObjectRefToInstanceID(ref),
	}
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
	if len(c.addedTags) == 0 && len(c.addedFiles) == 0 {
		c.cache = nil
		return nil
	}

	// Sort all new entries, for consistency.
	sortedTags := make([]tagKey, 0, len(c.addedTags))
	for key := range c.addedTags {
		sortedTags = append(sortedTags, key)
	}
	slices.SortFunc(sortedTags, tagKey.cmp)

	sortedFiles := make([]fileKey, 0, len(c.addedFiles))
	for key := range c.addedFiles {
		sortedFiles = append(sortedFiles, key)
	}
	slices.SortFunc(sortedFiles, fileKey.cmp)

	// Load the most recent data to avoid overwriting it. Load ALL entries, even
	// if they belong to different service: we have one global cache file and
	// should preserve all entries there.
	recent, err := ReadVersionCache(ctx, c.fs)
	if err != nil {
		return err
	}

	// Copy existing entries, except the ones we are moving to the tail. Carefully
	// copy entries belonging to other services too, we must not overwrite them.
	mergedTags := make([]*messages.VersionCache_Entry, 0, len(recent.GetEntries())+len(c.addedTags))
	for _, e := range recent.GetEntries() {
		key := tagKey{e.Package, e.Tag}
		if e.Service != c.service || c.addedTags[key] == nil {
			mergedTags = append(mergedTags, e)
		}
	}

	// Add new entries to the tail.
	for _, k := range sortedTags {
		mergedTags = append(mergedTags, c.addedTags[tagKey(k)])
	}

	// Trim the end result, discard the head: it's where old items are.
	if len(mergedTags) > maxCachedTags {
		mergedTags = mergedTags[len(mergedTags)-maxCachedTags:]
	}

	// Do the same for file entries.
	mergedFiles := make([]*messages.VersionCache_FileEntry, 0, len(recent.GetFileEntries())+len(c.addedFiles))
	for _, e := range recent.GetFileEntries() {
		key := fileKey{e.Package, e.InstanceId, e.FileName}
		if e.Service != c.service || c.addedFiles[key] == nil {
			mergedFiles = append(mergedFiles, e)
		}
	}
	for _, k := range sortedFiles {
		mergedFiles = append(mergedFiles, c.addedFiles[fileKey(k)])
	}
	if len(mergedFiles) > maxCachedExecutables {
		mergedFiles = mergedFiles[len(mergedFiles)-maxCachedExecutables:]
	}

	// Serialize and write to disk. We still can accidentally replace someone
	// else's changes, but the probability should be relatively low. It can happen
	// only if two processes call 'Flush' at the exact same time.
	updated := &messages.VersionCache{Entries: mergedTags, FileEntries: mergedFiles}
	if err := WriteVersionCache(ctx, c.fs, updated, c.useName); err != nil {
		return err
	}

	// The state is persisted now.
	c.cache = updated
	c.addedTags = nil
	c.addedFiles = nil

	return nil
}
