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

// Package testresults contains methods for accessing test results in Spanner.
package testresults

import (
	"context"
	"sort"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/pagination"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const pageTokenTimeFormat = time.RFC3339Nano

// The suffix used for all gerrit hostnames.
const GerritHostnameSuffix = "-review.googlesource.com"

var (
	// minTimestamp is the minimum Timestamp value in Spanner.
	// https://cloud.google.com/spanner/docs/reference/standard-sql/data-types#timestamp_type
	MinSpannerTimestamp = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	// maxSpannerTimestamp is the max Timestamp value in Spanner.
	// https://cloud.google.com/spanner/docs/reference/standard-sql/data-types#timestamp_type
	MaxSpannerTimestamp = time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC)
)

// Changelist represents a gerrit changelist.
type Changelist struct {
	// Host is the gerrit hostname. E.g. chromium-review.googlesource.com.
	Host      string
	Change    int64
	Patchset  int64
	OwnerKind pb.ChangelistOwnerKind
}

// SortChangelists sorts a slice of changelists to be in ascending
// lexicographical order by (host, change, patchset).
func SortChangelists(cls []Changelist) {
	sort.Slice(cls, func(i, j int) bool {
		// Returns true iff cls[i] is less than cls[j].
		if cls[i].Host < cls[j].Host {
			return true
		}
		if cls[i].Host == cls[j].Host && cls[i].Change < cls[j].Change {
			return true
		}
		if cls[i].Host == cls[j].Host && cls[i].Change == cls[j].Change && cls[i].Patchset < cls[j].Patchset {
			return true
		}
		return false
	})
}

// Sources captures information about the code sources that were
// tested by a test result.
type Sources struct {
	// 8-byte hash of the source reference (e.g. git branch) tested.
	// This refers to the base commit/version tested, before any changelists
	// are applied.
	RefHash []byte
	// The position along the source reference that was tested.
	// This refers to the base commit/version tested, before any changelists
	// are applied.
	Position int64
	// The gerrit changelists applied on top of the base version/commit.
	// At most 10 changelists should be specified here, if there are more
	// then limit to 10 and set HasDirtySources to true.
	Changelists []Changelist
	// Whether other modifications were made to the sources, not described
	// by the fields above. For example, a package was upreved in the build.
	// If this is set, then the source information is approximate: suitable
	// for plotting results by source position the UI but not good enough
	// for change point analysis.
	IsDirty bool
}

// TestResult represents a row in the TestResults table.
type TestResult struct {
	Project              string
	TestID               string
	PartitionTime        time.Time
	VariantHash          string
	IngestedInvocationID string
	RunIndex             int64
	ResultIndex          int64
	IsUnexpected         bool
	RunDuration          *time.Duration
	Status               pb.TestResultStatus
	StatusV2             pb.TestResult_Status
	// Properties of the test verdict (stored denormalised) follow.
	ExonerationReasons []pb.ExonerationReason
	Sources            Sources
	// Properties of the invocation (stored denormalised) follow.
	SubRealm        string
	IsFromBisection bool
}

