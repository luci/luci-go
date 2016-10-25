// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"io/ioutil"
	"os"
	"sort"
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/internal/messages"
	"github.com/luci/luci-go/cipd/client/cipd/local"
	"github.com/luci/luci-go/common/logging"
)

const (
	tagCacheMaxSize  = 300
	tagCacheFilename = "tagcache.db"
)

// TagCache provides a mapping (package name, tag) -> instance ID.
//
// This mapping is safe to cache because tags are not detachable: once a tag is
// successfully resolved to an instance ID it is guaranteed to resolve to same
// instance ID later or not resolve at all (e.g. if one tag is attached to
// multiple instances, in which case the tag is misused anyway). In any case,
// returning a cached instance ID does make sense. The primary purpose of this
// cache is to avoid round trips to the service to increase reliability of
// 'cipd ensure' calls that use only tags to specify versions. It happens to be
// the most common case of 'cipd ensure' usage by far.
type TagCache struct {
	fs   local.FileSystem
	lock sync.Mutex

	cache *messages.TagCache                  // the last loaded state, if not nil.
	added map[string]*messages.TagCache_Entry // entries added by AddTag
}

// tagMapKey constructs a key for TagCache.added map.
//
// We use string math instead of a struct to avoid reimplementing sorting
// interface.
func tagMapKey(pkgName, tag string) string {
	return pkgName + ":" + tag
}

// NewTagCache initializes TagCache.
//
// fs will be the root of the cache. It will be searched for tagcache.db file.
func NewTagCache(fs local.FileSystem) *TagCache {
	return &TagCache{fs: fs}
}

// ResolveTag returns cached tag or empty Pin{} if such tag is not in the cache.
//
// Returns error if the cache can't be read.
func (c *TagCache) ResolveTag(ctx context.Context, pkg, tag string) (common.Pin, error) {
	if err := common.ValidatePackageName(pkg); err != nil {
		return common.Pin{}, err
	}
	if err := common.ValidateInstanceTag(tag); err != nil {
		return common.Pin{}, err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	// Already added with AddTag recently?
	if e := c.added[tagMapKey(pkg, tag)]; e != nil {
		return common.Pin{
			PackageName: e.Package,
			InstanceID:  e.InstanceId,
		}, nil
	}

	// Lazy-load the cache the first time it is used. We reload it again in Save
	// right before overwriting the file. We don't reload it anywhere else though,
	// so the implementation essentially assumes Save is called relatively soon
	// after ResolveTag call. If it's not the case, cache updates made by other
	// processes will be "invisible" to the TagCache. It still tries not to
	// overwrite them in Save, so it's fine.
	if c.cache == nil {
		var err error
		if c.cache, err = c.loadFromDisk(ctx); err != nil {
			return common.Pin{}, err
		}
	}

	// Most recently used tags are usually at the end, search in reverse as a
	// silly optimization.
	for i := len(c.cache.Entries) - 1; i >= 0; i-- {
		e := c.cache.Entries[i]
		if e.Package == pkg && e.Tag == tag {
			return common.Pin{
				PackageName: pkg,
				InstanceID:  e.InstanceId,
			}, nil
		}
	}

	return common.Pin{}, nil
}

// AddTag records that (pin.PackageName, tag) maps to pin.InstanceID.
//
// Call 'Save' later to persist these changes to the cache file on disk.
func (c *TagCache) AddTag(ctx context.Context, pin common.Pin, tag string) error {
	if err := common.ValidatePackageName(pin.PackageName); err != nil {
		return err
	}
	if err := common.ValidateInstanceID(pin.InstanceID); err != nil {
		return err
	}
	if err := common.ValidateInstanceTag(tag); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.added == nil {
		c.added = make(map[string]*messages.TagCache_Entry, 1)
	}

	// 'Save' will merge this into 'c.cache' before dumping to disk.
	c.added[tagMapKey(pin.PackageName, tag)] = &messages.TagCache_Entry{
		Package:    pin.PackageName,
		Tag:        tag,
		InstanceId: pin.InstanceID,
	}

	return nil
}

// Save stores all pending cache updates to the file system.
//
// It effectively resets the object to the initial state.
func (c *TagCache) Save(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Nothing to store? Just clean the state, so that ResolveTag can fetch the
	// up-to-date cache from disk later if needed.
	if len(c.added) == 0 {
		c.cache = nil
		return nil
	}

	// Sort all new entries, for consistency.
	sorted := make([]string, 0, len(c.added))
	for k := range c.added {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	// Load the most recent data to avoid overwriting it.
	recent, err := c.loadFromDisk(ctx)
	if err != nil {
		return err
	}

	// Copy all existing entries, except the ones we are moving to the tail.
	merged := make([]*messages.TagCache_Entry, 0, len(recent.Entries)+len(c.added))
	for _, e := range recent.Entries {
		if c.added[tagMapKey(e.Package, e.Tag)] == nil {
			merged = append(merged, e)
		}
	}

	// Add new entries to the tail.
	for _, k := range sorted {
		merged = append(merged, c.added[k])
	}

	// Trim the end result, discard the head: it's where old items are.
	if len(merged) > tagCacheMaxSize {
		merged = merged[len(merged)-tagCacheMaxSize:]
	}

	// Serialize and write to disk. We still can accidentally replace someone
	// else's changes, but the probability should be relatively low. It can happen
	// only if two processes call 'Save' at the exact same time.
	updated := &messages.TagCache{Entries: merged}
	if err := c.dumpToDisk(ctx, updated); err != nil {
		return err
	}

	// The state is persisted now.
	c.cache = updated
	c.added = nil

	return nil
}

// loadFromDisk loads and parses the tag cache file.
//
// Returns empty struct if the file is missing or corrupted. Returns errors if
// the file can't be read.
func (c *TagCache) loadFromDisk(ctx context.Context) (*messages.TagCache, error) {
	path, err := c.fs.RootRelToAbs(tagCacheFilename)
	if err != nil {
		return nil, err
	}

	blob, err := ioutil.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return &messages.TagCache{}, nil
	case err != nil:
		return nil, err
	}

	// Just ignore the corrupted cache file.
	cache := &messages.TagCache{}
	if err := UnmarshalWithSHA1(blob, cache); err != nil {
		logging.Warningf(ctx, "cipd: can't deserialize tag cache - %s", err)
		return &messages.TagCache{}, nil
	}

	return cache, nil
}

// dumpToDisk serializes the tag cache and writes it to the file system.
func (c *TagCache) dumpToDisk(ctx context.Context, msg *messages.TagCache) error {
	path, err := c.fs.RootRelToAbs(tagCacheFilename)
	if err != nil {
		return err
	}

	blob, err := MarshalWithSHA1(msg)
	if err != nil {
		return err
	}

	return local.EnsureFile(ctx, c.fs, path, bytes.NewReader(blob))
}
