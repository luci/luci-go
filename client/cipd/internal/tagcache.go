// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"io/ioutil"
	"os"
	"sync"

	"github.com/luci/luci-go/client/cipd/common"
	"github.com/luci/luci-go/client/cipd/internal/messages"
)

// MaxTagCacheSize is how many entries to keep in TagCache database.
const MaxTagCacheSize = 300

// TagCache provides a thread safe mapping (package name, tag) -> instance ID.
// This mapping is safe to cache because tags are not detachable: once a tag is
// successfully resolved to an instance ID it is guaranteed to resolve to same
// instance ID later or not resolve at all (e.g. if one tag is attached to
// multiple instances, in which case the tag is misused anyway). In any case,
// returning  a cached instance ID does make sense. The primary purpose of this
// cache is to avoid round trips to the service to increase reliability of
// 'cipd ensure' calls that use only tags to specify versions. It happens to be
// the most common case of 'cipd ensure' usage by far.
type TagCache struct {
	lock  sync.Mutex
	cache messages.TagCache
	dirty bool
}

// LoadTagCacheFromFile reads tag cache state from given file path if it exists.
// Returns empty cache if file doesn't exist.
func LoadTagCacheFromFile(path string) (*TagCache, error) {
	buf, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return &TagCache{}, nil
	}
	if err != nil {
		return nil, err
	}
	cache := &TagCache{}
	if err := cache.Load(buf); err != nil {
		return nil, err
	}
	return cache, nil
}

// Load loads the state from given buffer.
func (c *TagCache) Load(buf []byte) error {
	cache := messages.TagCache{}
	if err := UnmarshalWithSHA1(buf, &cache); err != nil {
		return err
	}

	// Validate entries. Make sure to keep only MaxTagCacheSize number of them.
	goodOnes := make([]*messages.TagCache_Entry, 0, MaxTagCacheSize)
	for i := 0; i < len(cache.Entries) && len(goodOnes) < MaxTagCacheSize; i++ {
		e := cache.Entries[i]
		valid := e != nil &&
			common.ValidatePackageName(e.Package) == nil &&
			common.ValidateInstanceTag(e.Tag) == nil &&
			common.ValidateInstanceID(e.InstanceId) == nil
		if valid {
			goodOnes = append(goodOnes, e)
		}
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache.Entries = goodOnes
	c.dirty = false
	return nil
}

// Save dumps state to the byte buffer. Also resets 'Dirty' flag.
func (c *TagCache) Save() ([]byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Remove all "holes" left from moving entries in AddTag.
	compacted := make([]*messages.TagCache_Entry, 0, len(c.cache.Entries))
	for _, e := range c.cache.Entries {
		if e != nil {
			compacted = append(compacted, e)
		}
	}

	// Keep at most MaxTagCacheSize entries. Truncate head of the slice, since
	// it's  where old items are. All new hotness is at the tail, we need
	// to keep it.
	if len(compacted) > MaxTagCacheSize {
		compacted = compacted[len(compacted)-MaxTagCacheSize:]
	}
	c.cache.Entries = compacted

	out, err := MarshalWithSHA1(&c.cache)
	if err == nil {
		c.dirty = false
	}
	return out, err
}

// Dirty returns true if Save() needs to be called to persist changes.
func (c *TagCache) Dirty() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.dirty
}

// ResolveTag returns cached tag or empty Pin{} if such tag is not in cache.
func (c *TagCache) ResolveTag(pkg, tag string) common.Pin {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i := len(c.cache.Entries) - 1; i >= 0; i-- {
		e := c.cache.Entries[i]
		if e != nil && e.Package == pkg && e.Tag == tag {
			return common.Pin{
				PackageName: pkg,
				InstanceID:  e.InstanceId,
			}
		}
	}
	return common.Pin{}
}

// AddTag records that (pin.PackageName, tag) maps to pin.InstanceID.
func (c *TagCache) AddTag(pin common.Pin, tag string) {
	// Just skip invalid data. It should not be here anyway.
	bad := common.ValidatePackageName(pin.PackageName) != nil ||
		common.ValidateInstanceID(pin.InstanceID) != nil ||
		common.ValidateInstanceTag(tag) != nil
	if bad {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	// Try to find an existing entry in the cache. It will be moved to bottom
	// (thus promoted as "most recent one"). We put a "hole" (nil) in previous
	// position to avoid shifting array for no good reason. All "holes" are
	// compacted in Save().
	var existing *messages.TagCache_Entry
	for i, e := range c.cache.Entries {
		if e != nil && e.Package == pin.PackageName && e.Tag == tag {
			existing = e
			c.cache.Entries[i] = nil
			break
		}
	}
	if existing == nil {
		existing = &messages.TagCache_Entry{
			Package: pin.PackageName,
			Tag:     tag,
		}
	}
	existing.InstanceId = pin.InstanceID

	c.dirty = true
	c.cache.Entries = append(c.cache.Entries, existing)
}