// ReadTestResults reads test results from the TestResults table.
// Must be called in a spanner transactional context.
func ReadTestResults(ctx context.Context, keys spanner.KeySet, fn func(tr *TestResult) error) error {
	var b spanutil.Buffer
	fields := []string{
		"Project", "TestId", "PartitionTime", "VariantHash", "IngestedInvocationId",
		"RunIndex", "ResultIndex",
		"IsUnexpected", "RunDurationUsec", "Status", "StatusV2",
		"ExonerationReasons",
		"SourceRefHash", "SourcePosition",
		"ChangelistHosts", "ChangelistChanges", "ChangelistPatchsets", "ChangelistOwnerKinds",
		"HasDirtySources",
		"SubRealm",
		"IsFromBisection",
	}
	return span.Read(ctx, "TestResults", keys, fields).Do(
		func(row *spanner.Row) error {
			tr := &TestResult{}
			var runDurationUsec spanner.NullInt64
			var isUnexpected spanner.NullBool
			var statusV2 spanner.NullInt64
			var sourceRefHash []byte
			var sourcePosition spanner.NullInt64
			var changelistHosts []string
			var changelistChanges []int64
			var changelistPatchsets []int64
			var changelistOwnerKinds []string
			var hasDirtySources spanner.NullBool
			var isFromBisection spanner.NullBool
			err := b.FromSpanner(
				row,
				&tr.Project, &tr.TestID, &tr.PartitionTime, &tr.VariantHash, &tr.IngestedInvocationID,
				&tr.RunIndex, &tr.ResultIndex,
				&isUnexpected, &runDurationUsec, &tr.Status, &statusV2,
				&tr.ExonerationReasons,
				&sourceRefHash, &sourcePosition,
				&changelistHosts, &changelistChanges, &changelistPatchsets, &changelistOwnerKinds,
				&hasDirtySources,
				&tr.SubRealm,
				&isFromBisection,
			)
			if err != nil {
				return err
			}
			if runDurationUsec.Valid {
				runDuration := time.Microsecond * time.Duration(runDurationUsec.Int64)
				tr.RunDuration = &runDuration
			}
			if statusV2.Valid {
				tr.StatusV2 = pb.TestResult_Status(statusV2.Int64)
			}
			tr.IsUnexpected = isUnexpected.Valid && isUnexpected.Bool
			tr.IsFromBisection = isFromBisection.Valid && isFromBisection.Bool

			// Data in Spanner should be consistent, so commitPosition.Valid ==
			// (gitReferenceHash != nil).
			if sourcePosition.Valid {
				tr.Sources.RefHash = sourceRefHash
				tr.Sources.Position = sourcePosition.Int64
			}

			// Data in spanner should be consistent, so
			// len(changelistHosts) == len(changelistChanges)
			//    == len(changelistPatchsets).
			//
			// ChangeListOwnerKinds was retrofitted after the table
			// was first created, so it should be of equal length
			// only if present. It was introduced in November 2022,
			// so this special-case can be deleted in March 2023+.
			if len(changelistHosts) != len(changelistChanges) ||
				len(changelistChanges) != len(changelistPatchsets) ||
				(changelistOwnerKinds != nil && len(changelistOwnerKinds) != len(changelistPatchsets)) {
				panic("Changelist arrays have mismatched length in Spanner")
			}
			changelists := make([]Changelist, 0, len(changelistHosts))
			for i := range changelistHosts {
				var ownerKind pb.ChangelistOwnerKind
				if changelistOwnerKinds != nil {
					ownerKind = OwnerKindFromDB(changelistOwnerKinds[i])
				}
				changelists = append(changelists, Changelist{
					Host:      DecompressHost(changelistHosts[i]),
					Change:    changelistChanges[i],
					Patchset:  changelistPatchsets[i],
					OwnerKind: ownerKind,
				})
			}
			tr.Sources.Changelists = changelists
			tr.Sources.IsDirty = hasDirtySources.Valid && hasDirtySources.Bool

			return fn(tr)
		})
}

// TestResultSaveCols is the set of columns written to in a test result save.
// Allocated here once to avoid reallocating on every test result save.
var TestResultSaveCols = []string{
	"Project", "TestId", "PartitionTime", "VariantHash",
	"IngestedInvocationId", "RunIndex", "ResultIndex",
	"IsUnexpected", "RunDurationUsec", "Status", "StatusV2",
	"ExonerationReasons",
	"SourceRefHash", "SourcePosition",
	"ChangelistHosts", "ChangelistChanges", "ChangelistPatchsets",
	"ChangelistOwnerKinds",
	"HasDirtySources",
	"SubRealm", "IsFromBisection",
}

// SaveUnverified prepare a mutation to insert the test result into the
// TestResults table. The test result is not validated.
func (tr *TestResult) SaveUnverified() *spanner.Mutation {
	var runDurationUsec spanner.NullInt64
	if tr.RunDuration != nil {
		runDurationUsec.Int64 = tr.RunDuration.Microseconds()
		runDurationUsec.Valid = true
	}

	var sourceRefHash []byte
	var sourcePosition spanner.NullInt64
	if tr.Sources.Position > 0 && len(tr.Sources.RefHash) > 0 {
		sourceRefHash = tr.Sources.RefHash
		sourcePosition = spanner.NullInt64{Valid: true, Int64: tr.Sources.Position}
	}

	changelistHosts := make([]string, 0, len(tr.Sources.Changelists))
	changelistChanges := make([]int64, 0, len(tr.Sources.Changelists))
	changelistPatchsets := make([]int64, 0, len(tr.Sources.Changelists))
	changelistOwnerKinds := make([]string, 0, len(tr.Sources.Changelists))
	for _, cl := range tr.Sources.Changelists {
		changelistHosts = append(changelistHosts, CompressHost(cl.Host))
		changelistChanges = append(changelistChanges, cl.Change)
		changelistPatchsets = append(changelistPatchsets, int64(cl.Patchset))
		changelistOwnerKinds = append(changelistOwnerKinds, OwnerKindToDB(cl.OwnerKind))
	}

	hasDirtySources := spanner.NullBool{Bool: tr.Sources.IsDirty, Valid: tr.Sources.IsDirty}

	isUnexpected := spanner.NullBool{Bool: tr.IsUnexpected, Valid: tr.IsUnexpected}
	isFromBisection := spanner.NullBool{Bool: tr.IsFromBisection, Valid: tr.IsFromBisection}

	exonerationReasons := tr.ExonerationReasons
	if len(exonerationReasons) == 0 {
		// Store absence of exonerations as a NULL value in the database
		// rather than an empty array. Backfilling the column is too
		// time consuming and NULLs use slightly less storage space.
		exonerationReasons = nil
	}

	// Specify values in a slice directly instead of
	// creating a map and using spanner.InsertOrUpdateMap.
	// Profiling revealed ~15% of all CPU cycles spent
	// ingesting test results were wasted generating a
	// map and converting it back to the slice
	// needed for a *spanner.Mutation using InsertOrUpdateMap.
	// Ingestion appears to be CPU bound at times.
	vals := []any{
		tr.Project, tr.TestID, tr.PartitionTime, tr.VariantHash,
		tr.IngestedInvocationID, tr.RunIndex, tr.ResultIndex,
		isUnexpected, runDurationUsec, int64(tr.Status), int64(tr.StatusV2),
		spanutil.ToSpanner(exonerationReasons),
		sourceRefHash, sourcePosition,
		changelistHosts, changelistChanges, changelistPatchsets, changelistOwnerKinds,
		hasDirtySources,
		tr.SubRealm, isFromBisection,
	}
	return spanner.InsertOrUpdate("TestResults", TestResultSaveCols, vals)
}

