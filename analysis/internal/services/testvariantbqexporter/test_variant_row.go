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

package testvariantbqexporter

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal"
	"go.chromium.org/luci/analysis/internal/bqutil"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/pbutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func testVariantName(realm, testID, variantHash string) string {
	return fmt.Sprintf("realms/%s/tests/%s/variants/%s", realm, url.PathEscape(testID), variantHash)
}

func (b *BQExporter) populateQueryParameters() (inputs, params map[string]interface{}, err error) {
	inputs = map[string]interface{}{
		"TestIdFilter": b.options.Predicate.GetTestIdRegexp() != "",
		"StatusFilter": b.options.Predicate.GetStatus() != atvpb.Status_STATUS_UNSPECIFIED,
	}

	params = map[string]interface{}{
		"realm":              b.options.Realm,
		"flakyVerdictStatus": int(internal.VerdictStatus_VERDICT_FLAKY),
	}

	st, err := pbutil.AsTime(b.options.TimeRange.GetEarliest())
	if err != nil {
		return nil, nil, err
	}
	params["startTime"] = st

	et, err := pbutil.AsTime(b.options.TimeRange.GetLatest())
	if err != nil {
		return nil, nil, err
	}
	params["endTime"] = et

	if re := b.options.Predicate.GetTestIdRegexp(); re != "" && re != ".*" {
		params["testIdRegexp"] = fmt.Sprintf("^%s$", re)
	}

	if status := b.options.Predicate.GetStatus(); status != atvpb.Status_STATUS_UNSPECIFIED {
		params["status"] = int(status)
	}

	switch p := b.options.Predicate.GetVariant().GetPredicate().(type) {
	case *pb.VariantPredicate_Equals:
		inputs["VariantHashEquals"] = true
		params["variantHashEquals"] = pbutil.VariantHash(p.Equals)
	case *pb.VariantPredicate_HashEquals:
		inputs["VariantHashEquals"] = true
		params["variantHashEquals"] = p.HashEquals
	case *pb.VariantPredicate_Contains:
		if len(p.Contains.Def) > 0 {
			inputs["VariantContains"] = true
			params["variantContains"] = pbutil.VariantToStrings(p.Contains)
		}
	case nil:
		// No filter.
	default:
		return nil, nil, errors.Reason("unexpected variant predicate %q", p).Err()
	}
	return
}

type result struct {
	UnexpectedResultCount spanner.NullInt64
	TotalResultCount      spanner.NullInt64
	FlakyVerdictCount     spanner.NullInt64
	TotalVerdictCount     spanner.NullInt64
	Invocations           []string
}

type statusAndTimeRange struct {
	status atvpb.Status
	tr     *pb.TimeRange
}

