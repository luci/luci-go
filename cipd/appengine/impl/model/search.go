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

package model

import (
	"context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

// SearchInstances lists instances of a package with all given tags attached.
//
// Only does a query over Instances entities. Doesn't check whether the Package
// entity exists. Returns up to pageSize entities, plus non-nil cursor (if
// there are more results). 'pageSize' must be positive.
func SearchInstances(ctx context.Context, pkg string, tags []*api.Tag, pageSize int32, cursor datastore.Cursor) (out []*Instance, nextCur datastore.Cursor, err error) {
	switch {
	case len(tags) == 0:
		panic("tags must not be empty")
	case pageSize <= 0:
		panic("pageSize must be positive")
	}

	// Due to the way our entities organized, we can't efficiently query by all
	// tags at once. Instead we query by the first tag only, and then filter the
	// result to keep only instances that have the rest of tags attached. We do it
	// chunk by chunk, until we assemble the full page.

	seen := stringset.New(len(tags))

	// The principal tag we query by.
	mainTag := common.JoinInstanceTag(tags[0])
	seen.Add(mainTag)

	// Rest of the tags, dedupped, in the original order (the order influences the
	// query efficiency, see below).
	auxTags := make([]*api.Tag, 0, len(tags)-1)
	for i := 1; i < len(tags); i++ {
		if t := common.JoinInstanceTag(tags[i]); !seen.Has(t) {
			auxTags = append(auxTags, tags[i])
			seen.Add(t)
		}
	}

	for int32(len(out)) < pageSize {
		// Iterate over query on mainTag. This returns keys of instances that have
		// mainTag attached.
		var page []*datastore.Key
		page, cursor, err = queryByTag(ctx, pkg, mainTag, cursor, pageSize-int32(len(out)))
		if err != nil {
			return nil, nil, err
		}

		// Filter out instances that don't have any of remaining tags[1:] tags
		// attached.
		//
		// Note that we can potentially filter by all tags at once here, hitting
		// len(page)*(len(tags)-1) entities in parallel. Instead we assume each
		// additional tag noticeably limits number of results, so we sequentially
		// filter the result tag by tag, eventually making fewer requests total.
		//
		// Assuming N = len(page), T = len(tags)-1, and k is a portion of a page
		// left after filtering by one tag (e.g. k = 0.7 if 7 out of 10 results
		// survive filtering by a tag):
		//
		// For parallel filtering we hit the following number of entities:
		//    N * T.
		//
		// For sequential filtering we hit:
		//    N * (1 + k + k^2 + ... + k^(T-1)) = N * (1.0 - k^(T-1)) / (1.0 - k).
		//
		// For N = 100, T = 4, k = 0.7 this results in:
		//    400 instances hit via 1 RPC vs ~220 instances hit via 4 RPCs.
		//
		// The real life difference of course dramatically depends on k. But in
		// general we expect the sequential strategy will be cheaper overall (in
		// terms of combined Datastore + CPU cost). This hasn't been confirmed by
		// hard numbers though.
		for _, tag := range auxTags {
			if len(page) == 0 {
				break
			}
			page, err = filterByTag(ctx, page, tag)
			if err != nil {
				return nil, nil, err
			}
		}

		// Fetch actual instance bodies. It is possible (though highly improbable),
		// that some instances are gone already, so len(instances) may be less than
		// len(page).
		instances, err := fetchExistingInstances(ctx, page)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, instances...)

		if cursor == nil {
			break // listed everything we could
		}
	}

	nextCur = cursor
	return
}

// queryByTag returns keys of Instances that have the given tag attached.
//
// Returns up to 'pageSize' of results, along with a cursor to continue the
// query or nil if it was the end of it.
func queryByTag(ctx context.Context, pkg, tag string, cursor datastore.Cursor, pageSize int32) (
	out []*datastore.Key,
	next datastore.Cursor,
	err error) {

	// TODO(vadimsh): 'registered_ts' here is when the tag was attached. The
	// callers likely expect results ordered by instance registration time. This
	// is not possible currently. We'll need to copy Instance.registered_ts into
	// InstanceTag entities, so we can order by instance registration time.

	// TODO(vadimsh): Should we exclude unprocessed instances from the result?

	q := datastore.NewQuery("InstanceTag").
		Ancestor(PackageKey(ctx, pkg)).
		Eq("tag", tag).
		Order("-registered_ts").
		Limit(pageSize).
		KeysOnly(true)
	if cursor != nil {
		q = q.Start(cursor)
	}

	err = datastore.Run(ctx, q, func(k *datastore.Key, cb datastore.CursorCB) error {
		out = append(out, k.Parent())
		if len(out) >= int(pageSize) {
			if next, err = cb(); err != nil {
				return err
			}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to query by tag %q", tag).Tag(transient.Tag).Err()
	}
	return
}

// filterByTag takes keys of Instance entities and keeps only ones that have the
// given tag attached (preserving the order).
//
// Fetches the tags to verify their Tag fields really match 'tag' as a safeguard
// against malicious SHA1 collision on TagID.
func filterByTag(ctx context.Context, page []*datastore.Key, tag *api.Tag) ([]*datastore.Key, error) {
	tagID := TagID(tag)
	tagEnts := make([]*Tag, len(page))
	for i, inst := range page {
		tagEnts[i] = &Tag{
			ID:       tagID,
			Instance: inst,
		}
	}
	existing, _, err := fetchTags(ctx, tagEnts, func(int) *api.Tag { return tag })
	if err != nil {
		return nil, errors.Annotate(err, "failed by filter by tag %q", common.JoinInstanceTag(tag)).Err()
	}
	filtered := page[:0]
	for _, tagEnt := range existing {
		filtered = append(filtered, tagEnt.Instance)
	}
	return filtered, nil
}

// fetchExistingInstances fetches Instance entities given their keys.
//
// Skips missing ones.
func fetchExistingInstances(ctx context.Context, keys []*datastore.Key) ([]*Instance, error) {
	instances := make([]*Instance, len(keys))
	for i, k := range keys {
		instances[i] = &Instance{
			InstanceID: k.StringID(),
			Package:    k.Parent(),
		}
	}

	err := datastore.Get(ctx, instances)
	if err == nil {
		return instances, nil
	}

	merr, ok := err.(errors.MultiError)
	if !ok {
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch instances: %w", err))
	}

	existing := instances[:0]
	for i, inst := range instances {
		switch err := merr[i]; {
		case err == nil:
			existing = append(existing, inst)
		case err != datastore.ErrNoSuchEntity:
			return nil, errors.Annotate(err, "failed to fetch instance %q", inst.InstanceID).Tag(transient.Tag).Err()
		}
	}
	return existing, nil
}