// ReadTestHistoryOptions specifies options for ReadTestHistory().
type ReadTestHistoryOptions struct {
	Project                 string
	TestID                  string
	SubRealms               []string
	VariantPredicate        *pb.VariantPredicate
	SubmittedFilter         pb.SubmittedFilter
	TimeRange               *pb.TimeRange
	ExcludeBisectionResults bool
	PageSize                int
	PageToken               string
}

// statement generates a spanner statement for the specified query template.
func (opts ReadTestHistoryOptions) statement(ctx context.Context, tmpl string, paginationParams []string) (spanner.Statement, error) {
	params := map[string]any{
		"project":   opts.Project,
		"testId":    opts.TestID,
		"subRealms": opts.SubRealms,
		"limit":     opts.PageSize,

		// If the filter is unspecified, this param will be ignored during the
		// statement generation step.
		"hasUnsubmittedChanges": opts.SubmittedFilter == pb.SubmittedFilter_ONLY_UNSUBMITTED,

		// Verdict status v1 enum values.
		"unexpected":          int(pb.TestVerdictStatus_UNEXPECTED),
		"unexpectedlySkipped": int(pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED),
		"flaky":               int(pb.TestVerdictStatus_FLAKY),
		"exonerated":          int(pb.TestVerdictStatus_EXONERATED),
		"expected":            int(pb.TestVerdictStatus_EXPECTED),

		// Test result status v1 enum values.
		"skip": int(pb.TestResultStatus_SKIP),
		"pass": int(pb.TestResultStatus_PASS),

		// Verdict status v2 enum values.
		"passedVerdict":           int(pb.TestVerdict_PASSED),
		"flakyVerdict":            int(pb.TestVerdict_FLAKY),
		"failedVerdict":           int(pb.TestVerdict_FAILED),
		"skippedVerdict":          int(pb.TestVerdict_SKIPPED),
		"executionErroredVerdict": int(pb.TestVerdict_EXECUTION_ERRORED),
		"precludedVerdict":        int(pb.TestVerdict_PRECLUDED),

		// Test result status v2 enum values.
		"passedV2":           int(pb.TestResult_PASSED),
		"failedV2":           int(pb.TestResult_FAILED),
		"skippedV2":          int(pb.TestResult_SKIPPED),
		"executionErroredV2": int(pb.TestResult_EXECUTION_ERRORED),
	}
	input := map[string]any{
		"hasLimit":                opts.PageSize > 0,
		"hasSubmittedFilter":      opts.SubmittedFilter != pb.SubmittedFilter_SUBMITTED_FILTER_UNSPECIFIED,
		"excludeBisectionResults": opts.ExcludeBisectionResults,
		"pagination":              opts.PageToken != "",
		"params":                  params,
	}

	if opts.TimeRange.GetEarliest() != nil {
		params["afterTime"] = opts.TimeRange.GetEarliest().AsTime()
	} else {
		params["afterTime"] = MinSpannerTimestamp
	}
	if opts.TimeRange.GetLatest() != nil {
		params["beforeTime"] = opts.TimeRange.GetLatest().AsTime()
	} else {
		params["beforeTime"] = MaxSpannerTimestamp
	}

	switch p := opts.VariantPredicate.GetPredicate().(type) {
	case *pb.VariantPredicate_Equals:
		input["hasVariantHash"] = true
		params["variantHash"] = pbutil.VariantHash(p.Equals)
	case *pb.VariantPredicate_Contains:
		if len(p.Contains.Def) > 0 {
			input["hasVariantKVs"] = true
			params["variantKVs"] = pbutil.VariantToStrings(p.Contains)
		}
	case *pb.VariantPredicate_HashEquals:
		input["hasVariantHash"] = true
		params["variantHash"] = p.HashEquals
	case nil:
		// No filter.
	default:
		panic(errors.Reason("unexpected variant predicate %q", opts.VariantPredicate).Err())
	}

	if opts.PageToken != "" {
		tokens, err := pagination.ParseToken(opts.PageToken)
		if err != nil {
			return spanner.Statement{}, err
		}

		if len(tokens) != len(paginationParams) {
			return spanner.Statement{}, pagination.InvalidToken(errors.Reason("expected %d components, got %d", len(paginationParams), len(tokens)).Err())
		}

		// Keep all pagination params as strings and convert them to other data
		// types in the query as necessary. So we can have a unified way of handling
		// different page tokens.
		for i, param := range paginationParams {
			params[param] = tokens[i]
		}
	}

	stmt, err := spanutil.GenerateStatement(testHistoryQueryTmpl, tmpl, input)
	if err != nil {
		return spanner.Statement{}, err
	}
	stmt.Params = params

	return stmt, nil
}

