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

package model

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

// How long to keep historical records in the datastore.
const retentionPeriod = 365 * 24 * time.Hour // ~1 year

// CleanupOldEntities deletes old entities to keep datastore tidy.
//
// Doesn't abort on errors, just logs them.
func CleanupOldEntities(ctx context.Context) error {
	ctx, done := clock.WithTimeout(ctx, 8*time.Minute)
	defer done()

	cutoff := clock.Now(ctx).Add(-retentionPeriod)

	wg := sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		cleanupOld(ctx, "Actuation", cutoff)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		cleanupOld(ctx, "AssetHistory", cutoff)
	}()

	return nil
}

func cleanupOld(ctx context.Context, kind string, cutoff time.Time) {
	q := datastore.NewQuery(kind).
		Lt("Created", cutoff.UTC()).
		KeysOnly(true)

	const batchSize = 250
	var batch []*datastore.Key

	flushBatch := func() {
		if len(batch) != 0 {
			logging.Infof(ctx, "Deleting %d old %s entities", len(batch), kind)
			if err := datastore.Delete(ctx, batch); err != nil {
				logging.Errorf(ctx, "Failed to delete some of %d %s entities: %s", len(batch), kind, err)
			}
			batch = batch[:0]
		}
	}

	err := datastore.RunBatch(ctx, batchSize, q, func(key *datastore.Key) {
		batch = append(batch, key)
		if len(batch) == batchSize {
			flushBatch()
		}
	})
	if err != nil {
		logging.Errorf(ctx, "Failed to fetch old %s entities: %s", kind, err)
	}
	flushBatch()
}
