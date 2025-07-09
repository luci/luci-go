// Copyright 2019 The LUCI Authors.
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

// Package insert implements functions to insert rows for testing purposes.
package insert

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	durpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestRealm is the default realm used for invocation mutations returned by Invocation().
const TestRealm = "testproject:testrealm"

func updateDict(dest, source map[string]any) {
	for k, v := range source {
		dest[k] = v
	}
}

// Invocation returns a spanner mutation that inserts an invocation.
func Invocation(id invocations.ID, state pb.Invocation_State, extraValues map[string]any) *spanner.Mutation {
	future := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	values := map[string]any{
		"InvocationId":                      id,
		"ShardId":                           0,
		"State":                             state,
		"Realm":                             TestRealm,
		"InvocationExpirationTime":          future,
		"ExpectedTestResultsExpirationTime": future,
		"CreateTime":                        spanner.CommitTimestamp,
		"Deadline":                          future,
	}

	if state == pb.Invocation_FINALIZING || state == pb.Invocation_FINALIZED {
		values["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	if state == pb.Invocation_FINALIZED {
		values["FinalizeTime"] = spanner.CommitTimestamp
	}
	updateDict(values, extraValues)
	return spanutil.InsertMap("Invocations", values)
}

// FinalizedInvocationWithInclusions returns mutations to insert a finalized invocation with inclusions.
func FinalizedInvocationWithInclusions(id invocations.ID, extraValues map[string]any, included ...invocations.ID) []*spanner.Mutation {
	return InvocationWithInclusions(id, pb.Invocation_FINALIZED, extraValues, included...)
}

// InvocationWithInclusions returns mutations to insert an invocation with inclusions.
func InvocationWithInclusions(id invocations.ID, state pb.Invocation_State, extraValues map[string]any, included ...invocations.ID) []*spanner.Mutation {
	ms := []*spanner.Mutation{Invocation(id, state, extraValues)}
	for _, incl := range included {
		ms = append(ms, Inclusion(id, incl))
	}
	return ms
}

// Inclusion returns a spanner mutation that inserts an inclusion.
func Inclusion(including, included invocations.ID) *spanner.Mutation {
	return spanutil.InsertMap("IncludedInvocations", map[string]any{
		"InvocationId":         including,
		"IncludedInvocationId": included,
	})
}

// TestResults returns spanner mutations to insert test results.
func TestResults(t testing.TB, invID, testID string, v *pb.Variant, statuses ...pb.TestResult_Status) []*spanner.Mutation {
	return TestResultMessages(t, MakeTestResults(invID, testID, v, statuses...))
}

// TestResults returns spanner mutations to insert test results in legacy stored format.
func TestResultsLegacy(t testing.TB, invID, testID string, v *pb.Variant, statuses ...pb.TestStatus) []*spanner.Mutation {
	return TestResultMessagesLegacy(t, MakeTestResultsLegacy(invID, testID, v, statuses...))
}

// TestResultMessages returns spanner mutations to insert test results
func TestResultMessages(t testing.TB, trs []*pb.TestResult) []*spanner.Mutation {
	t.Helper()

	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testID, resultID, err := pbutil.ParseTestResultName(tr.Name)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())

		mutMap := map[string]any{
			"InvocationId":    invocations.ID(invID),
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         tr.Variant,
			"VariantHash":     pbutil.VariantHash(tr.Variant),
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          tr.Status,
			"StatusV2":        tr.StatusV2,
			"Tags":            tr.Tags,
			"StartTime":       tr.StartTime,
			"SummaryHtml":     spanutil.Compressed(tr.SummaryHtml),
		}
		if tr.Duration != nil {
			mutMap["RunDurationUsec"] = spanner.NullInt64{Int64: int64(tr.Duration.Seconds)*1e6 + int64(trs[i].Duration.Nanos)/1000, Valid: true}
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}
		if tr.TestMetadata != nil {
			tmdBytes, err := proto.Marshal(tr.TestMetadata)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["TestMetadata"] = spanutil.Compressed(tmdBytes)
		}
		if tr.FailureReason != nil {
			fr := proto.Clone(tr.FailureReason).(*pb.FailureReason)
			normaliseFailureReason(fr)
			frBytes, err := proto.Marshal(fr)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["FailureReason"] = spanutil.Compressed(frBytes)
		}
		if tr.Properties != nil {
			propertiesBytes, err := proto.Marshal(tr.Properties)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["Properties"] = spanutil.Compressed(propertiesBytes)
		}
		if tr.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
			mutMap["SkipReason"] = tr.SkipReason
		}
		if tr.SkippedReason != nil {
			skippedReasonBytes, err := proto.Marshal(tr.SkippedReason)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["SkippedReason"] = spanutil.Compressed(skippedReasonBytes)
		}
		if tr.FrameworkExtensions != nil {
			frameworkExtensionsBytes, err := proto.Marshal(tr.FrameworkExtensions)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["FrameworkExtensions"] = spanutil.Compressed(frameworkExtensionsBytes)
		}

		ms[i] = spanutil.InsertMap("TestResults", mutMap)
	}
	return ms
}