// timeRanges splits the exported time range into the distinct time ranges to be
// exported, corresponding to the different statuses the test had during the
// exported time range. Only the time ranges which had a status meeting the
// criteria to be exported are returned
//
// It checks if it's needed to narrow/split the original time range
// based on the test variant's status update history.
// For example if the original time range is [10:00, 11:00) and a test variant's
// status had the following updates:
//   - 9:30: CONSISTENTLY_EXPECTED -> FLAKY
//   - 10:10: FLAKY -> CONSISTENTLY_EXPECTED
//   - 10:20: CONSISTENTLY_EXPECTED -> FLAKY
//   - 10:30: FLAKY -> CONSISTENTLY_EXPECTED
//   - 10:50: CONSISTENTLY_EXPECTED -> FLAKY
//
// If b.options.Predicate.Status = FLAKY, the timeRanges will be
// [10:00, 10:10), [10:20, 10:30) and [10:50, 11:00).
// If b.options.Predicate.Status is not specified, the timeRanges will be
// [10:00, 10:10), [10:10, 10:20),  [10:20, 10:30), [10:20, 10:50) and [10:50, 11:00).
func (b *BQExporter) timeRanges(currentStatus atvpb.Status, statusUpdateTime spanner.NullTime, previousStatuses []atvpb.Status, previousUpdateTimes []time.Time) []statusAndTimeRange {
	if !statusUpdateTime.Valid {
		panic("Empty Status Update time")
	}

	// The timestamps have been verified in populateQueryParameters.
	exportStart, _ := pbutil.AsTime(b.options.TimeRange.Earliest)
	exportEnd, _ := pbutil.AsTime(b.options.TimeRange.Latest)

	exportStatus := b.options.Predicate.GetStatus()
	previousStatuses = append([]atvpb.Status{currentStatus}, previousStatuses...)
	previousUpdateTimes = append([]time.Time{statusUpdateTime.Time}, previousUpdateTimes...)

	var ranges []statusAndTimeRange
	rangeEnd := exportEnd
	// Iterate through status updates, from latest to earliest.
	for i, updateTime := range previousUpdateTimes {
		if updateTime.After(exportEnd) {
			continue
		}

		s := previousStatuses[i]
		shouldExport := s == exportStatus || exportStatus == atvpb.Status_STATUS_UNSPECIFIED
		if shouldExport {
			if updateTime.After(exportStart) {
				ranges = append(ranges, statusAndTimeRange{status: s, tr: &pb.TimeRange{
					Earliest: pbutil.MustTimestampProto(updateTime),
					Latest:   pbutil.MustTimestampProto(rangeEnd),
				}})
			} else {
				ranges = append(ranges, statusAndTimeRange{status: s, tr: &pb.TimeRange{
					Earliest: b.options.TimeRange.Earliest,
					Latest:   pbutil.MustTimestampProto(rangeEnd),
				}})
			}
		}
		rangeEnd = updateTime

		if !updateTime.After(exportStart) {
			break
		}
	}
	return ranges
}

type verdictInfo struct {
	verdict               *bqpb.Verdict
	ingestionTime         time.Time
	unexpectedResultCount int
	totalResultCount      int
}

// convertVerdicts converts strings to verdictInfos.
// Ordered by IngestionTime.
func (b *BQExporter) convertVerdicts(vs []string) ([]verdictInfo, error) {
	vis := make([]verdictInfo, 0, len(vs))
	for _, v := range vs {
		parts := strings.Split(v, "/")
		if len(parts) != 6 {
			return nil, fmt.Errorf("verdict %s in wrong format", v)
		}
		verdict := &bqpb.Verdict{
			Invocation: parts[0],
		}
		s, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		verdict.Status = internal.VerdictStatus(s).String()

		ct, err := time.Parse(time.RFC3339Nano, parts[2])
		if err != nil {
			return nil, err
		}
		verdict.CreateTime = timestamppb.New(ct)

		it, err := time.Parse(time.RFC3339Nano, parts[3])
		if err != nil {
			return nil, err
		}

		unexpectedResultCount, err := strconv.Atoi(parts[4])
		if err != nil {
			return nil, err
		}

		totalResultCount, err := strconv.Atoi(parts[5])
		if err != nil {
			return nil, err
		}

		vis = append(vis, verdictInfo{
			verdict:               verdict,
			ingestionTime:         it,
			unexpectedResultCount: unexpectedResultCount,
			totalResultCount:      totalResultCount,
		})
	}

	sort.Slice(vis, func(i, j int) bool { return vis[i].ingestionTime.Before(vis[j].ingestionTime) })

	return vis, nil
}

func (b *BQExporter) populateVerdictsInRange(tv *bqpb.TestVariantRow, vs []verdictInfo, tr *pb.TimeRange) {
	earliest, _ := pbutil.AsTime(tr.Earliest)
	latest, _ := pbutil.AsTime(tr.Latest)
	var vsInRange []*bqpb.Verdict
	for _, v := range vs {
		if (v.ingestionTime.After(earliest) || v.ingestionTime.Equal(earliest)) && v.ingestionTime.Before(latest) {
			vsInRange = append(vsInRange, v.verdict)
		}
	}
	tv.Verdicts = vsInRange
}

