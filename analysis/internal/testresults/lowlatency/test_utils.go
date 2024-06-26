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

package lowlatency

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TestResultBuilder provides methods to build a test result for testing.
type TestResultBuilder struct {
	result TestResult
}

func NewTestResult() TestResultBuilder {
	result := TestResult{
		Project:     "proj",
		TestID:      "test_id",
		VariantHash: "hash",
		Sources: testresults.Sources{
			RefHash:  []byte{0, 1, 2, 3, 4, 5, 6, 7},
			Position: 999444,
			Changelists: []testresults.Changelist{
				{
					Host:     "mygerrit-review.googlesource.com",
					Change:   12345678,
					Patchset: 9,
				},
				{
					Host:     "anothergerrit.gerrit.instance",
					Change:   234568790,
					Patchset: 1,
				},
			},
			IsDirty: true,
		},
		RootInvocationID: "root-inv-id",
		InvocationID:     "inv-id",
		ResultID:         "result-id",
		PartitionTime:    time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC),
		SubRealm:         "realm",
		IsUnexpected:     true,
		Status:           pb.TestResultStatus_PASS,
	}
	return TestResultBuilder{
		result: result,
	}
}

func (b TestResultBuilder) WithProject(project string) TestResultBuilder {
	b.result.Project = project
	return b
}

func (b TestResultBuilder) WithTestID(testID string) TestResultBuilder {
	b.result.TestID = testID
	return b
}

func (b TestResultBuilder) WithVariantHash(variantHash string) TestResultBuilder {
	b.result.VariantHash = variantHash
	return b
}

func (b TestResultBuilder) WithSources(sources testresults.Sources) TestResultBuilder {
	// Copy sources to avoid aliasing artifacts. Changes made to
	// slices within the sources struct after this call should
	// not propagate to the test result.
	b.result.Sources = testresults.CopySources(sources)
	return b
}

func (b TestResultBuilder) WithRootInvocationID(rootInvID string) TestResultBuilder {
	b.result.RootInvocationID = rootInvID
	return b
}

func (b TestResultBuilder) WithInvocationID(invID string) TestResultBuilder {
	b.result.InvocationID = invID
	return b
}

func (b TestResultBuilder) WithResultID(resultID string) TestResultBuilder {
	b.result.ResultID = resultID
	return b
}

// WithPartitionTime specifies the partition time of the test result.
func (b TestResultBuilder) WithPartitionTime(partitionTime time.Time) TestResultBuilder {
	b.result.PartitionTime = partitionTime
	return b
}

// WithSubRealm specifies the subrealm of the test result.
func (b TestResultBuilder) WithSubRealm(subRealm string) TestResultBuilder {
	b.result.SubRealm = subRealm
	return b
}

// WithIsUnexpected specifies whether the test result is unexpected.
func (b TestResultBuilder) WithIsUnexpected(isUnexpected bool) TestResultBuilder {
	b.result.IsUnexpected = isUnexpected
	return b
}

// WithStatus specifies the status of the test result.
func (b TestResultBuilder) WithStatus(status pb.TestResultStatus) TestResultBuilder {
	b.result.Status = status
	return b
}

func (b TestResultBuilder) Build() *TestResult {
	// Copy the result, so that calling further methods on the builder does
	// not change the returned test verdict.
	result := new(TestResult)
	*result = b.result
	result.Sources = testresults.CopySources(b.result.Sources)
	return result
}

// TestVerdictBuilder provides methods to build a test variant for testing.
type TestVerdictBuilder struct {
	baseResult  TestResult
	runStatuses []RunStatus
}

type RunStatus int64

const (
	Unexpected RunStatus = iota
	Flaky
	Expected
)

func NewTestVerdict() *TestVerdictBuilder {
	result := new(TestVerdictBuilder)
	result.baseResult = *NewTestResult().WithStatus(pb.TestResultStatus_PASS).Build()
	result.runStatuses = nil
	return result
}

// WithBaseTestResult specifies a test result to use as the template for
// the test variant's test results.
func (b *TestVerdictBuilder) WithBaseTestResult(testResult *TestResult) *TestVerdictBuilder {
	b.baseResult = *testResult
	return b
}

// WithRunStatus specifies the status of runs of the test verdict.
func (b *TestVerdictBuilder) WithRunStatus(runStatuses ...RunStatus) *TestVerdictBuilder {
	b.runStatuses = runStatuses
	return b
}

// applyRunStatus applies the given run status to the given test results.
func applyRunStatus(trs []*TestResult, runStatus RunStatus) {
	for _, tr := range trs {
		tr.IsUnexpected = true
	}
	switch runStatus {
	case Expected:
		for _, tr := range trs {
			tr.IsUnexpected = false
		}
	case Flaky:
		trs[0].IsUnexpected = false
	case Unexpected:
		// All test results already unexpected.
	}
}

func (b *TestVerdictBuilder) Build() []*TestResult {
	runs := 2
	if len(b.runStatuses) > 0 {
		runs = len(b.runStatuses)
	}

	// Create two test results per run, to allow
	// for all expected, all unexpected and
	// flaky (mixed expected+unexpected) statuses
	// to be represented.
	trs := make([]*TestResult, 0, runs*2)
	for i := 0; i < runs*2; i++ {
		tr := new(TestResult)
		*tr = b.baseResult
		tr.InvocationID = fmt.Sprintf("child-%s-run-%v", b.baseResult.RootInvocationID, i/2)
		tr.ResultID = fmt.Sprintf("result-%v", i%2)
		trs = append(trs, tr)
	}

	for i, runStatus := range b.runStatuses {
		runTRs := trs[i*2 : (i+1)*2]
		applyRunStatus(runTRs, runStatus)
	}

	return trs
}

// SetForTesting replaces the stored test results with the given list.
// For use in unit/integration tests only.
func SetForTesting(ctx context.Context, results []*TestResult) error {
	testutil.MustApply(ctx,
		spanner.Delete("TestResultsBySourcePosition", spanner.AllKeys()))
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, tr := range results {
			span.BufferWrite(ctx, tr.SaveUnverified())
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