// ReadTestHistory reads verdicts from the spanner database.
// Must be called in a spanner transactional context.
func ReadTestHistory(ctx context.Context, opts ReadTestHistoryOptions) (verdicts []*pb.TestVerdict, nextPageToken string, err error) {
	stmt, err := opts.statement(ctx, "testHistoryQuery", []string{"paginationTime", "paginationVariantHash", "paginationInvId"})
	if err != nil {
		return nil, "", err
	}

	var b spanutil.Buffer
	verdicts = make([]*pb.TestVerdict, 0, opts.PageSize)
	err = span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		tv := &pb.TestVerdict{
			TestId: opts.TestID,
		}
		var status int64
		var passedAvgDurationUsec spanner.NullInt64
		var changelistHosts []string
		var changelistChanges []int64
		var changelistPatchsets []int64
		var changelistOwnerKinds []string
		err := b.FromSpanner(
			row,
			&tv.PartitionTime,
			&tv.VariantHash,
			&tv.InvocationId,
			&status,
			&passedAvgDurationUsec,
			&changelistHosts,
			&changelistChanges,
			&changelistPatchsets,
			&changelistOwnerKinds,
		)
		if err != nil {
			return err
		}
		tv.Status = pb.TestVerdictStatus(status)
		if passedAvgDurationUsec.Valid {
			tv.PassedAvgDuration = durationpb.New(time.Microsecond * time.Duration(passedAvgDurationUsec.Int64))
		}

		// Data in spanner should be consistent, so
		// len(changelistHosts) == len(changelistChanges)
		//    == len(changelistPatchsets).
		//
		// ChangeListOwnerKinds was retrofitted after the table
		// was first created, so it should be of equal length
		// only if present. It was introduced in November 2022,
		// so this special-case can be deleted in March 2023+.
		if len(changelistHosts) != len(changelistChanges) ||
			len(changelistChanges) != len(changelistPatchsets) ||
			(changelistOwnerKinds != nil && len(changelistOwnerKinds) != len(changelistPatchsets)) {
			panic("Changelist arrays have mismatched length in Spanner")
		}
		changelists := make([]*pb.Changelist, 0, len(changelistHosts))
		for i := range changelistHosts {
			var ownerKind pb.ChangelistOwnerKind
			if changelistOwnerKinds != nil {
				ownerKind = OwnerKindFromDB(changelistOwnerKinds[i])
			}
			changelists = append(changelists, &pb.Changelist{
				Host:      DecompressHost(changelistHosts[i]),
				Change:    changelistChanges[i],
				Patchset:  int32(changelistPatchsets[i]),
				OwnerKind: ownerKind,
			})
		}
		tv.Changelists = changelists

		verdicts = append(verdicts, tv)
		return nil
	})
	if err != nil {
		return nil, "", errors.Annotate(err, "query test history").Err()
	}

	if opts.PageSize != 0 && len(verdicts) == opts.PageSize {
		lastTV := verdicts[len(verdicts)-1]
		nextPageToken = pagination.Token(lastTV.PartitionTime.AsTime().Format(pageTokenTimeFormat), lastTV.VariantHash, lastTV.InvocationId)
	}
	return verdicts, nextPageToken, nil
}

type verdictCountsV2 struct {
	Failed                     int64
	Flaky                      int64
	Passed                     int64
	Skipped                    int64
	ExecutionErrored           int64
	Precluded                  int64
	FailedExonerated           int64
	ExecutionErroredExonerated int64
	PrecludedExonerated        int64
}

type verdictCountsV1 struct {
	Unexpected          int64
	UnexpectedlySkipped int64
	Flaky               int64
	Exonerated          int64
	Expected            int64
}