func zeroFlakyStatistics() *atvpb.FlakeStatistics {
	return &atvpb.FlakeStatistics{
		FlakyVerdictCount:     0,
		TotalVerdictCount:     0,
		FlakyVerdictRate:      float32(0),
		UnexpectedResultCount: 0,
		TotalResultCount:      0,
		UnexpectedResultRate:  float32(0),
	}
}

func (b *BQExporter) populateFlakeStatistics(tv *bqpb.TestVariantRow, res *result, vs []verdictInfo, tr *pb.TimeRange) {
	if b.options.TimeRange.Earliest != tr.Earliest || b.options.TimeRange.Latest != tr.Latest {
		// The time range is different from the original one, so we cannot use the
		// statistics from query, instead we need to calculate using data from each verdicts.
		b.populateFlakeStatisticsByVerdicts(tv, vs, tr)
		return
	}
	zero64 := int64(0)
	if res.TotalResultCount.Valid && res.TotalResultCount.Int64 == zero64 {
		tv.FlakeStatistics = zeroFlakyStatistics()
		return
	}
	tv.FlakeStatistics = &atvpb.FlakeStatistics{
		FlakyVerdictCount:     res.FlakyVerdictCount.Int64,
		TotalVerdictCount:     res.TotalVerdictCount.Int64,
		FlakyVerdictRate:      float32(res.FlakyVerdictCount.Int64) / float32(res.TotalVerdictCount.Int64),
		UnexpectedResultCount: res.UnexpectedResultCount.Int64,
		TotalResultCount:      res.TotalResultCount.Int64,
		UnexpectedResultRate:  float32(res.UnexpectedResultCount.Int64) / float32(res.TotalResultCount.Int64),
	}
}

func (b *BQExporter) populateFlakeStatisticsByVerdicts(tv *bqpb.TestVariantRow, vs []verdictInfo, tr *pb.TimeRange) {
	if len(vs) == 0 {
		tv.FlakeStatistics = zeroFlakyStatistics()
		return
	}

	earliest, _ := pbutil.AsTime(tr.Earliest)
	latest, _ := pbutil.AsTime(tr.Latest)
	flakyVerdicts := 0
	totalVerdicts := 0
	unexpectedResults := 0
	totalResults := 0
	for _, v := range vs {
		if (v.ingestionTime.After(earliest) || v.ingestionTime.Equal(earliest)) && v.ingestionTime.Before(latest) {
			totalVerdicts++
			unexpectedResults += v.unexpectedResultCount
			totalResults += v.totalResultCount
			if v.verdict.Status == internal.VerdictStatus_VERDICT_FLAKY.String() {
				flakyVerdicts++
			}
		}
	}

	if totalResults == 0 {
		tv.FlakeStatistics = zeroFlakyStatistics()
		return
	}

	tv.FlakeStatistics = &atvpb.FlakeStatistics{
		FlakyVerdictCount:     int64(flakyVerdicts),
		TotalVerdictCount:     int64(totalVerdicts),
		FlakyVerdictRate:      float32(flakyVerdicts) / float32(totalVerdicts),
		UnexpectedResultCount: int64(unexpectedResults),
		TotalResultCount:      int64(totalResults),
		UnexpectedResultRate:  float32(unexpectedResults) / float32(totalResults),
	}
}

func deepCopy(tv *bqpb.TestVariantRow) *bqpb.TestVariantRow {
	return &bqpb.TestVariantRow{
		Name:         tv.Name,
		Realm:        tv.Realm,
		TestId:       tv.TestId,
		VariantHash:  tv.VariantHash,
		Variant:      tv.Variant,
		TestMetadata: tv.TestMetadata,
		Tags:         tv.Tags,
	}
}

