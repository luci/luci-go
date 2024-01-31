// Copyright 2024 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
)

var backfillRetentionKey = dsmapper.JobConfig{
	Mapper: "backfill-retention-key",
	Query: dsmapper.Query{
		Kind: "CL",
	},
	PageSize:   32,
	ShardCount: 4,
}

var backfillRetentionKeyFactory = func(_ context.Context, j *dsmapper.Job, _ int) (dsmapper.Mapper, error) {
	return func(ctx context.Context, keys []*datastore.Key) error {
		if len(keys) == 0 {
			return nil
		}
		updated := 0
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			cls := make([]*changelist.CL, len(keys))
			for i, k := range keys {
				cls[i] = &changelist.CL{
					ID: common.CLID(k.IntID()),
				}
			}
			if err := datastore.Get(ctx, cls); err != nil {
				return errors.Annotate(err, "failed to fetch CLs").Tag(transient.Tag).Err()
			}
			toUpdate := cls[:0]
			for _, cl := range cls {
				if cl.RetentionKey == "" {
					cl.UpdateRetentionKey()
					cl.EVersion++
					toUpdate = append(toUpdate, cl)
				}
			}
			updated = len(toUpdate)
			if updated == 0 {
				return nil
			}
			return datastore.Put(ctx, toUpdate)
		}, nil)
		if err != nil {
			return errors.Annotate(err, "failed to update CLs").Tag(transient.Tag).Err()
		}
		return nil
	}, nil
}
