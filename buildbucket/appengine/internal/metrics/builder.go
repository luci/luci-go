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
	"math/rand"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
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
		return errors.Fmt("fetching LUCI buckets w/ swarming config: %w", err)
	}

	bldrToMetrics, err := getBldrDefinedMetrics(ctx)
	if err != nil {
		return err
	}

	// Visit builders in randomized order to avoid giving preference to ones
	// that come first in alphabetical order.
	builders, err := datastore.RunQuery[*datastore.Key](ctx,
		datastore.NewQuery(model.BuilderStatKind),
	).AsSlice()
	if err != nil {
		return fmt.Errorf("fetching the list of builders: %w", err)
	}
	rand.Shuffle(len(builders), func(i, j int) { builders[i], builders[j] = builders[j], builders[i] })

	logging.Infof(ctx, "About to visit %d builders", len(builders))
	return parallel.WorkPool(4096, func(taskC chan<- func() error) {
		for _, k := range builders {
			project, bucket, builder := mustParseBuilderStatID(k.StringID())
			tctx := WithBuilder(ctx, project, bucket, builder)
			legacyBucket := bucket
			// V1 metrics format the bucket name in "luci.$project.$bucket"
			// if the bucket config has a swarming config.
			if luciBuckets.Has(protoutil.FormatBucketID(project, bucket)) {
				legacyBucket = legacyBucketName(project, bucket)
			}
			V2.BuilderPresence.Set(tctx, true)

			bldrDefinedMetrics := bldrToMetrics[protoutil.ToBuilderIDString(project, bucket, builder)]

			for name, cb := range map[string]func(context.Context, string, string, string, string, []string) error{
				"reportMaxAge":              reportMaxAge,
				"reportBuildCount":          reportBuildCount,
				"reportConsecutiveFailures": reportConsecutiveFailures,
			} {
				taskC <- func() error {
					start := clock.Now(tctx)
					err := cb(tctx, project, bucket, legacyBucket, builder, bldrDefinedMetrics)
					dt := clock.Since(tctx, start)
					if err != nil {
						logging.Warningf(tctx, "%s(%q) failed in %s: %s", name, k.StringID(), dt, err)
						return fmt.Errorf("%s(%q): %w", name, k.StringID(), err)
					}
					logging.Infof(tctx, "%s(%q) succeeded in %s", name, k.StringID(), dt)
					return nil
				}
			}
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
	for bucket, err := range datastore.RunBatchQuery[*model.Bucket](
		ctx, 128, datastore.NewQuery(model.BucketKind),
	).Results {
		if err != nil {
			return nil, err
		}
		if bucket.Proto.GetSwarming() != nil {
			ret.Add(protoutil.FormatBucketID(bucket.Parent.StringID(), bucket.ID))
		}
	}
	return ret, nil
}

// reportMaxAge computes and reports the age of the oldest builds with SCHEDULED.
//
// TODO: This can be merged into reportBuildCount without any performance
// degradation.
func reportMaxAge(ctx context.Context, project, bucket, legacyBucket, builder string, bldrDefinedMetrics []string) error {
	// TODO(b/357664566): Make builder a dedicated field in Build model and use
	// it instead of tags in the query.
	q := datastore.NewQuery(model.BuildKind).
		Eq("bucket_id", protoutil.FormatBucketID(project, bucket)).
		Eq("tags", "builder:"+builder).
		Eq("status_v2", pb.Status_SCHEDULED).
		Eq("experimental", false).
		Order("create_time").
		Limit(1)

	wg := newWorkGroup()

	var leastCTOK bool
	var leasedCT time.Time
	wg.Go(func() error {
		b, err := datastore.RunQuery[*model.Build](ctx, q.Eq("never_leased", false)).AsSlice()
		if err != nil {
			return fmt.Errorf("query over never_leased==false: %w", err)
		}
		if len(b) > 0 {
			leasedCT = b[0].CreateTime
		}
		leastCTOK = true
		return nil
	})

	var neverLeasedCTOK bool
	var neverLeasedCT time.Time
	wg.Go(func() error {
		b, err := datastore.RunQuery[*model.Build](ctx, q.Eq("never_leased", true)).AsSlice()
		if err != nil {
			return fmt.Errorf("query over never_leased==true: %w", err)
		}
		if len(b) > 0 {
			neverLeasedCT = b[0].CreateTime
		}
		neverLeasedCTOK = true
		return nil
	})

	base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED
	cms, cmNames := getBuilderDefinedMetricsPerBase(ctx, base, bldrDefinedMetrics)
	cmRes := make(map[string]time.Time)
	var mu sync.Mutex
	for _, name := range cmNames {
		wg.Go(func() (err error) {
			b, err := datastore.RunQuery[*model.Build](ctx, q.Eq("custom_builder_max_age_metrics", name)).AsSlice()
			if err != nil {
				return err
			}
			if len(b) > 0 {
				mu.Lock()
				cmRes[name] = b[0].CreateTime
				mu.Unlock()
			}
			return nil
		})
	}
	queryErr := wg.Wait()

	now := clock.Now(ctx)

	// Report MaxAgeScheduled only when successfully fetched necessary values.
	if leastCTOK && neverLeasedCTOK {
		var neverLeasedMax float64
		if !neverLeasedCT.IsZero() {
			neverLeasedMax = now.Sub(neverLeasedCT).Seconds()
		}

		// In V1, the metric value of a stream with "must_be_never_leased == false"
		// is the age of the oldest build w/ "must_be_never_leased == true|false".
		//
		// That is, it's the age of the oldest build regardless of the value
		// in must_be_never_leased.
		var max float64
		if !leasedCT.IsZero() {
			max = now.Sub(leasedCT).Seconds()
		}
		if max < neverLeasedMax {
			max = neverLeasedMax
		}
		V1.MaxAgeScheduled.Set(ctx, max, legacyBucket, builder, false /*must_be_never_leased*/)
		V1.MaxAgeScheduled.Set(ctx, neverLeasedMax, legacyBucket, builder, true)
		V2.MaxAgeScheduled.Set(ctx, max)
	}

	// Report all custom metrics that we managed to fetch successfully.
	for _, name := range cmNames {
		if cmRes[name].IsZero() {
			continue
		}
		max := now.Sub(cmRes[name]).Seconds()
		cms.Report(ctx, &Report{
			Base:  base,
			Name:  name,
			Value: max,
		})
	}

	return queryErr
}

// reportBuildCount computes and reports # and age of builds with SCHEDULED and STARTED.
func reportBuildCount(ctx context.Context, project, bucket, legacyBucket, builder string, bldrDefinedMetrics []string) error {
	// Local query stats, not reported.
	var visited int64
	var skipped int64

	// Reported as built-in metrics.
	var scheduled int64
	var started int64
	agesScheduled := distribution.New(V2.Age.Bucketer())

	// Reported as custom metrics.
	base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT
	cms, cmNames := getBuilderDefinedMetricsPerBase(ctx, base, bldrDefinedMetrics)
	cmCounts := make(map[string]map[pb.Status]int64)
	for _, name := range cmNames {
		cmCounts[name] = make(map[pb.Status]int64)
	}

	// A query over all SCHEDULED and STARTED builds. This directly hits a
	// composite index over these exact two fields. To avoid messing with it,
	// we'll filter out experimental builds manually (there shouldn't be too many
	// of them).
	q := datastore.NewQuery(model.BuildKind).
		Eq("builder_id", protoutil.ToBuilderIDString(project, bucket, builder)).
		Eq("incomplete", true)
	for build, err := range datastore.RunBatchQuery[*model.Build](ctx, 1024, q).Results {
		if err != nil {
			// Note we can't really report any of metrics if we failed to visit all
			// active builds, because we will be undercounting them and reporting
			// wrong results.
			return fmt.Errorf("enumerating active builds (after visiting %d builds and skipping %d builds): %w", visited, skipped, err)
		}
		if build.Experimental {
			skipped++
			continue
		}

		switch build.Status {
		case pb.Status_SCHEDULED:
			scheduled++
			agesScheduled.Add(clock.Since(ctx, build.Proto.CreateTime.AsTime()).Seconds())
		case pb.Status_STARTED:
			started++
		default:
			skipped++
			continue
		}

		for _, cm := range build.CustomBuilderCountMetrics {
			if known := cmCounts[cm]; known != nil {
				known[build.Status]++
			}
		}
	}

	V1.BuildCount.Set(ctx, scheduled, legacyBucket, builder, pb.Status_name[int32(pb.Status_SCHEDULED)])
	V1.BuildCount.Set(ctx, started, legacyBucket, builder, pb.Status_name[int32(pb.Status_STARTED)])
	V2.BuildCount.Set(ctx, scheduled, pb.Status_name[int32(pb.Status_SCHEDULED)])
	V2.BuildCount.Set(ctx, started, pb.Status_name[int32(pb.Status_STARTED)])
	V2.Age.Set(ctx, agesScheduled, pb.Status_name[int32(pb.Status_SCHEDULED)])

	reportCustomBuilderCounts(ctx, cms, base, cmNames, []pb.Status{pb.Status_SCHEDULED, pb.Status_STARTED}, cmCounts)
	return nil
}

func reportConsecutiveFailures(ctx context.Context, project, bucket, legacyBucket, builder string, bldrDefinedMetrics []string) error {
	statuses := []pb.Status{
		pb.Status_CANCELED,
		pb.Status_FAILURE,
		pb.Status_INFRA_FAILURE,
	}

	// TODO(b/357664566): Make builder a dedicated field in Build model and use
	// it instead of tags in the query.
	q := datastore.NewQuery(model.BuildKind).
		Eq("bucket_id", protoutil.FormatBucketID(project, bucket)).
		Eq("tags", "builder:"+builder).
		Order("-status_changed_time")
	b, err := datastore.RunQuery[*model.Build](ctx, q.Eq("status_v2", pb.Status_SUCCESS).Limit(1)).AsSlice()
	if err != nil {
		return fmt.Errorf("finding last successful build: %w", err)
	}

	// if there was at least one successful build, add Ge()
	// to narrow the scope of the index scan.
	if len(b) > 0 {
		q = q.Gt("status_changed_time", b[0].StatusChangedTime.UTC())
	}

	wg := newWorkGroup()

	perStatus := map[pb.Status]int64{}
	var perStatusM sync.Mutex
	for _, status := range statuses {
		wg.Go(func() error {
			count, err := datastore.Count(ctx, q.Eq("status_v2", status))
			if err != nil {
				return fmt.Errorf("counting %s: %w", status, err)
			}
			perStatusM.Lock()
			perStatus[status] = count
			perStatusM.Unlock()
			return nil
		})
	}

	base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT
	cms, cmNames := getBuilderDefinedMetricsPerBase(ctx, base, bldrDefinedMetrics)
	cmCounts := make(map[string]map[pb.Status]int64)
	var cmCountsM sync.Mutex
	for _, name := range cmNames {
		cmCounts[name] = make(map[pb.Status]int64)
	}
	for _, name := range cmNames {
		for _, status := range statuses {
			wg.Go(func() error {
				count, err := datastore.Count(ctx, q.Eq("status_v2", status).Eq("custom_builder_consecutive_failures_metrics", name))
				if err != nil {
					return fmt.Errorf("counting status==%s, metric=%q: %w", status, name, err)
				}
				cmCountsM.Lock()
				cmCounts[name][status] = count
				cmCountsM.Unlock()
				return nil
			})
		}
	}
	queryErr := wg.Wait()

	// These counts can be inaccurate a bit, but should be accurate enough.
	for status, count := range perStatus {
		V2.ConsecutiveFailureCount.Set(ctx, count, pb.Status_name[int32(status)])
	}

	// Report all custom metrics that we managed to fetch successfully.
	reportCustomBuilderCounts(ctx, cms, base, cmNames, statuses, cmCounts)
	return queryErr
}

// getBldrDefinedMetrics returns a map BuilderID -> a list of metric names.
func getBldrDefinedMetrics(ctx context.Context) (map[string][]string, error) {
	bldrmetrics := &model.CustomBuilderMetrics{Key: model.CustomBuilderMetricsKey(ctx)}
	err := datastore.Get(ctx, bldrmetrics)
	if err != nil && err != datastore.ErrNoSuchEntity {
		return nil, errors.Fmt("fetching bldrDefinedMetrics: %w", err)
	}

	bldrToMetrics := make(map[string][]string)
	for _, bm := range bldrmetrics.Metrics.GetMetrics() {
		for _, bldr := range bm.Builders {
			bldrStr := protoutil.FormatBuilderID(bldr)
			bldrToMetrics[bldrStr] = append(bldrToMetrics[bldrStr], bm.Name)
		}
	}
	return bldrToMetrics, nil
}

// getBuilderDefinedMetricsPerBase gets a list of custom metric names to query for the builder.
func getBuilderDefinedMetricsPerBase(ctx context.Context, base pb.CustomMetricBase, bldrDefinedMetrics []string) (*CustomMetrics, []string) {
	if len(bldrDefinedMetrics) == 0 {
		return nil, nil
	}

	cms := GetCustomMetrics(ctx)
	metrics := cms.getCustomMetricsByBase(base)
	if len(metrics) == 0 {
		return nil, nil
	}

	var cmNames []string
	for _, cm := range bldrDefinedMetrics {
		if _, ok := metrics[cm]; ok {
			cmNames = append(cmNames, cm)
		}
	}
	return cms, cmNames
}

func reportCustomBuilderCounts(ctx context.Context, cms *CustomMetrics, base pb.CustomMetricBase, cmNames []string, statuses []pb.Status, cmCounts map[string]map[pb.Status]int64) {
	for _, name := range cmNames {
		for _, status := range statuses {
			cms.Report(ctx, &Report{
				Base:     base,
				Name:     name,
				FieldMap: map[string]string{"status": pb.Status_name[int32(status)]},
				Value:    cmCounts[name][status],
			})
		}
	}
}

// workGroup runs a bunch of goroutines in parallel, collecting all errors.
type workGroup struct {
	eg    errgroup.Group
	errs  errors.MultiError
	errsM sync.Mutex
}

func newWorkGroup() *workGroup {
	wg := &workGroup{}
	wg.eg.SetLimit(200)
	return wg
}

func (wg *workGroup) Go(cb func() error) {
	wg.eg.Go(func() error {
		if err := cb(); err != nil {
			wg.errsM.Lock()
			wg.errs = append(wg.errs, err)
			wg.errsM.Unlock()
		}
		return nil
	})
}

func (wg *workGroup) Wait() error {
	_ = wg.eg.Wait()
	if len(wg.errs) == 0 {
		return nil
	}
	return wg.errs
}
