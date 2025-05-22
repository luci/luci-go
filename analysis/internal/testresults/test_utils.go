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

package testresults

import (
	"time"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TestResultBuilder provides methods to build a test result for testing.
type TestResultBuilder struct {
	result TestResult
}

func NewTestResult() TestResultBuilder {
	d := time.Hour
	result := TestResult{
		Project:              "proj",
		TestID:               "test_id",
		PartitionTime:        time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC),
		VariantHash:          "hash",
		IngestedInvocationID: "inv-id",
		RunIndex:             2,
		ResultIndex:          3,
		IsUnexpected:         true,
		RunDuration:          &d,
		Status:               pb.TestResultStatus_PASS,
		ExonerationReasons:   nil,
		SubRealm:             "realm",
		Sources: Sources{
			RefHash:  []byte{0, 1, 2, 3, 4, 5, 6, 7},
			Position: 999444,
			Changelists: []Changelist{
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

func (b TestResultBuilder) WithPartitionTime(partitionTime time.Time) TestResultBuilder {
	b.result.PartitionTime = partitionTime
	return b
}

func (b TestResultBuilder) WithVariantHash(variantHash string) TestResultBuilder {
	b.result.VariantHash = variantHash
	return b
}

func (b TestResultBuilder) WithIngestedInvocationID(invID string) TestResultBuilder {
	b.result.IngestedInvocationID = invID
	return b
}

func (b TestResultBuilder) WithRunIndex(runIndex int64) TestResultBuilder {
	b.result.RunIndex = runIndex
	return b
}

func (b TestResultBuilder) WithResultIndex(resultIndex int64) TestResultBuilder {
	b.result.ResultIndex = resultIndex
	return b
}

func (b TestResultBuilder) WithIsUnexpected(unexpected bool) TestResultBuilder {
	b.result.IsUnexpected = unexpected
	return b
}

func (b TestResultBuilder) WithRunDuration(duration time.Duration) TestResultBuilder {
	b.result.RunDuration = &duration
	return b
}

func (b TestResultBuilder) WithoutRunDuration() TestResultBuilder {
	b.result.RunDuration = nil
	return b
}

func (b TestResultBuilder) WithStatus(status pb.TestResultStatus) TestResultBuilder {
	b.result.Status = status
	return b
}

func (b TestResultBuilder) WithStatusV2(statusV2 pb.TestResult_Status) TestResultBuilder {
	b.result.StatusV2 = statusV2
	return b
}

func (b TestResultBuilder) WithExonerationReasons(exonerationReasons ...pb.ExonerationReason) TestResultBuilder {
	b.result.ExonerationReasons = exonerationReasons
	return b
}

func (b TestResultBuilder) WithoutExoneration() TestResultBuilder {
	b.result.ExonerationReasons = nil
	return b
}

func (b TestResultBuilder) WithSubRealm(subRealm string) TestResultBuilder {
	b.result.SubRealm = subRealm
	return b
}

func (b TestResultBuilder) WithSources(sources Sources) TestResultBuilder {
	// Copy sources to avoid aliasing artifacts. Changes made to
	// slices within the sources struct after this call should
	// not propagate to the test result.
	b.result.Sources = CopySources(sources)
	return b
}

func (b TestResultBuilder) WithIsFromBisection(value bool) TestResultBuilder {
	b.result.IsFromBisection = value
	return b
}

func (b TestResultBuilder) Build() *TestResult {
	// Copy the result, so that calling further methods on the builder does
	// not change the returned test verdict.
	result := new(TestResult)
	*result = b.result
	result.Sources = CopySources(b.result.Sources)
	return result
}

// CopySources makes a deep copy of the given code sources.
func CopySources(sources Sources) Sources {
	var refHash []byte
	if sources.RefHash != nil {
		refHash = make([]byte, len(sources.RefHash))
		copy(refHash, sources.RefHash)
	}

	cls := make([]Changelist, len(sources.Changelists))
	copy(cls, sources.Changelists)

	return Sources{
		RefHash:     refHash,
		Position:    sources.Position,
		Changelists: cls,
		IsDirty:     sources.IsDirty,
	}
}

// TestVerdictBuilder provides methods to build a test variant for testing.
type TestVerdictBuilder struct {
	baseResult        TestResult
	status            *pb.TestVerdict_Status
	statusOverride    pb.TestVerdict_StatusOverride
	runStatuses       []RunStatus
	passedAvgDuration *time.Duration
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
	status := pb.TestVerdict_FLAKY
	result.status = &status
	result.statusOverride = pb.TestVerdict_NOT_OVERRIDDEN
	result.runStatuses = nil
	d := 919191 * time.Microsecond
	result.passedAvgDuration = &d
	return result
}

// WithBaseTestResult specifies a test result to use as the template for
// the test variant's test results.
func (b *TestVerdictBuilder) WithBaseTestResult(testResult *TestResult) *TestVerdictBuilder {
	b.baseResult = *testResult
	return b
}

// WithPassedAvgDuration specifies the average duration to use for
// passed test results. If setting to a non-nil value, make sure
// to set the result status as passed on the base test result if
// using this option.
func (b *TestVerdictBuilder) WithPassedAvgDuration(duration *time.Duration) *TestVerdictBuilder {
	b.passedAvgDuration = duration
	return b
}

// WithStatus specifies the status of the test verdict.
func (b *TestVerdictBuilder) WithStatus(status pb.TestVerdict_Status) *TestVerdictBuilder {
	b.status = &status
	b.runStatuses = nil
	return b
}

// WithStatus specifies the status of the test verdict.
func (b *TestVerdictBuilder) WithStatusOverride(statusOverride pb.TestVerdict_StatusOverride) *TestVerdictBuilder {
	b.statusOverride = statusOverride
	return b
}

// WithRunStatus specifies the status of runs of the test verdict.
func (b *TestVerdictBuilder) WithRunStatus(runStatuses ...RunStatus) *TestVerdictBuilder {
	b.status = nil
	b.runStatuses = runStatuses
	return b
}

func applyStatus(trs []*TestResult, status pb.TestVerdict_Status, statusOverride pb.TestVerdict_StatusOverride) {
	// Set all test results to precluded, not exonerated by default.
	for _, tr := range trs {
		tr.StatusV2 = pb.TestResult_PRECLUDED
		tr.ExonerationReasons = nil
		// Populate the v1 Status values for backwards compatibility.
		tr.IsUnexpected = true
		tr.Status = pb.TestResultStatus_SKIP
	}

	switch status {
	case pb.TestVerdict_FAILED:
		// Adding a FAILED result makes the verdict FAILED provided
		// there are no PASSED results (which there are not).
		trs[0].StatusV2 = pb.TestResult_FAILED

		// Also create the v1 verdict status UNEXPECTED.
		for _, tr := range trs {
			tr.IsUnexpected = true
			// Needed to avoid the status becoming unexpectedly skipped.
			tr.Status = pb.TestResultStatus_FAIL
		}
	case pb.TestVerdict_PASSED:
		// Adding a passing result makes the verdict PASSED provided
		// there are no FAILED results (which there are not).
		trs[0].StatusV2 = pb.TestResult_PASSED

		// Make all results expected to also produce the the v1 verdict
		// status EXPECTED.
		for _, tr := range trs {
			tr.Status = pb.TestResultStatus_PASS // Allows computing average passed duration.
			tr.IsUnexpected = false
		}
	case pb.TestVerdict_FLAKY:
		// Adding a passing and failing result makes the status FLAKY
		// regardless of of the results.
		trs[0].StatusV2 = pb.TestResult_PASSED
		trs[1].StatusV2 = pb.TestResult_FAILED

		// Produce a flaky v1 verdict.
		trs[0].IsUnexpected = false
		trs[1].IsUnexpected = true

		// Make one of the results a pass so that we can contribute to average passed duration.
		trs[0].Status = pb.TestResultStatus_PASS
		trs[1].Status = pb.TestResultStatus_FAIL

	case pb.TestVerdict_SKIPPED:
		// Adding a SKIP result to a set of precluded or execution errored
		// results makes the verdict SKIP.
		trs[0].StatusV2 = pb.TestResult_SKIPPED

		// Also produce a v1 verdict status of expected.
		for _, tr := range trs {
			tr.Status = pb.TestResultStatus_SKIP // should not matter.
			tr.IsUnexpected = false
		}

	case pb.TestVerdict_EXECUTION_ERRORED:
		// Adding one execution errored result to a set of precluded results
		// makes the verdict EXECUTION_ERRORED.
		trs[0].StatusV2 = pb.TestResult_EXECUTION_ERRORED

	case pb.TestVerdict_PRECLUDED:
		// No need to change anything, verdict will already
		// be precluded as all test results are precluded.

	default:
		panic("status must be specified")
	}
	switch statusOverride {
	case pb.TestVerdict_EXONERATED:
		for _, tr := range trs {
			tr.ExonerationReasons = []pb.ExonerationReason{pb.ExonerationReason_OCCURS_ON_MAINLINE}
		}
	case pb.TestVerdict_NOT_OVERRIDDEN:
		// No changes required.
	default:
		panic("status override must be specified")
	}
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

func applyAvgPassedDuration(trs []*TestResult, passedAvgDuration *time.Duration) {
	if passedAvgDuration == nil {
		for _, tr := range trs {
			if tr.Status == pb.TestResultStatus_PASS {
				tr.RunDuration = nil
			}
		}
		return
	}

	passCount := 0
	for _, tr := range trs {
		if tr.Status == pb.TestResultStatus_PASS {
			passCount++
		}
	}
	passIndex := 0
	for _, tr := range trs {
		if tr.Status == pb.TestResultStatus_PASS {
			d := *passedAvgDuration
			if passCount == 1 {
				// If there is only one pass, assign it the
				// set duration.
				tr.RunDuration = &d
				break
			}
			if passIndex == 0 && passCount%2 == 1 {
				// If there are an odd number of passes, and
				// more than one pass, assign the first pass
				// a nil duration.
				tr.RunDuration = nil
			} else {
				// Assigning alternating passes 2*d the duration
				// and 0 duration, to keep the average correct.
				if passIndex%2 == 0 {
					d = d * 2
					tr.RunDuration = &d
				} else {
					d = 0
					tr.RunDuration = &d
				}
			}
			passIndex++
		}
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
		tr.RunIndex = int64(i / 2)
		tr.ResultIndex = int64(i % 2)
		trs = append(trs, tr)
	}

	// Normally only one of these should be set.
	// If both are set, run statuses has precedence.
	if b.status != nil {
		applyStatus(trs, *b.status, b.statusOverride)
	}
	for i, runStatus := range b.runStatuses {
		runTRs := trs[i*2 : (i+1)*2]
		applyRunStatus(runTRs, runStatus)
	}

	applyAvgPassedDuration(trs, b.passedAvgDuration)
	return trs
}
