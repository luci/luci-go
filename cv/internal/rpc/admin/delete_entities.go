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
)

const deleteEntityKind = "migration.VerifiedCQDRun"

var deleteEntitiesKey = dsmapper.JobConfig{
	Mapper: "delete-entities",
	Query: dsmapper.Query{
		Kind: deleteEntityKind,
	},
	PageSize:   32,
	ShardCount: 4,
}

var deleteEntitiesFactory = func(_ context.Context, j *dsmapper.Job, _ int) (dsmapper.Mapper, error) {
	return func(ctx context.Context, keys []*datastore.Key) error {
		if len(keys) == 0 {
			return nil
		}
		var merrs errors.MultiError
		switch err := datastore.Delete(ctx, keys); {
		case err == nil:
			return nil
		case errors.As(err, merrs):
			for _, err := range merrs {
				if !errors.Is(err, datastore.ErrNoSuchEntity) {
					return transient.Tag.Apply(errors.Fmt("failed to delete keys: %w", merrs))
				}
			}
			return nil
		default:
			return errors.Annotate(err, "failed to delete keys").Tag(transient.Tag).Err()
		}
	}, nil
}
