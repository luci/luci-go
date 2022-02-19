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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func expireBuilds(ctx context.Context, bs []*model.Build, mr parallel.MultiRunner) error {
	nOrig := len(bs)
	if nOrig == 0 {
		return nil
	}

	toUpdate := make([]*model.Build, 0, len(bs))
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Get(ctx, bs); err != nil {
			return err
		}

		now := clock.Now(ctx)
		for _, b := range bs {
			// skip updating, if it's no longer in a non-terminal status.
			if protoutil.IsEnded(b.Proto.Status) {
				continue
			}

			protoutil.SetStatus(now, b.Proto, pb.Status_INFRA_FAILURE)
			if b.Proto.StatusDetails == nil {
				b.Proto.StatusDetails = &pb.StatusDetails{}
			}
			b.Proto.StatusDetails.Timeout = &pb.StatusDetails_Timeout{}
			b.ClearLease()
			toUpdate = append(toUpdate, b)
		}

		if len(toUpdate) == 0 {
			return nil
		}
		return mr.RunMulti(func(workC chan<- func() error) {
			for _, b := range toUpdate {
				b := b
				workC <- func() error { return tasks.NotifyPubSub(ctx, b) }
				workC <- func() error {
					return tasks.ExportBigQuery(ctx, &taskdefs.ExportBigQuery{BuildId: b.ID})
				}
				workC <- func() error {
					return tasks.FinalizeResultDB(ctx, &taskdefs.FinalizeResultDB{BuildId: b.ID})
				}
			}
			workC <- func() error { return datastore.Put(ctx, toUpdate) }
		})
	}, nil)

	if err == nil {
		logging.Infof(ctx, "Processed %d/%d expired builds", len(toUpdate), nOrig)
		for _, b := range toUpdate {
			logging.Infof(ctx, "Build %d: completed by cron(expire_builds) with status %q.",
				b.ID, b.Status)
			metrics.BuildCompleted(ctx, b)
		}
	}
	return err
}

// TimeoutExpiredBuilds marks incomplete builds that were created longer than
// model.BuildMaxCompletionTime w/ INFRA_FAILURE.
func TimeoutExpiredBuilds(ctx context.Context) error {
	const batchSize = 32
	// Processing each batch requires at most 5 goroutines.
	// - 1 for ds.RunTransaction()
	// - 4 for add tasks into TQ and ds.Put()
	//
	// Also, there is another goroutine for scanning expired builds.
	// Hence, this can run at most 6 transactions in parallel.
	const nWorkers = 32
	q := datastore.NewQuery(model.BuildKind).
		Gt("__key__", buildKeyByAge(ctx, model.BuildMaxCompletionTime)).
		KeysOnly(true)

	return parallel.RunMulti(ctx, nWorkers, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(workC chan<- func() error) {
			ch := make(chan []*model.Build, nWorkers)
			workC <- func() error {
				defer close(ch)

				// Queries within a transcation must include an Ancestor filter.
				// Hence, this searches expired builds out of a transaction first,
				// and then update them in a transaction.
				for _, st := range []pb.Status{pb.Status_SCHEDULED, pb.Status_STARTED} {
					bs := make([]*model.Build, 0, batchSize)
					err := datastore.RunBatch(ctx, int32(batchSize), q.Eq("status_v2", st),
						func(b *model.Build) error {
							bs = append(bs, b)
							if len(bs) == batchSize {
								ch <- bs
								bs = make([]*model.Build, 0, batchSize)
							}
							return nil
						},
					)
					if len(bs) > 0 {
						ch <- bs
					}
					if err != nil {
						return errors.Annotate(err, "querying expired %s builds", st).Err()
					}
				}
				return nil
			}

			for bs := range ch {
				bs := bs
				workC <- func() error {
					return errors.Annotate(expireBuilds(ctx, bs, mr), "expireBuilds").Err()
				}
			}
		})
	})
}

func resetLeases(ctx context.Context, bs []*model.Build, mr parallel.MultiRunner) error {
	nOrig := len(bs)
	if nOrig == 0 {
		return nil
	}

	toReset := make([]*model.Build, 0, len(bs))
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Get(ctx, bs); err != nil {
			return err
		}

		now := clock.Now(ctx)
		for _, b := range bs {
			if !b.IsLeased || b.LeaseExpirationDate.After(now) {
				continue
			}
			// A terminated build cannot be leased.
			// It must be that the data is corrupted or there is a bug.
			if protoutil.IsEnded(b.Proto.Status) {
				logging.Errorf(ctx, "Build %d is leased and terminated", b.ID)
			} else {
				protoutil.SetStatus(now, b.Proto, pb.Status_SCHEDULED)
			}
			b.ClearLease()
			toReset = append(toReset, b)
		}
		if len(toReset) == 0 {
			return nil
		}
		return mr.RunMulti(func(workC chan<- func() error) {
			for _, b := range toReset {
				b := b
				workC <- func() error { return tasks.NotifyPubSub(ctx, b) }
			}
			workC <- func() error { return datastore.Put(ctx, toReset) }
		})
	}, nil)

	if err == nil {
		logging.Infof(ctx, "Reset %d/%d expired leases.", len(toReset), nOrig)
		for _, b := range toReset {
			logging.Infof(ctx, "Build %d: expired lease was reset", b.ID)
			metrics.ExpiredLeaseReset(ctx, b)
		}
	}
	return err
}

// ResetExpiredLeases resets expired leases.
func ResetExpiredLeases(ctx context.Context) error {
	const batchSize = 32
	const nWorkers = 12
	q := datastore.NewQuery(model.BuildKind).
		Eq("is_leased", true).
		Lte("lease_expiration_date", clock.Now(ctx).UTC()).
		KeysOnly(true)

	return parallel.RunMulti(ctx, nWorkers, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(workC chan<- func() error) {
			ch := make(chan []*model.Build, nWorkers)
			workC <- func() error {
				defer close(ch)
				bs := make([]*model.Build, 0, batchSize)
				err := datastore.RunBatch(ctx, int32(batchSize), q, func(b *model.Build) error {
					bs = append(bs, b)
					if len(bs) == batchSize {
						ch <- bs
						bs = make([]*model.Build, 0, batchSize)
					}
					return nil
				})
				if len(bs) > 0 {
					ch <- bs
				}
				return errors.Annotate(err, "querying expired, leased builds").Err()
			}

			for bs := range ch {
				bs := bs
				workC <- func() error {
					return errors.Annotate(resetLeases(ctx, bs, mr),
						"resetting %d expired leases", len(bs)).Err()
				}
			}
		})
	})
}
