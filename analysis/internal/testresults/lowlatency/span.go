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
