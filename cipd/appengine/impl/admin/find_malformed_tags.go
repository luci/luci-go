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
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	api "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/common"
)

func init() {
	initMapper(mapperDef{
		Kind: api.MapperKind_FIND_MALFORMED_TAGS,
		Func: findMalformedTagsMapper,
		Config: dsmapper.JobConfig{
			Query:         dsmapper.Query{Kind: "InstanceTag"},
			ShardCount:    512,
			PageSize:      256, // note: 500 is a strict limit imposed by GetMulti
			TrackProgress: true,
		},
	})
}

func findMalformedTagsMapper(ctx context.Context, job dsmapper.JobID, _ *api.JobConfig, keys []*datastore.Key) error {
	return visitAndMarkTags(ctx, job, keys, func(t *model.Tag) string {
		if err := common.ValidateInstanceTag(t.Tag); err != nil {
			return err.Error()
		}
		return ""
	})
}

func fixMarkedTags(ctx context.Context, job dsmapper.JobID) (fixed []*api.TagFixReport_Tag, err error) {
	ctx, cancel := clock.WithTimeout(ctx, time.Minute)
	defer cancel()

	var marked []markedTag
	if err := datastore.GetAll(ctx, queryMarkedTags(job), &marked); err != nil {
		return nil, transient.Tag.Apply(errors.
			Fmt("failed to query marked tags: %w", err))
	}

	// Partition all tags per entity group they belong too, to avoid concurrent
	// transactions hitting same group.
	perRoot := map[string][]*datastore.Key{}
	for _, t := range marked {
		root := t.Key.Root().Encode()
		perRoot[root] = append(perRoot[root], t.Key)
	}

	// Fix tags in each entity group in parallel, because why not. We assume here
	// the number of tags to be fixed is small (so transactions are small and
	// don't timeout and don't OOM).
	err = parallel.WorkPool(32, func(tasks chan<- func() error) {
		var mu sync.Mutex
		for _, keys := range perRoot {
			root := keys[0].Root()
			tasks <- func() error {
				var fixedHere []*api.TagFixReport_Tag
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
					fixedHere, err = txnFixTagsInEG(ctx, keys)
					return err
				}, nil)
				if err != nil {
					return errors.Fmt("in entity group %s: %w", root, err)
				}
				mu.Lock()
				fixed = append(fixed, fixedHere...)
				mu.Unlock()
				return nil
			}
		}
	})
	return fixed, transient.Tag.Apply(err)
}

func txnFixTagsInEG(ctx context.Context, keys []*datastore.Key) (report []*api.TagFixReport_Tag, err error) {
	err = multiGetTags(ctx, keys, func(key *datastore.Key, tag *model.Tag) error {
		out := &api.TagFixReport_Tag{
			Pkg:       key.Parent().Parent().StringID(),
			Instance:  key.Parent().StringID(),
			BrokenTag: tag.Tag,
		}
		if common.ValidateInstanceTag(tag.Tag) == nil {
			logging.Infof(ctx, "In %s:%s - skipping tag %q, it is not broken anymore", out.Pkg, out.Instance, tag.Tag)
			return nil
		}

		// Maybe we can just strip whitespace to "fix" the tag?
		fixed, err := common.ParseInstanceTag(strings.TrimSpace(tag.Tag))
		if err != nil {
			fixed = nil // nope, still broken, just need to delete it then.
		}

		// Delete the old tag no matter what, it is broken.
		if err := datastore.Delete(ctx, key); err != nil {
			return errors.Fmt("failed to delete the tag %s: %w", key, err)
		}

		// Create the new tag if we managed to "fix" the deleted one.
		if fixed != nil {
			fixedTag := *tag
			fixedTag.ID = model.TagID(fixed)
			fixedTag.Tag = common.JoinInstanceTag(fixed)
			logging.Infof(ctx, "In %s:%s - replacing tag %q => %q", out.Pkg, out.Instance, tag.Tag, fixedTag.Tag)
			if err := datastore.Put(ctx, &fixedTag); err != nil {
				return errors.Fmt("failed to create a fixed tag %s instead of %s: %w", fixedTag.Tag, key, err)
			}
			out.FixedTag = fixedTag.Tag
		} else {
			logging.Infof(ctx, "In %s:%s - deleting tag %q", out.Pkg, out.Instance, tag.Tag)
		}

		// Record what we have done for the API response. No need for a lock,
		// multiGetTags calls the callback sequentially.
		report = append(report, out)
		return nil
	})
	return
}
