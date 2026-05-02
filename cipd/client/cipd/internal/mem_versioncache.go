// Copyright 2026 The LUCI Authors.
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
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/common"
)

// memoryVersionCache is a version of the versioncache suitable for making fast
// lookups and fast insertions.
type memoryVersionCache struct {
	tags  tagMap
	files fileMap
	refs  refMap
}

// load resets the cache and populates it with data in `vc`.
//
// Safe to pass nil `vc`; it will just reset the memory cache.
//
// If `loadRefs` is true, this will also load refs (otherwise they are
// ignored).
func (m *memoryVersionCache) load(ctx context.Context, vc *messages.VersionCache, loadRefs bool) {
	m.reset()

	if entries := vc.GetEntries(); len(entries) > 0 {
		m.tags = make(tagMap, len(entries))
		for _, e := range entries {
			m.tags[mkTagKey(e)] = e
		}
	}

	if entries := vc.GetFileEntries(); len(entries) > 0 {
		m.files = make(fileMap, len(entries))
		for _, e := range entries {
			// We do this because we will call common.InstanceIDToObjectRef on
			// these values later, which will panic if the InstanceId is malformed.
			if err := common.ValidateInstanceID(e.ObjectRef, common.AnyHash); err != nil {
				logging.Errorf(ctx, "Stored object_ref %q for %q in %s is invalid, ignoring it: %s",
					e.ObjectRef, e.FileName, e.Package, err)
				continue
			}
			m.files[mkFileKey(e)] = e
		}
	}

	if entries := vc.GetRefEntries(); loadRefs && len(entries) > 0 {
		m.refs = make(refMap, len(entries))
		for _, e := range entries {
			m.refs[mkRefKey(e)] = e
		}
	}
}

// addTag inserts/overwrites any existing mapping of `tagKey` -> `iid`.
func (m *memoryVersionCache) addTag(key tagKey, iid string) {
	if m.tags == nil {
		m.tags = make(tagMap, 1)
	}
	m.tags[key] = &messages.VersionCache_Entry{
		Service:    key.service,
		Package:    key.pkg,
		Tag:        key.tag,
		InstanceId: iid,
	}
}

// getTag is a helper for [resolveFrom].
func (m *memoryVersionCache) getTag(key tagKey) *messages.VersionCache_Entry {
	return m.tags[key]
}

// addFile inserts/overwrites any existing mapping of `fileKey` ->
// InstanceID(`ref`).
func (m *memoryVersionCache) addFile(key fileKey, ref *caspb.ObjectRef) {
	if m.files == nil {
		m.files = make(fileMap, 1)
	}
	m.files[key] = &messages.VersionCache_FileEntry{
		Service:    key.service,
		Package:    key.pkg,
		InstanceId: key.instance,
		FileName:   key.file,
		ObjectRef:  common.ObjectRefToInstanceID(ref),
	}
}

// getFile is a helper for [resolveFrom].
func (m *memoryVersionCache) getFile(key fileKey) *messages.VersionCache_FileEntry {
	return m.files[key]
}

func (m *memoryVersionCache) addRef(key refKey, iid string, now time.Time) {
	if m.refs == nil {
		m.refs = make(refMap, 1)
	}
	m.refs[key] = &messages.VersionCache_RefEntry{
		Service:    key.service,
		Package:    key.pkg,
		Ref:        key.ref,
		InstanceId: iid,
		Captured:   timestamppb.New(now),
	}
}

// getRef is a helper for [resolveFrom].
func (m *memoryVersionCache) getRef(key refKey) *messages.VersionCache_RefEntry {
	return m.refs[key]
}

// count returns the total number of mappings trapped by this
// memoryVersionCache.
func (m *memoryVersionCache) count() int {
	return len(m.tags) + len(m.files) + len(m.refs)
}

// reset clears this memoryVersionCache, resetting all its maps to nil.
func (m *memoryVersionCache) reset() {
	m.tags = nil
	m.files = nil
	m.refs = nil
}

// entry is the archetype of messages.VersionCache_*Entry.
type entry interface {
	comparable
	proto.Message
	GetService() string
}

// findEntry finds an entry by key (e.g. tagKey/fileKey) in a VersionCache,
// possibly loading the VersionCache's on-disk representation if it's not
// already present in memory.
func findEntry[K comparable, E entry](
	ctx context.Context,
	c *VersionCache,
	key K,
	lookup func(*memoryVersionCache, K) E,
) (E, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var eZero E

	// Already added with AddTag recently?
	if e := lookup(&c.added, key); e != eZero {
		return e, nil
	}

	if err := c.lazyLoadLocked(ctx); err != nil {
		return eZero, err
	}

	return lookup(&c.cache, key), nil
}

// sortedEntryMap is the generic archetype of tagMap, fileMap and refMap.
type sortedEntryMap[K comparable, E entry] interface {
	~map[K]E
	has(E) bool
	sorted(yield func(K, E) bool)
}

// pruneEntries takes a slice of versioncache entries, adds/updates new
// entries, truncates this to `maxCount` and returns it.
//
// Includes existing entries, except the ones we are moving to the tail.
//
// Args:
//   - recent - recently-loaded entries.
//   - added - map of entries which we have added in this process and want to
//     merge.
//   - dropAdded - if true, ignore `added` and return `recent` directly.
//   - maxCount - the maximum number of entries to return after merging
//     `added`.
//
// If `dropAdded` is true, this ignores `added` completely (just returning
// `recent`).
func pruneEntries[E entry, K comparable, EM sortedEntryMap[K, E]](recent []E, added EM, dropAdded bool, maxCount int) []E {
	if len(added) == 0 || dropAdded {
		// respect maxCount here as well to keep the argument behaviors orthogonal.
		if len(recent) > maxCount {
			recent = recent[len(recent)-maxCount:]
		}
		return recent
	}

	ret := make([]E, 0, len(recent)+len(added))
	for _, e := range recent {
		if !added.has(e) {
			ret = append(ret, e)
		}
	}

	// Add new entries to the tail.
	for _, entry := range added.sorted {
		ret = append(ret, entry)
	}

	// Trim the end result, discard the head: it's where old items are.
	if len(ret) > maxCount {
		ret = ret[len(ret)-maxCount:]
	}

	return ret
}