// ReadTestHistoryStats reads stats of verdicts grouped by UTC dates from the
// spanner database.
// Must be called in a spanner transactional context.
func ReadTestHistoryStats(ctx context.Context, opts ReadTestHistoryOptions) (groups []*pb.QueryTestHistoryStatsResponse_Group, nextPageToken string, err error) {
	stmt, err := opts.statement(ctx, "testHistoryStatsQuery", []string{"paginationDate", "paginationVariantHash"})
	if err != nil {
		return nil, "", err
	}

	var b spanutil.Buffer
	groups = make([]*pb.QueryTestHistoryStatsResponse_Group, 0, opts.PageSize)
	err = span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		group := &pb.QueryTestHistoryStatsResponse_Group{}
		var (
			verdictCountsV2       verdictCountsV2
			verdictCountsV1       verdictCountsV1
			passedAvgDurationUsec spanner.NullInt64
		)
		err := b.FromSpanner(
			row,
			&group.PartitionTime,
			&group.VariantHash,
			&verdictCountsV2.Failed,
			&verdictCountsV2.Flaky,
			&verdictCountsV2.Passed,
			&verdictCountsV2.Skipped,
			&verdictCountsV2.ExecutionErrored,
			&verdictCountsV2.Precluded,
			&verdictCountsV2.FailedExonerated,
			&verdictCountsV2.ExecutionErroredExonerated,
			&verdictCountsV2.PrecludedExonerated,
			&verdictCountsV1.Unexpected,
			&verdictCountsV1.UnexpectedlySkipped,
			&verdictCountsV1.Flaky,
			&verdictCountsV1.Exonerated,
			&verdictCountsV1.Expected,
			&passedAvgDurationUsec,
		)
		if err != nil {
			return err
		}

		group.VerdictCounts = &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
			Failed:                     int32(verdictCountsV2.Failed),
			Flaky:                      int32(verdictCountsV2.Flaky),
			Passed:                     int32(verdictCountsV2.Passed),
			Skipped:                    int32(verdictCountsV2.Skipped),
			ExecutionErrored:           int32(verdictCountsV2.ExecutionErrored),
			Precluded:                  int32(verdictCountsV2.Precluded),
			FailedExonerated:           int32(verdictCountsV2.FailedExonerated),
			ExecutionErroredExonerated: int32(verdictCountsV2.ExecutionErroredExonerated),
			PrecludedExonerated:        int32(verdictCountsV2.PrecludedExonerated),
		}
		group.UnexpectedCount = int32(verdictCountsV1.Unexpected)
		group.UnexpectedlySkippedCount = int32(verdictCountsV1.UnexpectedlySkipped)
		group.FlakyCount = int32(verdictCountsV1.Flaky)
		group.ExoneratedCount = int32(verdictCountsV1.Exonerated)
		group.ExpectedCount = int32(verdictCountsV1.Expected)
		if passedAvgDurationUsec.Valid {
			group.PassedAvgDuration = durationpb.New(time.Microsecond * time.Duration(passedAvgDurationUsec.Int64))
		}
		groups = append(groups, group)
		return nil
	})
	if err != nil {
		return nil, "", errors.Annotate(err, "query test history stats").Err()
	}

	if opts.PageSize != 0 && len(groups) == opts.PageSize {
		lastGroup := groups[len(groups)-1]
		nextPageToken = pagination.Token(lastGroup.PartitionTime.AsTime().Format(pageTokenTimeFormat), lastGroup.VariantHash)
	}
	return groups, nextPageToken, nil
}

// TestVariantRealm represents a row in the TestVariantRealm table.
type TestVariantRealm struct {
	Project           string
	TestID            string
	VariantHash       string
	SubRealm          string
	Variant           *pb.Variant
	LastIngestionTime time.Time
}

// ReadTestVariantRealms read test variant realms from the TestVariantRealms
// table.
// Must be called in a spanner transactional context.
func ReadTestVariantRealms(ctx context.Context, keys spanner.KeySet, fn func(tvr *TestVariantRealm) error) error {
	var b spanutil.Buffer
	fields := []string{"Project", "TestId", "VariantHash", "SubRealm", "Variant", "LastIngestionTime"}
	return span.Read(ctx, "TestVariantRealms", keys, fields).Do(
		func(row *spanner.Row) error {
			tvr := &TestVariantRealm{}
			err := b.FromSpanner(
				row,
				&tvr.Project,
				&tvr.TestID,
				&tvr.VariantHash,
				&tvr.SubRealm,
				&tvr.Variant,
				&tvr.LastIngestionTime,
			)
			if err != nil {
				return err
			}
			return fn(tvr)
		})
}

// TestVariantRealmSaveCols is the set of columns written to in a test variant
// realm save. Allocated here once to avoid reallocating on every save.
var TestVariantRealmSaveCols = []string{
	"Project", "TestId", "VariantHash", "SubRealm",
	"Variant", "LastIngestionTime",
}

// SaveUnverified creates a mutation to save the test variant realm into
// the TestVariantRealms table. The test variant realm is not verified.
// Must be called in spanner RW transactional context.
func (tvr *TestVariantRealm) SaveUnverified() *spanner.Mutation {
	vals := []any{
		tvr.Project, tvr.TestID, tvr.VariantHash, tvr.SubRealm,
		pbutil.VariantToStrings(tvr.Variant), tvr.LastIngestionTime,
	}
	return spanner.InsertOrUpdate("TestVariantRealms", TestVariantRealmSaveCols, vals)
}

// TestVariantRealm represents a row in the TestVariantRealm table.
type TestRealm struct {
	Project           string
	TestID            string
	SubRealm          string
	LastIngestionTime time.Time
}

