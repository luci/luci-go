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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
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
	CreatedTime time.Time `json:"create_time"`
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
	return p, p.SetValue(datastore.PropertyMap{
		"build_id":     datastore.MkProperty(e.BuildID),
		"bucket_id":    datastore.MkProperty(e.BucketID),
		"created_time": datastore.MkProperty(e.CreatedTime),
	}, datastore.NoIndex)
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

// ErrTagIndexIncomplete means the tag index is incomplete and thus cannot be searched.
var ErrTagIndexIncomplete = errors.Reason("tag index incomplete").Err()

// SearchTagIndex searches the tag index for the given tag.
// Returns ErrTagIndexIncomplete if the tag index is incomplete and thus cannot be searched.
func SearchTagIndex(ctx context.Context, key, val string) ([]TagIndexEntry, error) {
	shds := make([]TagIndex, TagIndexShardCount)
	for i := range shds {
		var id string
		if i == 0 {
			id = fmt.Sprintf("%s:%s", key, val)
		} else {
			id = fmt.Sprintf(":%d:%s:%s", i, key, val)
		}
		shds[i] = TagIndex{
			ID: id,
		}
	}
	if err := datastore.Get(ctx, shds); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return nil, errors.Annotate(err, "error fetching tag index for %q", fmt.Sprintf("%s:%s", key, val)).Err()
		}
		for _, e := range merr {
			if e != nil && e != datastore.ErrNoSuchEntity {
				return nil, errors.Annotate(err, "error fetching tag index for %q", fmt.Sprintf("%s:%s", key, val)).Err()
			}
		}
	}
	ents := make([]TagIndexEntry, 0)
	for _, s := range shds {
		if s.Incomplete {
			return nil, ErrTagIndexIncomplete
		}
		ents = append(ents, s.Entries...)
	}
	return ents, nil
}