// generateTestVariantRows converts a bq.Row to *bqpb.TestVariantRows.
//
// For the most cases it should return one row. But if the test variant
// changes status during the default time range, it may need to export 2 rows
// for the previous and current statuses with smaller time ranges.
func (b *BQExporter) generateTestVariantRows(ctx context.Context, row *spanner.Row, bf spanutil.Buffer) ([]*bqpb.TestVariantRow, error) {
	tv := &bqpb.TestVariantRow{}
	va := &pb.Variant{}
	var vs []*result
	var statusUpdateTime spanner.NullTime
	var tmd spanutil.Compressed
	var status atvpb.Status
	var previousStatuses []atvpb.Status
	var previousUpdateTimes []time.Time
	if err := bf.FromSpanner(
		row,
		&tv.Realm,
		&tv.TestId,
		&tv.VariantHash,
		&va,
		&tv.Tags,
		&tmd,
		&status,
		&statusUpdateTime,
		&previousStatuses,
		&previousUpdateTimes,
		&vs,
	); err != nil {
		return nil, err
	}

	tv.Name = testVariantName(tv.Realm, tv.TestId, tv.VariantHash)
	if len(vs) != 1 {
		return nil, fmt.Errorf("fail to get verdicts for test variant %s", tv.Name)
	}

	tv.Variant = pbutil.VariantToStringPairs(va)
	tv.Status = status.String()

	if len(tmd) > 0 {
		tv.TestMetadata = &pb.TestMetadata{}
		if err := proto.Unmarshal(tmd, tv.TestMetadata); err != nil {
			return nil, errors.Annotate(err, "error unmarshalling test_metadata for %s", tv.Name).Err()
		}
	}

	timeRanges := b.timeRanges(status, statusUpdateTime, previousStatuses, previousUpdateTimes)
	verdicts, err := b.convertVerdicts(vs[0].Invocations)
	if err != nil {
		return nil, err
	}

	var tvs []*bqpb.TestVariantRow
	for _, str := range timeRanges {
		newTV := deepCopy(tv)
		newTV.TimeRange = str.tr
		newTV.PartitionTime = str.tr.Latest
		newTV.Status = str.status.String()
		b.populateFlakeStatistics(newTV, vs[0], verdicts, str.tr)
		b.populateVerdictsInRange(newTV, verdicts, str.tr)
		tvs = append(tvs, newTV)
	}

	return tvs, nil
}

func (b *BQExporter) query(ctx context.Context, f func(*bqpb.TestVariantRow) error) error {
	inputs, params, err := b.populateQueryParameters()
	if err != nil {
		return err
	}
	st, err := spanutil.GenerateStatement(testVariantRowsTmpl, testVariantRowsTmpl.Name(), inputs)
	if err != nil {
		return err
	}
	st.Params = params

	var bf spanutil.Buffer
	return span.Query(ctx, st).Do(
		func(row *spanner.Row) error {
			tvrs, err := b.generateTestVariantRows(ctx, row, bf)
			if err != nil {
				return err
			}
			for _, tvr := range tvrs {
				if err := f(tvr); err != nil {
					return err
				}
			}
			return nil
		},
	)
}

func (b *BQExporter) queryTestVariantsToExport(ctx context.Context, batchC chan []*bqpb.TestVariantRow) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	tvrs := make([]*bqpb.TestVariantRow, 0, maxBatchRowCount)
	rowCount := 0
	err := b.query(ctx, func(tvr *bqpb.TestVariantRow) error {
		tvrs = append(tvrs, tvr)
		rowCount++
		if len(tvrs) >= maxBatchRowCount {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- tvrs:
			}
			tvrs = make([]*bqpb.TestVariantRow, 0, maxBatchRowCount)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(tvrs) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchC <- tvrs:
		}
	}

	logging.Infof(ctx, "fetched %d rows for exporting %s test variants", rowCount, b.options.Realm)
	return nil
}

// inserter is implemented by bigquery.Inserter.
type inserter interface {
	// PutWithRetries uploads one or more rows to the BigQuery service.
	PutWithRetries(ctx context.Context, src []*bq.Row) error
}

