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
	tagCacheMaxSize    = 300
	tagCacheMaxExeSize = 20
	tagCacheFilename   = "tagcache.db"
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
//
// Additionally, this TagCache stores a mapping of (pin, file_name) ->
// hex(sha1(executable)) to assist in the `selfupdate` flow. Whenever selfupdate
// resolves what instance ID it SHOULD be at, it checks the SHA1 of its own
// binary to see if it's actually already at that instance ID (since instance ID
// is of the whole package, and not of the actual executable inside the
// package).
type TagCache struct {
	fs      local.FileSystem
	service string

	lock sync.Mutex

	cache      *messages.TagCache                       // the last loaded state, if not nil.
	addedTags  map[tagKey]*messages.TagCache_Entry      // entries added by AddTag
	addedFiles map[fileKey]*messages.TagCache_FileEntry // entries added by AddFile
}

type tagKey string
type fileKey string

// makeTagKey constructs key for the TagCache.addedTags map.
//
// We use string math instead of a struct to avoid reimplementing sorting
// interface, ain't nobody got time for that.
func makeTagKey(pkg, tag string) tagKey {
	return tagKey(pkg + ":" + tag)
}

// makeFileKey constructs key for the TagCache.addedFiles map.
//
// It is distinct from tagKey structurally, thus uses different type.
func makeFileKey(pkg, instance, file string) fileKey {
	return fileKey(pkg + ":" + instance + ":" + file)
}

// NewTagCache initializes TagCache.
//
// fs will be the root of the cache. It will be searched for tagcache.db file.
func NewTagCache(fs local.FileSystem, service string) *TagCache {
	return &TagCache{fs: fs, service: service}
}

func (c *TagCache) lazyLoadLocked(ctx context.Context) (err error) {
	// Lazy-load the cache the first time it is used. We reload it again in Save
	// right before overwriting the file. We don't reload it anywhere else though,
	// so the implementation essentially assumes Save is called relatively soon
	// after ResolveTag call. If it's not the case, cache updates made by other
	// processes will be "invisible" to the TagCache. It still tries not to
	// overwrite them in Save, so it's fine.
	if c.cache == nil {
		c.cache, err = c.loadFromDisk(ctx, false)
	}
	return
}

