// Copyright 2022 The LUCI Authors.
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

package buildercron

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
)

// RemoveInactiveBuilderStats removes inactive BuilderStats.
//
// BuilderStat is inactive, if either or both of the following conditions are
// true.
// - the Builder hasn't scheduled a build for model.BuilderExpirationDuration
// - the Builder config no longer exists.
func RemoveInactiveBuilderStats(ctx context.Context) error {
	var toDelete []*datastore.Key
	q := datastore.NewQuery(model.BuilderStatKind)
	err := datastore.RunBatch(ctx, 128, q, func(stat *model.BuilderStat) error {
		bk := stat.BuilderKey(ctx)
		if clock.Since(ctx, stat.LastScheduled) > model.BuilderExpirationDuration {
			// Inactive for too long.
			logging.Infof(ctx, "%s: BuilderStat.LastScheduled(%s) is too old; removing BuilderStat",
				stat.ID, stat.LastScheduled)
			toDelete = append(toDelete, datastore.KeyForObj(ctx, stat))
			return nil
		}
		switch r, err := datastore.Exists(ctx, bk); {
		case err != nil:
			return err
		case !r.Any():
			// The Builder config no longer exists?
			logging.Infof(ctx, "%s: the Builder no longer exists; removing BuilderStat", stat.ID)
			toDelete = append(toDelete, datastore.KeyForObj(ctx, stat))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return datastore.Delete(ctx, toDelete)
}