// ReadTestRealms read test variant realms from the TestRealms table.
// Must be called in a spanner transactional context.
func ReadTestRealms(ctx context.Context, keys spanner.KeySet, fn func(tr *TestRealm) error) error {
	var b spanutil.Buffer
	fields := []string{"Project", "TestId", "SubRealm", "LastIngestionTime"}
	return span.Read(ctx, "TestRealms", keys, fields).Do(
		func(row *spanner.Row) error {
			tr := &TestRealm{}
			err := b.FromSpanner(
				row,
				&tr.Project,
				&tr.TestID,
				&tr.SubRealm,
				&tr.LastIngestionTime,
			)
			if err != nil {
				return err
			}
			return fn(tr)
		})
}

// TestRealmSaveCols is the set of columns written to in a test variant
// realm save. Allocated here once to avoid reallocating on every save.
var TestRealmSaveCols = []string{"Project", "TestId", "SubRealm", "LastIngestionTime"}

// SaveUnverified creates a mutation to save the test realm into the TestRealms
// table. The test realm is not verified.
// Must be called in spanner RW transactional context.
func (tvr *TestRealm) SaveUnverified() *spanner.Mutation {
	vals := []any{tvr.Project, tvr.TestID, tvr.SubRealm, tvr.LastIngestionTime}
	return spanner.InsertOrUpdate("TestRealms", TestRealmSaveCols, vals)
}

// ReadVariantsOptions specifies options for ReadVariants().
type ReadVariantsOptions struct {
	SubRealms        []string
	VariantPredicate *pb.VariantPredicate
	PageSize         int
	PageToken        string
}

// parseQueryVariantsPageToken parses the positions from the page token.
func parseQueryVariantsPageToken(pageToken string) (afterHash string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return "", err
	}

	if len(tokens) != 1 {
		return "", pagination.InvalidToken(errors.Reason("expected 1 components, got %d", len(tokens)).Err())
	}

	return tokens[0], nil
}

// ReadVariants reads all the variants of the specified test from the
// spanner database.
// Must be called in a spanner transactional context.
func ReadVariants(ctx context.Context, project, testID string, opts ReadVariantsOptions) (variants []*pb.QueryVariantsResponse_VariantInfo, nextPageToken string, err error) {
	paginationVariantHash := ""
	if opts.PageToken != "" {
		paginationVariantHash, err = parseQueryVariantsPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}

	params := map[string]any{
		"project":   project,
		"testId":    testID,
		"subRealms": opts.SubRealms,

		// Control pagination.
		"limit":                 opts.PageSize,
		"paginationVariantHash": paginationVariantHash,
	}
	input := map[string]any{
		"hasLimit": opts.PageSize > 0,
		"params":   params,
	}

	switch p := opts.VariantPredicate.GetPredicate().(type) {
	case *pb.VariantPredicate_Equals:
		input["hasVariantHash"] = true
		params["variantHash"] = pbutil.VariantHash(p.Equals)
	case *pb.VariantPredicate_Contains:
		if len(p.Contains.Def) > 0 {
			input["hasVariantKVs"] = true
			params["variantKVs"] = pbutil.VariantToStrings(p.Contains)
		}
	case *pb.VariantPredicate_HashEquals:
		input["hasVariantHash"] = true
		params["variantHash"] = p.HashEquals
	case nil:
		// No filter.
	default:
		panic(errors.Reason("unexpected variant predicate %q", opts.VariantPredicate).Err())
	}

	stmt, err := spanutil.GenerateStatement(variantsQueryTmpl, variantsQueryTmpl.Name(), input)
	if err != nil {
		return nil, "", err
	}
	stmt.Params = params

	var b spanutil.Buffer
	variants = make([]*pb.QueryVariantsResponse_VariantInfo, 0, opts.PageSize)
	err = span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		variant := &pb.QueryVariantsResponse_VariantInfo{}
		err := b.FromSpanner(
			row,
			&variant.VariantHash,
			&variant.Variant,
		)
		if err != nil {
			return err
		}
		variants = append(variants, variant)
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	if opts.PageSize != 0 && len(variants) == opts.PageSize {
		lastVariant := variants[len(variants)-1]
		nextPageToken = pagination.Token(lastVariant.VariantHash)
	}
	return variants, nextPageToken, nil
}

// QueryTestsOptions specifies options for QueryTests().
type QueryTestsOptions struct {
	CaseSensitive bool
	SubRealms     []string
	PageSize      int
	PageToken     string
}

// parseQueryTestsPageToken parses the positions from the page token.
func parseQueryTestsPageToken(pageToken string) (afterTestId string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return "", err
	}

	if len(tokens) != 1 {
		return "", pagination.InvalidToken(errors.Reason("expected 1 components, got %d", len(tokens)).Err())
	}

	return tokens[0], nil
}

