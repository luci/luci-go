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

package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// ReportBuilderMetrics computes and reports Builder metrics.
func ReportBuilderMetrics(ctx context.Context) error {
	// Reset the metric to stop reporting no-longer-existing builders.
	tsmon.GetState(ctx).Store().Reset(ctx, V2.BuilderPresence)
	luciBuckets, err := fetchLUCIBuckets(ctx)
	if err != nil {
		return errors.Annotate(err, "fetching LUCI buckets w/ swarming config").Err()
	}

	return parallel.WorkPool(256, func(taskC chan<- func() error) {
		q := datastore.NewQuery(model.BuilderStatKind)
		err := datastore.RunBatch(ctx, 64, q, func(k *datastore.Key) error {
			project, bucket, builder := mustParseBuilderStatID(k.StringID())
			tctx := WithBuilder(ctx, project, bucket, builder)
			legacyBucket := bucket
			// V1 metrics format the bucket name in "luci.$project.$bucket"
			// if the bucket config has a swarming config.
			if luciBuckets.Has(protoutil.FormatBucketID(project, bucket)) {
				legacyBucket = legacyBucketName(project, bucket)
			}
			V2.BuilderPresence.Set(tctx, true)

			taskC <- func() error {
				return errors.Annotate(
					reportMaxAge(tctx, project, bucket, legacyBucket, builder),
					"reportMaxAge",
				).Err()
			}
			taskC <- func() error {
				return errors.Annotate(
					reportBuildCount(tctx, project, bucket, legacyBucket, builder),
					"reportBuildCount",
				).Err()
			}
			taskC <- func() error {
				return errors.Annotate(
					reportConsecutiveFailures(tctx, project, bucket, builder),
					"reportConsecutiveFailures",
				).Err()
			}
			return nil
		})
		if err != nil {
			taskC <- func() error { return errors.Annotate(err, "datastore.RunBatch").Err() }
		}
	})
}

func mustParseBuilderStatID(id string) (project, bucket, builder string) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		panic(fmt.Errorf("invalid BuilderStatID: %s", id))
	}
	project, bucket, builder = parts[0], parts[1], parts[2]
	return
}

// fetchLUCIBuckets returns a stringset.Set with the ID of the buckets
// w/ swarming config.
func fetchLUCIBuckets(ctx context.Context) (stringset.Set, error) {
	ret := stringset.Set{}
	err := datastore.RunBatch(
		ctx, 128, datastore.NewQuery(model.BucketKind),
		func(bucket *model.Bucket) error {
			if bucket.Proto.GetSwarming() != nil {
				ret.Add(protoutil.FormatBucketID(bucket.Parent.StringID(), bucket.ID))
			}
			return nil
		},
	)
	return ret, err
}

// reportMaxAge computes and reports the age of the oldest builds with SCHEDULED.
func reportMaxAge(ctx context.Context, project, bucket, legacyBucket, builder string) error {
	var leasedCT, neverLeasedCT time.Time
	q := datastore.NewQuery(model.BuildKind).
		Eq("bucket_id", protoutil.FormatBucketID(project, bucket)).
		Eq("tags", "builder:"+builder).
		Eq("status_v2", pb.Status_SCHEDULED).
		Eq("experimental", false).
		Order("create_time").
		Limit(1)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var b []*model.Build
		if err := datastore.GetAll(ctx, q.Eq("never_leased", false), &b); err != nil {
			return err
		}
		if len(b) > 0 {
			leasedCT = b[0].CreateTime
		}
		return nil
	})
	eg.Go(func() error {
		var b []*model.Build
		if err := datastore.GetAll(ctx, q.Eq("never_leased", true), &b); err != nil {
			return err
		}
		if len(b) > 0 {
			neverLeasedCT = b[0].CreateTime
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	var max, neverLeasedMax float64
	now := clock.Now(ctx)
	if !neverLeasedCT.IsZero() {
		neverLeasedMax = now.Sub(neverLeasedCT).Seconds()
	}

	// In V1, the metric value of a stream with "must_be_never_leased == false"
	// is the age of the oldest build w/ "must_be_never_leased == true|false".
	//
	// That is, it's the age of the oldest build regardless of the value
	// in must_be_never_leased.
	if !leasedCT.IsZero() {
		max = now.Sub(leasedCT).Seconds()
	}
	if max < neverLeasedMax {
		max = neverLeasedMax
	}
	V1.MaxAgeScheduled.Set(ctx, max, legacyBucket, builder, false /*must_be_never_leased*/)
	V1.MaxAgeScheduled.Set(ctx, neverLeasedMax, legacyBucket, builder, true)
	V2.MaxAgeScheduled.Set(ctx, max)
	return nil
}

// reportBuildCount computes and reports # of builds with SCHEDULED and STARTED.
func reportBuildCount(ctx context.Context, project, bucket, legacyBucket, builder string) error {
	var nScheduled, nStarted int64
	q := datastore.NewQuery(model.BuildKind).
		Eq("bucket_id", protoutil.FormatBucketID(project, bucket)).
		Eq("experimental", false).
		Eq("tags", "builder:"+builder)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		nScheduled, err = datastore.Count(ctx, q.Eq("status_v2", pb.Status_SCHEDULED))
		return
	})
	eg.Go(func() (err error) {
		nStarted, err = datastore.Count(ctx, q.Eq("status_v2", pb.Status_STARTED))
		return
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	V1.BuildCount.Set(ctx, nScheduled, legacyBucket, builder, pb.Status_name[int32(pb.Status_SCHEDULED)])
	V1.BuildCount.Set(ctx, nStarted, legacyBucket, builder, pb.Status_name[int32(pb.Status_STARTED)])
	V2.BuildCount.Set(ctx, nScheduled, pb.Status_name[int32(pb.Status_SCHEDULED)])
	V2.BuildCount.Set(ctx, nStarted, pb.Status_name[int32(pb.Status_STARTED)])
	return nil
}

func reportConsecutiveFailures(ctx context.Context, project, bucket, builder string) error {
	var b []*model.Build
	q := datastore.NewQuery(model.BuildKind).
		Eq("bucket_id", protoutil.FormatBucketID(project, bucket)).
		Eq("tags", "builder:"+builder).
		Order("-status_changed_time")
	if err := datastore.GetAll(ctx, q.Eq("status_v2", pb.Status_SUCCESS).Limit(1), &b); err != nil {
		return err
	}

	// if there was at least one successful build, add Ge()
	// to narrow the scope of the index scan.
	if len(b) > 0 {
		q = q.Gt("status_changed_time", b[0].StatusChangedTime.UTC())
	}

	var nCancels, nFailures, nInfraFailures int64
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		nCancels, err = datastore.Count(ctx, q.Eq("status_v2", pb.Status_CANCELED))
		return
	})
	eg.Go(func() (err error) {
		nFailures, err = datastore.Count(ctx, q.Eq("status_v2", pb.Status_FAILURE))
		return
	})
	eg.Go(func() (err error) {
		nInfraFailures, err = datastore.Count(ctx, q.Eq("status_v2", pb.Status_INFRA_FAILURE))
		return
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	// These counts can be inaccurate a bit, but should be accurate enough.
	V2.ConsecutiveFailureCount.Set(ctx, nCancels, pb.Status_name[int32(pb.Status_CANCELED)])
	V2.ConsecutiveFailureCount.Set(ctx, nFailures, pb.Status_name[int32(pb.Status_FAILURE)])
	V2.ConsecutiveFailureCount.Set(ctx, nInfraFailures, pb.Status_name[int32(pb.Status_INFRA_FAILURE)])
	return nil
}