func (b *BQExporter) batchExportRows(ctx context.Context, ins inserter, batchC chan []*bqpb.TestVariantRow) error {
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	for rows := range batchC {
		rows := rows
		if err := b.batchSem.Acquire(ctx, 1); err != nil {
			return err
		}

		eg.Go(func() error {
			defer b.batchSem.Release(1)
			err := b.insertRows(ctx, ins, rows)
			if bqutil.FatalError(err) {
				err = tq.Fatal.Apply(err)
			}
			return err
		})
	}

	return eg.Wait()
}

// insertRows inserts rows into BigQuery.
// Retries on transient errors.
func (b *BQExporter) insertRows(ctx context.Context, ins inserter, rowProtos []*bqpb.TestVariantRow) error {
	if err := b.putLimiter.Wait(ctx); err != nil {
		return err
	}

	rows := make([]*bq.Row, 0, len(rowProtos))
	for _, ri := range rowProtos {
		row := &bq.Row{
			Message:  ri,
			InsertID: bigquery.NoDedupeID,
		}
		rows = append(rows, row)
	}

	return ins.PutWithRetries(ctx, rows)
}

func (b *BQExporter) exportTestVariantRows(ctx context.Context, ins inserter) error {
	batchC := make(chan []*bqpb.TestVariantRow)
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return b.batchExportRows(ctx, ins, batchC)
	})

	eg.Go(func() error {
		defer close(batchC)
		return b.queryTestVariantsToExport(ctx, batchC)
	})

	return eg.Wait()
}

var testVariantRowsTmpl = template.Must(template.New("testVariantRowsTmpl").Parse(`
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH test_variants AS (
		SELECT
			Realm,
			TestId,
			VariantHash,
		FROM AnalyzedTestVariants
		WHERE Realm = @realm
		{{/* Filter by TestId */}}
		{{if .TestIdFilter}}
			AND REGEXP_CONTAINS(TestId, @testIdRegexp)
		{{end}}
		{{/* Filter by Variant */}}
		{{if .VariantHashEquals}}
			AND VariantHash = @variantHashEquals
		{{end}}
		{{if .VariantContains }}
			AND (SELECT LOGICAL_AND(kv IN UNNEST(Variant)) FROM UNNEST(@variantContains) kv)
		{{end}}
		{{/* Filter by status */}}
		{{if .StatusFilter}}
			AND (
				(Status = @status AND StatusUpdateTime < @endTime)
				-- Status updated within the time range, we need to check if the previous
				-- status(es) satisfies the filter.
				OR StatusUpdateTime > @startTime
			)
		{{end}}
	)

	SELECT
		Realm,
		TestId,
		VariantHash,
		Variant,
		Tags,
		TestMetadata,
		Status,
		StatusUpdateTime,
		PreviousStatuses,
		PreviousStatusUpdateTimes,
		ARRAY(
		SELECT
			AS STRUCT SUM(UnexpectedResultCount) UnexpectedResultCount,
			SUM(TotalResultCount) TotalResultCount,
			COUNTIF(Status=30) FlakyVerdictCount,
			COUNT(*) TotalVerdictCount,
			-- Using struct here will trigger the "null-valued array of struct" query shape
			-- which is not supported by Spanner.
			-- Use a string to work around it.
			ARRAY_AGG(FORMAT('%s/%d/%s/%s/%d/%d', InvocationId, Status, FORMAT_TIMESTAMP("%FT%H:%M:%E*S%Ez", InvocationCreationTime), FORMAT_TIMESTAMP("%FT%H:%M:%E*S%Ez", IngestionTime), UnexpectedResultCount, TotalResultCount)) Invocations
		FROM
			Verdicts
		WHERE
			Verdicts.Realm = test_variants.Realm
			AND Verdicts.TestId=test_variants.TestId
			AND Verdicts.VariantHash=test_variants.VariantHash
			AND IngestionTime >= @startTime
			AND IngestionTime < @endTime ) Results
	FROM
		test_variants
	JOIN
		AnalyzedTestVariants
	USING
		(Realm,
			TestId,
			VariantHash)
`))
