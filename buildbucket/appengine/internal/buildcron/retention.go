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

package buildcron

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/model"
)

// buildKeyByAge returns a datastore key for a given age old Build.
func buildKeyByAge(ctx context.Context, age time.Duration) *datastore.Key {
	now := clock.Now(ctx)
	// the low time yields the high ID.
	// i.e., 1st param yields 2nd element in the return tuple.
	_, idHigh := buildid.IDRange(
		now.Add(-age), /* low time */
		now,           /* high time */
	)
	return datastore.MakeKey(ctx, model.BuildKind, idHigh)
}

func deleteBuilds(ctx context.Context, keys []*datastore.Key) error {
	if len(keys) == 0 {
		return nil
	}
	// flatten first w/o filtering to calculate how many builds were actually
	// removed.
	allErrs := 0
	badErrs := 0
	err := errors.Flatten(datastore.Delete(ctx, keys))
	if err != nil {
		allErrs = len(err.(errors.MultiError))
		// ignore it; multiple crons are running??
		err = errors.Filter(err, datastore.ErrNoSuchEntity)
		err = errors.Flatten(err)
		badErrs = len(err.(errors.MultiError))
	}
	if allErrs < len(keys) {
		logging.Infof(ctx, "Removed %d out of %d builds (%d errors, %d already gone)",
			len(keys)-(allErrs-badErrs), len(keys), badErrs, allErrs-badErrs)
	}
	return err
}

// DeleteOldBuilds deletes builds and other descendants that were created longer
// than model.BuildStorageDuration ago.
func DeleteOldBuilds(ctx context.Context) error {
	const batchSize = 128
	const nWorkers = 4
	// kindless query for Build and all of its descendants.
	q := datastore.NewQuery("").
		Gt("__key__", buildKeyByAge(ctx, model.BuildStorageDuration)).
		Lt("__key__", datastore.MakeKey(ctx, model.BuildKind, buildid.BuildIDMax)).
		KeysOnly(true)

	return parallel.WorkPool(nWorkers, func(workC chan<- func() error) {
		ch := make(chan []*datastore.Key, nWorkers)
		workC <- func() error {
			defer close(ch)

			bks := make([]*datastore.Key, 0, batchSize)
			err := datastore.RunBatch(ctx, int32(batchSize), q, func(bk *datastore.Key) error {
				bks = append(bks, bk)
				if len(bks) == batchSize {
					ch <- bks
					bks = make([]*datastore.Key, 0, batchSize)
				}
				return nil
			})
			if len(bks) > 0 {
				ch <- bks
			}
			return err
		}

		for bks := range ch {
			workC <- func() error { return deleteBuilds(ctx, bks) }
		}
	})
}