// QueryTests finds all the test IDs with the specified testIDSubstring from
// the spanner database.
// Must be called in a spanner transactional context.
func QueryTests(ctx context.Context, project, testIDSubstring string, opts QueryTestsOptions) (testIDs []string, nextPageToken string, err error) {
	paginationTestID := ""
	if opts.PageToken != "" {
		paginationTestID, err = parseQueryTestsPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}
	params := map[string]any{
		"project":       project,
		"testIdPattern": "%" + spanutil.QuoteLike(testIDSubstring) + "%",
		"subRealms":     opts.SubRealms,

		// Control pagination.
		"limit":            opts.PageSize,
		"paginationTestId": paginationTestID,
	}
	input := map[string]any{
		"hasLimit":      opts.PageSize > 0,
		"params":        params,
		"caseSensitive": opts.CaseSensitive,
	}

	stmt, err := spanutil.GenerateStatement(QueryTestsQueryTmpl, QueryTestsQueryTmpl.Name(), input)
	if err != nil {
		return nil, "", err
	}
	stmt.Params = params

	var b spanutil.Buffer
	testIDs = make([]string, 0, opts.PageSize)
	err = span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		var testID string
		err := b.FromSpanner(
			row,
			&testID,
		)
		if err != nil {
			return err
		}
		testIDs = append(testIDs, testID)
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	if opts.PageSize != 0 && len(testIDs) == opts.PageSize {
		lastTestID := testIDs[len(testIDs)-1]
		nextPageToken = pagination.Token(lastTestID)
	}
	return testIDs, nextPageToken, nil
}

var testHistoryQueryTmpl = template.Must(template.New("").Parse(`
	{{define "tvStatus"}}
		CASE
			WHEN ANY_VALUE(ExonerationReasons IS NOT NULL AND ARRAY_LENGTH(ExonerationReasons) > 0) THEN @exonerated
			-- Use COALESCE as IsUnexpected uses NULL to indicate false.
			WHEN LOGICAL_AND(NOT COALESCE(IsUnexpected, FALSE)) THEN @expected
			WHEN LOGICAL_AND(COALESCE(IsUnexpected, FALSE) AND Status = @skip) THEN @unexpectedlySkipped
			WHEN LOGICAL_AND(COALESCE(IsUnexpected, FALSE)) THEN @unexpected
			ELSE @flaky
		END TvStatus
	{{end}}
	{{define "tvStatusV2"}}
		CASE
			WHEN LOGICAL_OR(StatusV2 = @passedV2) AND LOGICAL_OR(StatusV2 = @failedV2) THEN @flakyVerdict
			WHEN LOGICAL_OR(StatusV2 = @passedV2) THEN @passedVerdict
			WHEN LOGICAL_OR(StatusV2 = @failedV2) THEN @failedVerdict
			WHEN LOGICAL_OR(StatusV2 = @skippedV2) THEN @skippedVerdict
			WHEN LOGICAL_OR(StatusV2 = @executionErroredV2) THEN @executionErroredVerdict
			ELSE @precludedVerdict
		END TvStatusV2
	{{end}}

	{{define "testResultFilter"}}
		Project = @project
			AND TestId = @testId
			AND PartitionTime >= @afterTime
			AND PartitionTime < @beforeTime
			AND SubRealm IN UNNEST(@subRealms)
			{{if .hasVariantHash}}
				AND VariantHash = @variantHash
			{{end}}
			{{if .hasVariantKVs}}
				AND VariantHash IN (
					SELECT DISTINCT VariantHash
					FROM TestVariantRealms
					WHERE
						Project = @project
						AND TestId = @testId
						AND SubRealm IN UNNEST(@subRealms)
						AND (SELECT LOGICAL_AND(kv IN UNNEST(Variant)) FROM UNNEST(@variantKVs) kv)
				)
			{{end}}
			{{if .hasSubmittedFilter}}
				AND (ARRAY_LENGTH(ChangelistHosts) > 0) = @hasUnsubmittedChanges
			{{end}}
			{{if .excludeBisectionResults}}
				-- IsFromBisection uses NULL to indicate false.
				AND IsFromBisection IS NULL
			{{end}}
	{{end}}

	{{define "testHistoryQuery"}}
		SELECT
			PartitionTime,
			VariantHash,
			IngestedInvocationId,
			{{template "tvStatus" .}},
			CAST(AVG(IF(Status = @pass, RunDurationUsec, NULL)) AS INT64) AS PassedAvgDurationUsec,
			ANY_VALUE(ChangelistHosts) AS ChangelistHosts,
			ANY_VALUE(ChangelistChanges) AS ChangelistChanges,
			ANY_VALUE(ChangelistPatchsets) AS ChangelistPatchsets,
			ANY_VALUE(ChangelistOwnerKinds) AS ChangelistOwnerKinds,
		FROM TestResults
		WHERE
			{{template "testResultFilter" .}}
			{{if .pagination}}
				AND	(
					PartitionTime < TIMESTAMP(@paginationTime)
						OR (PartitionTime = TIMESTAMP(@paginationTime) AND VariantHash > @paginationVariantHash)
						OR (PartitionTime = TIMESTAMP(@paginationTime) AND VariantHash = @paginationVariantHash AND IngestedInvocationId > @paginationInvId)
				)
			{{end}}
		GROUP BY PartitionTime, VariantHash, IngestedInvocationId
		ORDER BY
			PartitionTime DESC,
			VariantHash ASC,
			IngestedInvocationId ASC
		{{if .hasLimit}}
			LIMIT @limit
		{{end}}
	{{end}}

	{{define "testHistoryStatsQuery"}}
		WITH verdicts AS (
			SELECT
				PartitionTime,
				VariantHash,
				IngestedInvocationId,
				{{template "tvStatus" .}},
				{{template "tvStatusV2" .}},
				ANY_VALUE(ExonerationReasons IS NOT NULL AND ARRAY_LENGTH(ExonerationReasons) > 0) As IsExonerated,
				COUNTIF(Status = @pass AND RunDurationUsec IS NOT NULL) AS PassedWithDurationCount,
				SUM(IF(Status = @pass, RunDurationUsec, 0)) AS SumPassedDurationUsec,
			FROM TestResults
			WHERE
				{{template "testResultFilter" .}}
				{{if .pagination}}
					AND	PartitionTime < TIMESTAMP_ADD(TIMESTAMP(@paginationDate), INTERVAL 1 DAY)
				{{end}}
			GROUP BY PartitionTime, VariantHash, IngestedInvocationId
		)

		SELECT
			TIMESTAMP_TRUNC(PartitionTime, DAY, "UTC") AS PartitionDate,
			VariantHash,
			COUNTIF(TvStatusV2 = @failedVerdict) AS Failed,
			COUNTIF(TvStatusV2 = @flakyVerdict) AS Flaky,
			COUNTIF(TvStatusV2 = @passedVerdict) AS Passed,
			COUNTIF(TvStatusV2 = @skippedVerdict) AS Skipped,
			COUNTIF(TvStatusV2 = @executionErroredVerdict) AS ExecutionErrored,
			COUNTIF(TvStatusV2 = @precludedVerdict) AS Precluded,
			COUNTIF(TvStatusV2 = @failedVerdict AND IsExonerated) AS FailedExonerated,
			COUNTIF(TvStatusV2 = @executionErroredVerdict AND IsExonerated) AS ExecutionErroredExonerated,
			COUNTIF(TvStatusV2 = @precludedVerdict AND IsExonerated) AS PrecludedExonerated,
			COUNTIF(TvStatus = @unexpected) AS Unexpected,
			COUNTIF(TvStatus = @unexpectedlySkipped) AS UnexpectedlySkipped,
			COUNTIF(TvStatus = @flaky) AS Flaky,
			COUNTIF(TvStatus = @exonerated) AS Exonerated,
			COUNTIF(TvStatus = @expected) AS Expected,
			CAST(SAFE_DIVIDE(SUM(SumPassedDurationUsec), SUM(PassedWithDurationCount)) AS INT64) AS PassedAvgDurationUsec,
		FROM verdicts
		GROUP BY PartitionDate, VariantHash
		{{if .pagination}}
			HAVING
				PartitionDate < TIMESTAMP(@paginationDate)
					OR (PartitionDate = TIMESTAMP(@paginationDate) AND VariantHash > @paginationVariantHash)
		{{end}}
		ORDER BY
			PartitionDate DESC,
			VariantHash ASC
		{{if .hasLimit}}
			LIMIT @limit
		{{end}}
	{{end}}
`))

