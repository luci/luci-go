// Copyright 2021 The LUCI Authors.
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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit/metadata"
)

var upgradeCLConfig = dsmapper.JobConfig{
	Mapper: "cl-metadata",
	Query: dsmapper.Query{
		Kind: "CL",
	},
	PageSize:   16,
	ShardCount: 4,
}

var upgradeCLFactory = func(context.Context, *dsmapper.Job, int) (dsmapper.Mapper, error) {
	return upgradeCLs, nil
}

func upgradeCLs(ctx context.Context, keys []*datastore.Key) error {
	needUpgrade := func(cls []*changelist.CL) []*changelist.CL {
		toUpdate := cls[:0]
		for _, cl := range cls {
			// NOTE: metadata.Extract returns empty slice, hence comparison
			// against nil, not len(...) == 0.
			if cl.Snapshot == nil || cl.Snapshot.GetMetadata() != nil {
				continue
			}
			toUpdate = append(toUpdate, cl)
		}
		return toUpdate
	}

	cls := make([]*changelist.CL, len(keys))
	for i, k := range keys {
		cls[i] = &changelist.CL{ID: common.CLID(k.IntID())}
	}

	// Check before a transaction if an update is even necessary.
	if err := datastore.Get(ctx, cls); err != nil {
		return errors.Annotate(err, "failed to fetch CLs").Tag(transient.Tag).Err()
	}
	cls = needUpgrade(cls)
	if len(cls) == 0 {
		return nil
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Reload inside transaction to avoid races with other CV parts.
		if err := datastore.Get(ctx, cls); err != nil {
			return errors.Annotate(err, "failed to fetch CLs").Tag(transient.Tag).Err()
		}
		cls = needUpgrade(cls)
		if len(cls) == 0 {
			return nil
		}
		now := datastore.RoundTime(clock.Now(ctx).UTC())
		for _, cl := range cls {
			ci := cl.Snapshot.GetGerrit().GetInfo()
			// metadata.Extract always returns a non-nil slice, possibly empty.
			cl.Snapshot.Metadata = metadata.Extract(ci.GetRevisions()[ci.GetCurrentRevision()].GetCommit().GetMessage())
			cl.UpdateTime = now
			cl.EVersion++
		}
		return datastore.Put(ctx, cls)
	}, nil)
	if err != nil {
		return errors.Annotate(err, "failed to update CLs").Tag(transient.Tag).Err()
	}
	return nil
}