// normaliseFailureReason normalises a failure reason into its storage format.
func normaliseFailureReason(fr *pb.FailureReason) {
	if len(fr.Errors) == 0 && fr.PrimaryErrorMessage != "" {
		// Older results: normalise by set Errors collection from
		// PrimaryErrorMessage.
		fr.Errors = []*pb.FailureReason_Error{{Message: fr.PrimaryErrorMessage}}
	}
	fr.PrimaryErrorMessage = ""
}

// TestResultMessages returns spanner mutations to insert test results
// in legacy format. This may have some data loss.
func TestResultMessagesLegacy(t testing.TB, trs []*pb.TestResult) []*spanner.Mutation {
	t.Helper()

	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testID, resultID, err := pbutil.ParseTestResultName(tr.Name)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())

		mutMap := map[string]any{
			"InvocationId":    invocations.ID(invID),
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         tr.Variant,
			"VariantHash":     pbutil.VariantHash(tr.Variant),
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          tr.Status,
			"StatusV2":        tr.StatusV2,
			"Tags":            tr.Tags,
			"StartTime":       tr.StartTime,
			"SummaryHtml":     spanutil.Compressed(tr.SummaryHtml),
		}
		if tr.Duration != nil {
			mutMap["RunDurationUsec"] = spanner.NullInt64{Int64: int64(tr.Duration.Seconds)*1e6 + int64(trs[i].Duration.Nanos)/1000, Valid: true}
		}
		if tr.SkipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
			mutMap["SkipReason"] = tr.SkipReason
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}
		if tr.TestMetadata != nil {
			tmdBytes, err := proto.Marshal(tr.TestMetadata)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["TestMetadata"] = spanutil.Compressed(tmdBytes)
		}
		if tr.FailureReason != nil {
			fr := proto.Clone(tr.FailureReason).(*pb.FailureReason)
			if len(fr.Errors) > 0 {
				// At least until October 2026: legacy failure reasons may
				// might not set Errors collection and instead set PrimaryErrorMessage.
				fr.PrimaryErrorMessage = fr.Errors[0].Message
				if len(fr.Errors) == 1 {
					fr.Errors = nil
				}
			}
			// At least until October 2026: legacy failure reasons may
			// not have a kind set.
			fr.Kind = pb.FailureReason_KIND_UNSPECIFIED

			frBytes, err := proto.Marshal(fr)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["FailureReason"] = spanutil.Compressed(frBytes)
		}
		if tr.Properties != nil {
			propertiesBytes, err := proto.Marshal(tr.Properties)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			mutMap["Properties"] = spanutil.Compressed(propertiesBytes)
		}

		ms[i] = spanutil.InsertMap("TestResults", mutMap)
	}
	return ms
}