var variantsQueryTmpl = template.Must(template.New("variantsQuery").Parse(`
	SELECT
		VariantHash,
		ANY_VALUE(Variant) as Variant,
	FROM TestVariantRealms
	WHERE
		Project = @project
			AND TestId = @testId
			AND SubRealm IN UNNEST(@subRealms)
			{{if .hasVariantHash}}
				AND VariantHash = @variantHash
			{{end}}
			{{if .hasVariantKVs}}
				AND (SELECT LOGICAL_AND(kv IN UNNEST(Variant)) FROM UNNEST(@variantKVs) kv)
			{{end}}
			AND VariantHash > @paginationVariantHash
	GROUP BY VariantHash
	ORDER BY VariantHash ASC
	{{if .hasLimit}}
		LIMIT @limit
	{{end}}
`))

// The query is written in a way to force spanner NOT to put
// `SubRealm IN UNNEST(@subRealms)` check in Filter Scan seek condition, which
// can significantly increase the time it takes to scan the table.
var QueryTestsQueryTmpl = template.Must(template.New("QueryTestsQuery").Parse(`
  @{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH Tests as (
		SELECT DISTINCT TestId, SubRealm IN UNNEST(@subRealms) as HasAccess
		FROM TestRealms
		WHERE
			Project = @project
				AND TestId > @paginationTestId
				{{if .caseSensitive}}
					AND TestId LIKE @testIdPattern
				{{else}}
					AND TestIdLower LIKE LOWER(@testIdPattern)
				{{end}}
	)
	SELECT TestId FROM Tests
	WHERE HasAccess
	ORDER BY TestId ASC
	{{if .hasLimit}}
		LIMIT @limit
	{{end}}
`))
