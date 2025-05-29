// Copyright 2020 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/protoutil"
)

// Ensure TagIndexEntry implements datastore.PropertyConverter.
var _ datastore.PropertyConverter = &TagIndexEntry{}

// TagIndexEntry refers to a particular Build entity.
type TagIndexEntry struct {
	// BuildID is the ID of the Build entity this entry refers to.
	BuildID int64 `json:"build_id"`
	// <project>/<bucket>. Bucket is in v2 format.
	// e.g. chromium/try (never chromium/luci.chromium.try).
	BucketID string `json:"bucket_id"`
	// CreatedTime is the time this entry was created.
	CreatedTime time.Time `json:"created_time"`
}

// FromProperty deserializes TagIndexEntries from the datastore.
// Implements datastore.PropertyConverter.
func (e *TagIndexEntry) FromProperty(p datastore.Property) error {
	for key, val := range p.Value().(datastore.PropertyMap) {
		switch key {
		case "build_id":
			e.BuildID = val.Slice()[0].Value().(int64)
		case "bucket_id":
			e.BucketID = val.Slice()[0].Value().(string)
		case "created_time":
			e.CreatedTime = val.Slice()[0].Value().(time.Time)
		}
	}
	return nil
}

// ToProperty serializes TagIndexEntries to datastore format.
// Implements datastore.PropertyConverter.
func (e *TagIndexEntry) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	err := p.SetValue(datastore.PropertyMap{
		"build_id":     datastore.MkProperty(e.BuildID),
		"bucket_id":    datastore.MkProperty(e.BucketID),
		"created_time": datastore.MkProperty(e.CreatedTime),
	}, datastore.NoIndex)
	return p, err
}

// MaxTagIndexEntries is the maximum number of entries that may be associated
// with a single TagIndex entity.
const MaxTagIndexEntries = 1000

// TagIndexShardCount is the number of shards used by the TagIndex.
const TagIndexShardCount = 16

// TagIndex is an index used to search Build entities by tag.
type TagIndex struct {
	_kind string `gae:"$kind,TagIndex"`
	// ID is a "<key>:<value>" or ":<index>:<key>:<value>" string for index > 0.
	ID string `gae:"$id"`
	// Incomplete means there are more than MaxTagIndexEntries entities
	// with the same ID, and therefore the index is incomplete and cannot be
	// searched.
	Incomplete bool `gae:"permanently_incomplete,noindex"`
	// Entries is a slice of TagIndexEntries matching this ID.
	Entries []TagIndexEntry `gae:"entries,noindex"`
}

// TagIndexIncomplete means the tag index is incomplete and thus cannot be searched.
var TagIndexIncomplete = errtag.Make("tag index incomplete", true)

// SearchTagIndex searches the tag index for the given tag.
// Returns an error tagged with TagIndexIncomplete if the tag index is
// incomplete and thus cannot be searched.
func SearchTagIndex(ctx context.Context, key, val string) ([]*TagIndexEntry, error) {
	shds := make([]TagIndex, TagIndexShardCount)
	for i := range shds {
		if i == 0 {
			shds[i].ID = fmt.Sprintf("%s:%s", key, val)
		} else {
			shds[i].ID = fmt.Sprintf(":%d:%s:%s", i, key, val)
		}
	}
	if err := GetIgnoreMissing(ctx, shds); err != nil {
		return nil, errors.Fmt("error fetching tag index for %q: %w", fmt.Sprintf("%s:%s", key, val), err)
	}
	var ents []*TagIndexEntry
	for _, s := range shds {
		if s.Incomplete {
			return nil, TagIndexIncomplete.Apply(errors.Fmt("tag index incomplete for %q", fmt.Sprintf("%s:%s", key, val)))
		}
		for i := range s.Entries {
			// check in case the tagIndexEntry is corrupted.
			if _, _, err := protoutil.ParseBucketID(s.Entries[i].BucketID); err != nil {
				logging.Warningf(ctx, "Bad TagIndexEntry(%+v): %s in TagIndex %s", s.Entries[i], err, s.ID)
				continue
			}
			ents = append(ents, &s.Entries[i])
		}
	}
	return ents, nil
}

// UpdateTagIndex updates the tag index for the given tag.
func UpdateTagIndex(ctx context.Context, tag string, ents []TagIndexEntry) error {
	if len(ents) == 0 {
		return nil
	}
	return updateTagIndex(ctx, tag, mathrand.Intn(ctx, TagIndexShardCount), ents)
}

// updateTagIndex updates the tag index's specified shard for the given tag.
func updateTagIndex(ctx context.Context, tag string, shard int, ents []TagIndexEntry) error {
	if len(ents) == 0 {
		return nil
	}
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		shd := &TagIndex{
			ID: tag,
		}
		if shard > 0 {
			shd.ID = fmt.Sprintf(":%d:%s", shard, tag)
		}
		switch err := datastore.Get(ctx, shd); {
		case err == datastore.ErrNoSuchEntity:
		case err != nil:
			return errors.Fmt("error fetching tag index for %q: %w", shd.ID, err)
		case shd.Incomplete:
			// No point in updating an incomplete index because it cannot be searched.
			return nil
		}

		orig := len(shd.Entries)
		shd.Entries = append(shd.Entries, ents...)
		if len(shd.Entries) > MaxTagIndexEntries {
			shd.Entries = nil
			shd.Incomplete = true
			logging.Warningf(ctx, "marking tag index incomplete for %q", shd.ID)
		} else {
			logging.Debugf(ctx, "updating tag index for %q (entries %d -> %d)", shd.ID, orig, len(ents))
		}

		if err := datastore.Put(ctx, shd); err != nil {
			return errors.Fmt("error updating tag index for %q: %w", shd.ID, err)
		}
		return nil
	}, nil)
}