// TestExonerations returns Spanner mutations to insert test exonerations.
func TestExonerations(invID invocations.ID, testID string, variant *pb.Variant, reasons ...pb.ExonerationReason) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(reasons))
	for i := range reasons {
		ms[i] = spanutil.InsertMap("TestExonerations", map[string]any{
			"InvocationId":    invID,
			"TestId":          testID,
			"ExonerationId":   strconv.Itoa(i),
			"Variant":         variant,
			"VariantHash":     pbutil.VariantHash(variant),
			"ExplanationHTML": spanutil.Compressed(fmt.Sprintf("explanation %d", i)),
			"Reason":          reasons[i],
		})
	}
	return ms
}

// Artifact returns a Spanner mutation to insert an artifact.
func Artifact(invID invocations.ID, parentID, artID string, extraValues map[string]any) *spanner.Mutation {
	values := map[string]any{
		"InvocationId": invID,
		"ParentID":     parentID,
		"ArtifactId":   artID,
	}
	updateDict(values, extraValues)
	return spanutil.InsertMap("Artifacts", values)
}

func TestMetadataRows(rows []*testmetadata.TestMetadataRow) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(rows))
	for i, row := range rows {
		mutMap := map[string]any{
			"Project":      row.Project,
			"TestId":       row.TestID,
			"SubRealm":     row.SubRealm,
			"RefHash":      row.RefHash,
			"LastUpdated":  row.LastUpdated,
			"TestMetadata": spanutil.Compressed(pbutil.MustMarshal(row.TestMetadata)),
			"SourceRef":    spanutil.Compressed(pbutil.MustMarshal(row.SourceRef)),
			"Position":     int64(row.Position),
		}
		ms[i] = spanutil.InsertMap("TestMetadata", mutMap)
	}
	return ms
}

func MakeTestMetadataRow(project, testID, subRealm string, refHash []byte) *testmetadata.TestMetadataRow {
	return &testmetadata.TestMetadataRow{
		Project:     project,
		TestID:      testID,
		RefHash:     refHash,
		SubRealm:    subRealm,
		LastUpdated: time.Time{},
		TestMetadata: &pb.TestMetadata{
			Name: project + "\n" + testID + "\n" + subRealm + "\n" + string(refHash),
			Location: &pb.TestLocation{
				Repo:     "testRepo",
				FileName: "testFile",
				Line:     0,
			},
			BugComponent: &pb.BugComponent{},
		},
		SourceRef: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{Host: "testHost"},
			},
		},
		Position: 0,
	}
}

// MakeTestResult creates test results. Test results are returned with
// output only fields set.
func MakeTestResults(invID, testID string, v *pb.Variant, statuses ...pb.TestResult_Status) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)

		var failureReason *pb.FailureReason
		var skipReason pb.SkipReason
		var skippedReason *pb.SkippedReason
		if status == pb.TestResult_FAILED {
			failureReason = &pb.FailureReason{
				Kind:                pb.FailureReason_ORDINARY,
				PrimaryErrorMessage: "failure reason",
				Errors: []*pb.FailureReason_Error{
					{Message: "failure reason"},
				},
			}
		}
		if status == pb.TestResult_SKIPPED {
			skipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			skippedReason = &pb.SkippedReason{
				Kind:          pb.SkippedReason_DEMOTED,
				ReasonMessage: "skip reason",
			}
		}

		var legacyStatus pb.TestStatus
		var expected bool
		switch status {
		case pb.TestResult_PASSED:
			legacyStatus = pb.TestStatus_PASS
			expected = true
		case pb.TestResult_FAILED:
			legacyStatus = pb.TestStatus_FAIL
			expected = false
		case pb.TestResult_SKIPPED:
			legacyStatus = pb.TestStatus_SKIP
			expected = true
		case pb.TestResult_EXECUTION_ERRORED, pb.TestResult_PRECLUDED:
			legacyStatus = pb.TestStatus_SKIP
			expected = false
		}

		tvID, err := pbutil.ParseStructuredTestIdentifierForOutput(testID, v)
		if err != nil {
			panic(errors.Fmt("parse test variant identifier: %w", err))
		}

		trs[i] = &pb.TestResult{
			Name:             pbutil.TestResultName(invID, testID, resultID),
			TestId:           testID,
			ResultId:         resultID,
			TestIdStructured: tvID,
			Variant:          v,
			VariantHash:      pbutil.VariantHash(v),
			Expected:         expected,
			Status:           legacyStatus,
			StatusV2:         status,
			StartTime:        timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
			Duration:         &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
			SummaryHtml:      "SummaryHtml",
			Tags: []*pb.StringPair{
				{Key: "k1", Value: "v1"},
				{Key: "k2", Value: "v2"},
			},
			TestMetadata:  &pb.TestMetadata{Name: "testname"},
			FailureReason: failureReason,
			Properties:    &structpb.Struct{Fields: map[string]*structpb.Value{"key": {Kind: &structpb.Value_StringValue{StringValue: "value"}}}},
			SkipReason:    skipReason,
			SkippedReason: skippedReason,
			FrameworkExtensions: &pb.FrameworkExtensions{
				WebTest: &pb.WebTest{
					Status:     pb.WebTest_PASS,
					IsExpected: true,
				},
			},
		}
	}
	return trs
}

