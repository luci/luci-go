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

package tryjob

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// LoadTryjobsByIDs loads the tryjobs with the given ids from datastore.
func LoadTryjobsByIDs(ctx context.Context, ids []common.TryjobID) ([]*Tryjob, error) {
	tryjobs := make([]*Tryjob, len(ids))
	for i, tjid := range ids {
		tryjobs[i] = &Tryjob{ID: tjid}
	}
	err := datastore.Get(ctx, tryjobs)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return tryjobs, nil
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Reason("tryjob %d not found in datastore", ids[i]).Err()
			}
		}
		count, err := merr.Summary()
		return nil, errors.Annotate(err, "failed to load %d out of %d tryjobs", count, len(ids)).Tag(transient.Tag).Err()
	default:
		return nil, errors.Annotate(err, "failed to load tryjobs").Tag(transient.Tag).Err()
	}
}

// LoadTryjobsMapByIDs get a map of tryjobs with the given ids from datastore.
func LoadTryjobsMapByIDs(ctx context.Context, ids []common.TryjobID) (map[common.TryjobID]*Tryjob, error) {
	switch tryjobs, err := LoadTryjobsByIDs(ctx, ids); {
	case err != nil:
		return nil, err
	default:
		tryjobMap := make(map[common.TryjobID]*Tryjob, len(ids))
		for _, tj := range tryjobs {
			tryjobMap[tj.ID] = tj
		}
		return tryjobMap, nil
	}
}
