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
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"
)

var deleteFinishedCQDRunConfig = dsmapper.JobConfig{
	Mapper: "delete-FinishedCQDRun",
	Query: dsmapper.Query{
		Kind: "migration.FinishedCQDRun",
	},
	PageSize:   128,
	ShardCount: 4,
}

var deleteFinishedCQDRunFactory = func(_ context.Context, j *dsmapper.Job, _ int) (dsmapper.Mapper, error) {
	tsJobName := string(j.Config.Mapper)
	tsJobID := int64(j.ID)

	deleteMany := func(ctx context.Context, keys []*datastore.Key) error {
		// Quick sanity check that we are deleting FinishedCQDRun and nothing
		// something else.
		expKey, err := datastore.KeyForObjErr(ctx, &migration.FinishedCQDRun{AttemptKey: "beef"})
		if err != nil {
			return err
		}
		for _, k := range keys {
			if k.Kind() != expKey.Kind() {
				panic(fmt.Errorf("deleteFinishedCQDRunConfig wants to delete %s, but %s expected", k.Kind(), expKey.Kind()))
			}
		}

		// OK, safe enough to delete.
		if err = datastore.Delete(ctx, keys); err != nil {
			return errors.Annotate(err, "failed to delete FinishedCQDRuns").Tag(transient.Tag).Err()
		}
		metricUpgraded.Add(ctx, int64(len(keys)), tsJobName, tsJobID, "FinishedCQDRun")
		return nil
	}

	return deleteMany, nil
}