// MakeTestResult prepares legacy test results.
//
// The returned result represents what one would expect to read
// back for a test result stored 18 months ago. Fields that are
// set at upload time for new results may not be set for these results.
//
// These results should be passed to TestResultMessagesLegacy or
// asserted against service responses.
func MakeTestResultsLegacy(invID, testID string, v *pb.Variant, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)

		var reason *pb.FailureReason
		if status != pb.TestStatus_PASS && status != pb.TestStatus_SKIP {
			reason = &pb.FailureReason{
				PrimaryErrorMessage: "failure reason",
				Errors: []*pb.FailureReason_Error{
					{Message: "failure reason"},
				},
			}
		}
		var skipReason pb.SkipReason
		if status == pb.TestStatus_SKIP {
			skipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
		}
		tvID, err := pbutil.ParseStructuredTestIdentifierForOutput(testID, v)
		if err != nil {
			panic(errors.Fmt("parse test variant identifier: %w", err))
		}

		expected := status == pb.TestStatus_PASS

		// This implements the backfill applied to legacy data in b/410660296#comment22.
		var statusV2 pb.TestResult_Status
		if expected {
			if status == pb.TestStatus_SKIP {
				// Expected skip.
				statusV2 = pb.TestResult_SKIPPED
			} else {
				// Expected pass, fail, crash, abort.
				statusV2 = pb.TestResult_PASSED
			}
		} else {
			if status == pb.TestStatus_SKIP {
				// Unexpected skip.
				statusV2 = pb.TestResult_EXECUTION_ERRORED
			} else {
				// Unexpected pass, fail, crash, abort.
				statusV2 = pb.TestResult_FAILED
			}
		}

		trs[i] = &pb.TestResult{
			Name:             pbutil.TestResultName(invID, testID, resultID),
			TestId:           testID,
			ResultId:         resultID,
			TestIdStructured: tvID,
			Variant:          v,
			VariantHash:      pbutil.VariantHash(v),
			Expected:         status == pb.TestStatus_PASS,
			Status:           status,
			StatusV2:         statusV2,
			Duration:         &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
			SummaryHtml:      "SummaryHtml",
			TestMetadata:     &pb.TestMetadata{Name: "testname"},
			FailureReason:    reason,
			Properties:       &structpb.Struct{Fields: map[string]*structpb.Value{"key": {Kind: &structpb.Value_StringValue{StringValue: "value"}}}},
			SkipReason:       skipReason,
		}
	}
	return trs
}

