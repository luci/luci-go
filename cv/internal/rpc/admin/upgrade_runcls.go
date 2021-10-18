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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

var upgradeRunCLConfig = dsmapper.JobConfig{
	Mapper: "runcl-externalid",
	Query: dsmapper.Query{
		Kind: "RunCL",
	},
	PageSize:   16,
	ShardCount: 4,
}

var upgradeRunCLFactory = func(ctx context.Context, j *dsmapper.Job, shard int) (dsmapper.Mapper, error) {
	tsJobName := string(j.Config.Mapper)
	tsJobID := int64(j.ID)

	upgradeRunCLs := func(ctx context.Context, keys []*datastore.Key) error {
		needUpgrade := func(rcls []*run.RunCL) []*run.RunCL {
			toUpdate := rcls[:0]
			for _, rcl := range rcls {
				if rcl.ExternalID == "" {
					toUpdate = append(toUpdate, rcl)
				}
			}
			return toUpdate
		}

		rcls := make([]*run.RunCL, len(keys))
		for i, k := range keys {
			rcls[i] = &run.RunCL{
				ID:  common.CLID(k.IntID()),
				Run: k.Parent(),
			}
		}

		// Most RunCLs are already correct.
		// So, check before a transaction if an update is even necessary.
		if err := datastore.Get(ctx, rcls); err != nil {
			return errors.Annotate(err, "failed to fetch RunCLs").Tag(transient.Tag).Err()
		}
		rcls = needUpgrade(rcls)
		if len(rcls) == 0 {
			return nil
		}

		// Compute CLID -> ExternalID map by loading corresponding CL entities.
		cls := make([]*changelist.CL, len(rcls))
		for i, rcl := range rcls {
			cls[i] = &changelist.CL{ID: rcl.ID}
		}
		if err := datastore.Get(ctx, cls); err != nil {
			return errors.Annotate(err, "failed to fetch CLs").Tag(transient.Tag).Err()
		}
		mp := make(map[common.CLID]changelist.ExternalID, len(cls))
		for _, cl := range cls {
			mp[cl.ID] = cl.ExternalID
		}

		// Finally, update RunCL entities.
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			// Reload inside transaction to avoid any races.
			if err := datastore.Get(ctx, rcls); err != nil {
				return errors.Annotate(err, "failed to fetch RunCLs").Tag(transient.Tag).Err()
			}
			rcls = needUpgrade(rcls)
			if len(rcls) == 0 {
				return nil
			}
			for _, rcl := range rcls {
				rcl.ExternalID = mp[rcl.ID]
			}
			return datastore.Put(ctx, rcls)
		}, nil)
		if err != nil {
			return errors.Annotate(err, "failed to update RunCLs").Tag(transient.Tag).Err()
		}
		metricUpgraded.Add(ctx, int64(len(rcls)), tsJobName, tsJobID, "RunCL")
		return nil
	}

	return upgradeRunCLs, nil
}
