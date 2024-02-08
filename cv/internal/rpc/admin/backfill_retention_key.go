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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

var backfillRetentionKey = dsmapper.JobConfig{
	Mapper: "backfill-retention-key",
	Query: dsmapper.Query{
		Kind: "Tryjob",
	},
	PageSize:   32,
	ShardCount: 4,
}

var backfillRetentionKeyFactory = func(_ context.Context, j *dsmapper.Job, _ int) (dsmapper.Mapper, error) {
	return func(ctx context.Context, keys []*datastore.Key) error {
		if len(keys) == 0 {
			return nil
		}
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			tryjobs := make([]*tryjob.Tryjob, len(keys))
			for i, k := range keys {
				tryjobs[i] = &tryjob.Tryjob{
					ID: common.TryjobID(k.IntID()),
				}
			}
			if err := datastore.Get(ctx, tryjobs); err != nil {
				return errors.Annotate(err, "failed to fetch Tryjobs").Tag(transient.Tag).Err()
			}
			toUpdate := tryjobs[:0] // reuse the slice
			for _, tj := range tryjobs {
				if tj.RetentionKey == "" {
					tj.EVersion++
					toUpdate = append(toUpdate, tj)
				}
			}
			if len(toUpdate) == 0 {
				return nil
			}
			return datastore.Put(ctx, toUpdate)
		}, nil)
		if err != nil {
			return errors.Annotate(err, "failed to update Tryjobs").Tag(transient.Tag).Err()
		}
		return nil
	}, nil
}