// Checkpoint returns a Spanner mutation to insert an checkpoint.
func Checkpoint(ctx context.Context, project, resourceID, processID, uniquifier string) *spanner.Mutation {
	values := map[string]any{
		"Project":      project,
		"ResourceID":   resourceID,
		"ProcessID":    processID,
		"Uniquifier":   uniquifier,
		"ExpiryTime":   clock.Now(ctx).Add(time.Hour),
		"CreationTime": spanner.CommitTimestamp,
	}
	return spanutil.InsertMap("Checkpoints", values)
}

// MakeRootInvocation returns a root invocation row for testing.
func MakeRootInvocation(id rootinvocations.ID, state pb.RootInvocation_State) rootinvocations.RootInvocationRow {
	row := rootinvocations.RootInvocationRow{
		RootInvocationID: id,
		State:            state,
		Realm:            TestRealm,
		CreateTime:       time.Date(2025, 4, 25, 1, 2, 3, 4000, time.UTC),
		CreatedBy:        "user:test@example.com",

		// Spanner stores times in microsecond precision. Round times to this resolution to avoid roundtrip failures.
		Deadline:                                time.Date(2025, 4, 28, 1, 2, 3, 4000, time.UTC),
		UninterestingTestVerdictsExpirationTime: spanner.NullTime{Valid: true, Time: time.Date(2025, 6, 28, 1, 2, 3, 4000, time.UTC)},
		CreateRequestID:                         "test-request-id",
		ProducerResource:                        "//builds.example.com/builds/123",
		Tags:                                    pbutil.StringPairs("k1", "v1"),
		Properties: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		},
		Sources: &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "chromium.googlesource.com",
				Project:    "chromium/src",
				Ref:        "refs/heads/main",
				CommitHash: "1234567890abcdef1234567890abcdef12345678",
				Position:   123,
			},
		},
		IsSourcesFinal: true,
		BaselineID:     "baseline",
		Submitted:      false,
	}
	if state == pb.RootInvocation_FINALIZED || state == pb.RootInvocation_FINALIZING {
		row.FinalizeStartTime = spanner.NullTime{Valid: true, Time: time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)}
	}
	if state == pb.RootInvocation_FINALIZED {
		row.FinalizeTime = spanner.NullTime{Valid: true, Time: time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)}
	}
	return row
}

// RootInvocation inserts the rootInvocation record and all the
// RootInvocationShards records for a root invocation.
func RootInvocation(r rootinvocations.RootInvocationRow) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 16+1) // 16 shard and 1 root invocation
	ms = append(ms, spanutil.InsertMap("RootInvocations", map[string]any{
		"RootInvocationId":      r.RootInvocationID,
		"SecondaryIndexShardId": r.SecondaryIndexShardID,
		"State":                 r.State,
		"Realm":                 r.Realm,
		"CreateTime":            r.CreateTime,
		"CreatedBy":             r.CreatedBy,
		"FinalizeStartTime":     r.FinalizeStartTime,
		"FinalizeTime":          r.FinalizeTime,
		"Deadline":              r.Deadline,
		"UninterestingTestVerdictsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateRequestId":                         r.CreateRequestID,
		"ProducerResource":                        r.ProducerResource,
		"Tags":                                    r.Tags,
		"Properties":                              spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"Sources":                                 spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourcesFinal":                          r.IsSourcesFinal,
		"BaselineId":                              r.BaselineID,
		"Submitted":                               r.Submitted,
	}))

	for i := 0; i < 16; i++ {
		ms = append(ms, spanutil.InsertMap("RootInvocationShards", map[string]any{
			"RootInvocationShardId": rootinvocations.ShardID{RootInvocationID: r.RootInvocationID, ShardIndex: i},
			"ShardIndex":            i,
			"RootInvocationId":      r.RootInvocationID,
			"State":                 r.State,
			"Realm":                 r.Realm,
			"CreateTime":            r.CreateTime,
			"Sources":               spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
			"IsSourcesFinal":        r.IsSourcesFinal,
		}))
	}
	return ms
}
