// Copyright 2018 The LUCI Authors.
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

package admin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	"go.chromium.org/luci/cipd/appengine/impl/model"
)

// markedTag is a root entity created for each tag marked by some particular
// mapper operation.
//
// We can create many of these at arbitrary rate. Once the mapper finishes
// running and (eventually consistent) datastore indexes settle, we can query
// for all such marked tags using Eq() filter and deal with them in arbitrary
// order or at arbitrary rate.
//
// The core assumption here is that number of "interesting" tags found by
// a mapper is much less than total number of tags, but they may be clustered
// close together (so naively updating them inside the mapper causes transaction
// collisions).
type markedTag struct {
	_kind  string                `gae:"$kind,mapper.MarkedTag"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID  string         `gae:"$id"` // see .genID()
	Job dsmapper.JobID // ID of a mapping job that produced it, for queries

	Key *datastore.Key `gae:",noindex"` // key of the corresponding Tag entity
	Tag string         `gae:",noindex"` // the original tag string (k:v)
	Why string         `gae:",noindex"` // why the tag was marked, for humans
}

// genID derives an ID for this markedTag entity.
//
// Called internally by visitAndMarkTags().
//
// Each unique tag marked by some particular job gets its own key, i.e. each
// job has its own namespace for marked tags.
func (t *markedTag) genID() {
	h := sha256.New()
	fmt.Fprintf(h, "%d\n%s", t.Job, t.Key.Encode())
	t.ID = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// queryMarkedTags returns a query for markedTags entities produced by a job.
func queryMarkedTags(job dsmapper.JobID) *datastore.Query {
	return datastore.NewQuery("mapper.MarkedTag").Eq("Job", job)
}

// multiGetTags fetches a bunch of tags using GetMulti.
//
// On overall RPC error returns a transient error.
//
// If it managed to fetch something, calls cb(key, tag) sequentially for
// all tags that were found (silently skipping missing ones). If a tag was not
// fetched for some other reason, returns the error right away.
//
// If the callback returns an error this error is returned immediately by
// multiGetTags.
func multiGetTags(ctx context.Context, keys []*datastore.Key, cb func(*datastore.Key, *model.Tag) error) error {
	tags := make([]model.Tag, len(keys))
	for i, k := range keys {
		tags[i] = model.Tag{
			ID:       k.StringID(),
			Instance: k.Parent(),
		}
	}

	errAt := func(idx int) error { return nil }
	if err := datastore.Get(ctx, tags); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return transient.Tag.Apply(errors.Fmt("GetMulti RPC error when fetching %d tags: %w", len(tags), err))
		}
		errAt = func(idx int) error { return merr[idx] }
	}

	for i, t := range tags {
		switch err := errAt(i); {
		case err == datastore.ErrNoSuchEntity:
			continue
		case err != nil:
			return errors.Fmt("failed to fetch tag entity with key %s: %w", keys[i], err)
		}
		if err := cb(keys[i], &t); err != nil {
			return err
		}
	}
	return nil
}

// visitAndMarkTags fetches Tag entities given their keys and passes them to
// the callback, which may choose to mark a tag by returning non-empty string
// with the human-readable reason why the tag was marked.
//
// Such marked tags are then stored in the datastore and later can be queried.
func visitAndMarkTags(ctx context.Context, job dsmapper.JobID, keys []*datastore.Key, cb func(*model.Tag) string) error {
	var marked []*markedTag
	err := multiGetTags(ctx, keys, func(key *datastore.Key, tag *model.Tag) error {
		if why := cb(tag); why != "" {
			mt := &markedTag{
				Job: job,
				Key: key,
				Tag: tag.Tag,
				Why: why,
			}
			mt.genID()
			marked = append(marked, mt)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := datastore.Put(ctx, marked); err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to store %d markedTag(s): %w", len(marked), err))
	}
	return nil
}
