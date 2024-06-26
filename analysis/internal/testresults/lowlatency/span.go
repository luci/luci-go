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

// Package lowlatency contains methods for accessing the
// low-latency test results table in Spanner.
package lowlatency

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testresults"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TestResult represents a row in the TestResultsBySourcePosition table.
type TestResult struct {
	Project          string
	TestID           string
	VariantHash      string
	Sources          testresults.Sources
	RootInvocationID string
	InvocationID     string
	ResultID         string
	PartitionTime    time.Time
	SubRealm         string
	IsUnexpected     bool
	Status           pb.TestResultStatus
}

// ReadAllForTesting reads all test results from the
// TestResultsBySourcePosition table for testing.
// Must be called in a spanner transactional context.
func ReadAllForTesting(ctx context.Context) ([]*TestResult, error) {
	sql := `SELECT Project, TestId, VariantHash, SourceRefHash, SourcePosition,
		RootInvocationId, InvocationId, ResultId, PartitionTime, SubRealm,
		IsUnexpected, Status, ChangelistHosts, ChangelistChanges,
		ChangelistPatchsets, ChangelistOwnerKinds, HasDirtySources
	FROM TestResultsBySourcePosition
	ORDER BY Project, TestId, VariantHash, RootInvocationId, InvocationId, ResultId`

	stmt := spanner.NewStatement(sql)
	var b spanutil.Buffer
	var results []*TestResult
	it := span.Query(ctx, stmt)

	f := func(row *spanner.Row) error {
		tr := &TestResult{}
		var changelistHosts []string
		var changelistChanges []int64
		var changelistPatchsets []int64
		var changelistOwnerKinds []string
		err := b.FromSpanner(
			row,
			&tr.Project,
			&tr.TestID,
			&tr.VariantHash,
			&tr.Sources.RefHash,
			&tr.Sources.Position,
			&tr.RootInvocationID,
			&tr.InvocationID,
			&tr.ResultID,
			&tr.PartitionTime,
			&tr.SubRealm,
			&tr.IsUnexpected,
			&tr.Status,
			&changelistHosts, &changelistChanges, &changelistPatchsets, &changelistOwnerKinds,
			&tr.Sources.IsDirty,
		)
		if err != nil {
			return err
		}

		// Data in spanner should be consistent, so
		// len(changelistHosts) == len(changelistChanges)
		//    == len(changelistPatchsets)
		//    == len(changelistOwnerKinds).
		if len(changelistHosts) != len(changelistChanges) ||
			len(changelistHosts) != len(changelistPatchsets) ||
			len(changelistHosts) != len(changelistOwnerKinds) {
			panic("Changelist arrays have mismatched length in Spanner")
		}
		changelists := make([]testresults.Changelist, 0, len(changelistHosts))
		for i := range changelistHosts {
			var ownerKind pb.ChangelistOwnerKind
			if changelistOwnerKinds != nil {
				ownerKind = testresults.OwnerKindFromDB(changelistOwnerKinds[i])
			}
			changelists = append(changelists, testresults.Changelist{
				Host:      testresults.DecompressHost(changelistHosts[i]),
				Change:    changelistChanges[i],
				Patchset:  changelistPatchsets[i],
				OwnerKind: ownerKind,
			})
		}
		tr.Sources.Changelists = changelists

		results = append(results, tr)
		return nil
	}
	err := it.Do(f)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// TestResultSaveCols is the set of columns written to in a test result save.
// Allocated here once to avoid reallocating on every test result save.
var TestResultSaveCols = []string{
	"Project", "TestId", "VariantHash", "SourceRefHash", "SourcePosition",
	"RootInvocationId", "InvocationId", "ResultId", "PartitionTime", "SubRealm",
	"IsUnexpected", "Status", "ChangelistHosts", "ChangelistChanges",
	"ChangelistPatchsets", "ChangelistOwnerKinds", "HasDirtySources",
}

// SaveUnverified prepare a mutation to insert the test result into the
// TestResultsBySourcePosition table. The test result is not validated.
func (tr *TestResult) SaveUnverified() *spanner.Mutation {
	changelistHosts := make([]string, 0, len(tr.Sources.Changelists))
	changelistChanges := make([]int64, 0, len(tr.Sources.Changelists))
	changelistPatchsets := make([]int64, 0, len(tr.Sources.Changelists))
	changelistOwnerKinds := make([]string, 0, len(tr.Sources.Changelists))
	for _, cl := range tr.Sources.Changelists {
		changelistHosts = append(changelistHosts, testresults.CompressHost(cl.Host))
		changelistChanges = append(changelistChanges, cl.Change)
		changelistPatchsets = append(changelistPatchsets, int64(cl.Patchset))
		changelistOwnerKinds = append(changelistOwnerKinds, testresults.OwnerKindToDB(cl.OwnerKind))
	}

	// Specify values in a slice directly instead of
	// creating a map and using spanner.InsertOrUpdateMap.
	// Profiling revealed ~15% of all CPU cycles spent
	// ingesting test results were wasted generating a
	// map and converting it back to the slice
	// needed for a *spanner.Mutation using InsertOrUpdateMap.
	vals := []any{
		tr.Project, tr.TestID, tr.VariantHash, tr.Sources.RefHash, tr.Sources.Position,
		tr.RootInvocationID, tr.InvocationID, tr.ResultID, tr.PartitionTime, tr.SubRealm,
		tr.IsUnexpected, int64(tr.Status), changelistHosts, changelistChanges,
		changelistPatchsets, changelistOwnerKinds, tr.Sources.IsDirty,
	}
	return spanner.InsertOrUpdate("TestResultsBySourcePosition", TestResultSaveCols, vals)
}

// SourceVerdict aggregates all test results at a source position.
type SourceVerdict struct {
	// The source position.
	Position int64
	// Test verdicts at the position. Limited to 20.
	Verdicts []SourceVerdictTestVerdict
}

// SourceVerdictTestVerdict is a test verdict that is part of a source verdict.
type SourceVerdictTestVerdict struct {
	// The root invocation for the verdict.
	RootInvocationID string
	// Status is one of SKIPPED, EXPECTED, UNEXPECTED, FLAKY.
	Status pb.QuerySourceVerdictsResponse_VerdictStatus
	// Changelists tested by the verdict.
	Changelists []testresults.Changelist
}

type ReadSourceVerdictsOptions struct {
	// The LUCI Project.
	Project     string
	TestID      string
	VariantHash string
	RefHash     []byte
	// Only test verdicts with allowed invocation realms can be returned.
	AllowedSubrealms []string
	// The minimum source position to return, inclusive.
	StartSourcePosition int64
	// The maximum source position to return, exclusive.
	EndSourcePosition int64
	// The earliest partition time to include in the results, inclusive.
	StartPartitionTime time.Time
}

// ReadSourceVerdicts reads source verdicts for a test variant branch
// for a given range of source positions.
// Must be called in a spanner transactional context.
func ReadSourceVerdicts(ctx context.Context, opts ReadSourceVerdictsOptions) ([]SourceVerdict, error) {
	sql := `WITH TestVerdictsPrecompute AS (
			SELECT
				RootInvocationID,
				-- All of these should be the same for all test results in a verdict.
				ANY_VALUE(SourcePosition) AS Position,
				ANY_VALUE(ChangelistHosts) as ChangelistHosts,
				ANY_VALUE(ChangelistChanges) as ChangelistChanges,
				ANY_VALUE(ChangelistPatchsets) as ChangelistPatchsets,
				ANY_VALUE(ChangelistOwnerKinds) as ChangelistOwnerKinds,
				ANY_VALUE(PartitionTime) as PartitionTime,
				-- For computing the test verdict.
				COUNTIF(Status <> @skipStatus AND NOT IsUnexpected) AS ExpectedUnskippedCount,
				COUNTIF(Status <> @skipStatus AND IsUnexpected) AS UnexpectedUnskippedCount,
			FROM TestResultsBySourcePosition
			WHERE
				Project = @project
				AND TestId = @testID
				AND VariantHash = @variantHash
				AND SourceRefHash = @refHash
				AND SourcePosition < @endSourcePosition
				AND SourcePosition >= @startSourcePosition
				AND PartitionTime >= @startPartitionTime
				-- Filter out dirty sources, these results are always ignored by changepoint analysis.
				AND NOT HasDirtySources
				-- Limit to realms we can see.
				AND SubRealm IN UNNEST(@allowedSubrealms)
			GROUP BY RootInvocationID
		)
	SELECT
		Position,
		RootInvocationID,
		-- Compute the status of the test verdict as seen by changepoint analysis.
		-- Possible verdicts are EXPECTED, UNEXPECTEDLY_SKIPPED, UNEXPECTED and SKIPPED.
		CASE
			WHEN (ExpectedUnskippedCount + UnexpectedUnskippedCount) = 0 THEN @skippedVerdict
			WHEN ExpectedUnskippedCount > 0 AND UnexpectedUnskippedCount > 0 THEN @flakyVerdict
			WHEN ExpectedUnskippedCount > 0 THEN @expectedVerdict
			ELSE @unexpectedVerdict
		END AS Status,
		ChangelistHosts,
		ChangelistChanges,
		ChangelistPatchsets,
		ChangelistOwnerKinds
	FROM TestVerdictsPrecompute
	-- For each position, we want to keep the most recent test verdicts first (preferentially).
	ORDER BY Position, PartitionTime DESC, RootInvocationID`

	stmt := spanner.NewStatement(sql)
	stmt.Params = map[string]interface{}{
		"project":             opts.Project,
		"testID":              opts.TestID,
		"variantHash":         opts.VariantHash,
		"refHash":             opts.RefHash,
		"startSourcePosition": opts.StartSourcePosition,
		"endSourcePosition":   opts.EndSourcePosition,
		"startPartitionTime":  opts.StartPartitionTime,
		"allowedSubrealms":    opts.AllowedSubrealms,
		"skipStatus":          int64(pb.TestResultStatus_SKIP),
		"skippedVerdict":      int64(pb.QuerySourceVerdictsResponse_SKIPPED),
		"flakyVerdict":        int64(pb.QuerySourceVerdictsResponse_FLAKY),
		"expectedVerdict":     int64(pb.QuerySourceVerdictsResponse_EXPECTED),
		"unexpectedVerdict":   int64(pb.QuerySourceVerdictsResponse_UNEXPECTED),
	}
	it := span.Query(ctx, stmt)

	var b spanutil.Buffer
	var results []SourceVerdict

	f := func(row *spanner.Row) error {
		var position int64
		var rootInvocationID string
		var changelistHosts []string
		var changelistChanges []int64
		var changelistPatchsets []int64
		var changelistOwnerKinds []string
		var status int64

		err := b.FromSpanner(
			row,
			&position,
			&rootInvocationID,
			&status,
			&changelistHosts,
			&changelistChanges,
			&changelistPatchsets,
			&changelistOwnerKinds,
		)
		if err != nil {
			return err
		}

		// Data in spanner should be consistent, so
		// len(changelistHosts) == len(changelistChanges)
		//    == len(changelistPatchsets)
		//    == len(changelistOwnerKinds).
		if len(changelistHosts) != len(changelistChanges) ||
			len(changelistHosts) != len(changelistPatchsets) ||
			len(changelistHosts) != len(changelistOwnerKinds) {
			panic("Changelist arrays have mismatched length in Spanner")
		}
		var changelists []testresults.Changelist
		for i := range changelistHosts {
			var ownerKind pb.ChangelistOwnerKind
			if changelistOwnerKinds != nil {
				ownerKind = testresults.OwnerKindFromDB(changelistOwnerKinds[i])
			}
			changelists = append(changelists, testresults.Changelist{
				Host:      testresults.DecompressHost(changelistHosts[i]),
				Change:    changelistChanges[i],
				Patchset:  changelistPatchsets[i],
				OwnerKind: ownerKind,
			})
		}

		if len(results) == 0 || results[len(results)-1].Position != position {
			// Advanced position, start a new source verdict.
			results = append(results, SourceVerdict{
				Position: position,
			})
		}
		sourceVerdict := &results[len(results)-1]

		// Limit the number of verdicts recorded per source verdict.
		if len(sourceVerdict.Verdicts) < 20 {
			sourceVerdict.Verdicts = append(sourceVerdict.Verdicts, SourceVerdictTestVerdict{
				RootInvocationID: rootInvocationID,
				Status:           pb.QuerySourceVerdictsResponse_VerdictStatus(status),
				Changelists:      changelists,
			})
		}
		return nil
	}
	err := it.Do(f)
	if err != nil {
		return nil, err
	}
	return results, nil
}