// ResolveTag returns cached tag or empty Pin{} if such tag is not in the cache.
//
// Returns error if the cache can't be read.
func (c *TagCache) ResolveTag(ctx context.Context, pkg, tag string) (pin common.Pin, err error) {
	if err = common.ValidatePackageName(pkg); err != nil {
		return
	}
	if err = common.ValidateInstanceTag(tag); err != nil {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	// Already added with AddTag recently?
	if e := c.addedTags[makeTagKey(pkg, tag)]; e != nil {
		return common.Pin{
			PackageName: e.Package,
			InstanceID:  e.InstanceId,
		}, nil
	}

	if err = c.lazyLoadLocked(ctx); err != nil {
		return
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

	return
}

// ResolveFile returns file hash or "" if that file is not in the cache.
//
// Returns error if the cache can't be read.
func (c *TagCache) ResolveFile(ctx context.Context, pin common.Pin, fileName string) (hash string, err error) {
	if err = common.ValidatePin(pin); err != nil {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	key := makeFileKey(pin.PackageName, pin.InstanceID, fileName)
	if e := c.addedFiles[key]; e != nil {
		return e.Hash, nil
	}

	if err = c.lazyLoadLocked(ctx); err != nil {
		return
	}

	// Most recently used tags are usually at the end, search in reverse as a
	// silly optimization.
	for i := len(c.cache.FileEntries) - 1; i >= 0; i-- {
		e := c.cache.FileEntries[i]
		if e.Package == pin.PackageName && e.InstanceId == pin.InstanceID && e.FileName == fileName {
			hash = e.Hash
			return
		}
	}
	return
}

// AddTag records that (pin.PackageName, tag) maps to pin.InstanceID.
//
// Call 'Save' later to persist these changes to the cache file on disk.
func (c *TagCache) AddTag(ctx context.Context, pin common.Pin, tag string) error {
	if err := common.ValidatePin(pin); err != nil {
		return err
	}
	if err := common.ValidateInstanceTag(tag); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.addedTags == nil {
		c.addedTags = make(map[tagKey]*messages.TagCache_Entry, 1)
	}

	// 'Save' will merge this into 'c.cache' before dumping to disk.
	c.addedTags[makeTagKey(pin.PackageName, tag)] = &messages.TagCache_Entry{
		Service:    c.service,
		Package:    pin.PackageName,
		Tag:        tag,
		InstanceId: pin.InstanceID,
	}

	return nil
}

// AddFile records that (pin, fileName) maps to hash (where hash is the
// hex-encoded sha1 of the named file).
//
// Call 'Save' later to persist these changes to the cache file on disk.
func (c *TagCache) AddFile(ctx context.Context, pin common.Pin, fileName, hash string) error {
	if err := common.ValidatePin(pin); err != nil {
		return err
	}
	if err := common.ValidateFileHash(hash); err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.addedFiles == nil {
		c.addedFiles = make(map[fileKey]*messages.TagCache_FileEntry, 1)
	}
	c.addedFiles[makeFileKey(pin.PackageName, pin.InstanceID, fileName)] = &messages.TagCache_FileEntry{
		Service:    c.service,
		Package:    pin.PackageName,
		InstanceId: pin.InstanceID,
		FileName:   fileName,
		Hash:       hash,
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
	if len(c.addedTags) == 0 && len(c.addedFiles) == 0 {
		c.cache = nil
		return nil
	}

	// Sort all new entries, for consistency.
	sortedTags := make([]string, 0, len(c.addedTags))
	for k := range c.addedTags {
		sortedTags = append(sortedTags, string(k))
	}
	sort.Strings(sortedTags)
	sortedFiles := make([]string, 0, len(c.addedFiles))
	for k := range c.addedFiles {
		sortedFiles = append(sortedFiles, string(k))
	}
	sort.Strings(sortedFiles)

	// Load the most recent data to avoid overwriting it. Load ALL entries, even
	// if they belong to different service: we have one global cache file and
	// should preserve all entries there.
	recent, err := c.loadFromDisk(ctx, true)
	if err != nil {
		return err
	}

	// Copy existing entries, except the ones we are moving to the tail. Carefully
	// copy entries belonging to other services too, we must not overwrite them.
	mergedTags := make([]*messages.TagCache_Entry, 0, len(recent.Entries)+len(c.addedTags))
	for _, e := range recent.Entries {
		key := makeTagKey(e.Package, e.Tag)
		if e.Service != c.service || c.addedTags[key] == nil {
			mergedTags = append(mergedTags, e)
		}
	}

	// Add new entries to the tail.
	for _, k := range sortedTags {
		mergedTags = append(mergedTags, c.addedTags[tagKey(k)])
	}

	// Trim the end result, discard the head: it's where old items are.
	if len(mergedTags) > tagCacheMaxSize {
		mergedTags = mergedTags[len(mergedTags)-tagCacheMaxSize:]
	}

	// Do the same for file entries.
	mergedFiles := make([]*messages.TagCache_FileEntry, 0, len(recent.FileEntries)+len(c.addedFiles))
	for _, e := range recent.FileEntries {
		key := makeFileKey(e.Package, e.InstanceId, e.FileName)
		if e.Service != c.service || c.addedFiles[key] == nil {
			mergedFiles = append(mergedFiles, e)
		}
	}
	for _, k := range sortedFiles {
		mergedFiles = append(mergedFiles, c.addedFiles[fileKey(k)])
	}
	if len(mergedFiles) > tagCacheMaxExeSize {
		mergedFiles = mergedFiles[len(mergedFiles)-tagCacheMaxExeSize:]
	}

	// Serialize and write to disk. We still can accidentally replace someone
	// else's changes, but the probability should be relatively low. It can happen
	// only if two processes call 'Save' at the exact same time.
	updated := &messages.TagCache{Entries: mergedTags, FileEntries: mergedFiles}
	if err := c.dumpToDisk(ctx, updated); err != nil {
		return err
	}

	// The state is persisted now.
	c.cache = updated
	c.addedTags = nil
	c.addedFiles = nil

	return nil
}

// loadFromDisk loads and parses the tag cache file.
//
// If 'allService' is true, returns all cache entries (regardless of what
// service they correspond to), otherwise filters out all entries that don't
// match c.service.
//
// Returns empty struct if the file is missing or corrupted. Returns errors if
// the file can't be read.
func (c *TagCache) loadFromDisk(ctx context.Context, allServices bool) (*messages.TagCache, error) {
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
	cache := messages.TagCache{}
	if err := UnmarshalWithSHA1(blob, &cache); err != nil {
		logging.Warningf(ctx, "cipd: can't deserialize tag cache - %s", err)
		return &messages.TagCache{}, nil
	}

	// Throw away all entries with empty Service. They are not longer valid.
	// Also apply 'allServices' filter.
	filtered := &messages.TagCache{}
	for _, e := range cache.Entries {
		if e.Service != "" && (e.Service == c.service || allServices) {
			filtered.Entries = append(filtered.Entries, e)
		}
	}
	for _, e := range cache.FileEntries {
		if e.Service != "" && (e.Service == c.service || allServices) {
			filtered.FileEntries = append(filtered.FileEntries, e)
		}
	}

	return filtered, nil
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
