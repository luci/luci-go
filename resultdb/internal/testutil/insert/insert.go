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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	durpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
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

// TestResults returns spanner mutations to insert test results
func TestResults(invID, testID string, v *pb.Variant, statuses ...pb.TestStatus) []*spanner.Mutation {
	return TestResultMessages(MakeTestResults(invID, testID, v, statuses...))
}

// TestResultMessages returns spanner mutations to insert test results
func TestResultMessages(trs []*pb.TestResult) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testID, resultID, err := pbutil.ParseTestResultName(tr.Name)
		So(err, ShouldBeNil)

		mutMap := map[string]any{
			"InvocationId":    invocations.ID(invID),
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         tr.Variant,
			"VariantHash":     pbutil.VariantHash(tr.Variant),
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          tr.Status,
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
			So(err, ShouldBeNil)
			mutMap["TestMetadata"] = spanutil.Compressed(tmdBytes)
		}
		if tr.FailureReason != nil {
			frBytes, err := proto.Marshal(tr.FailureReason)
			So(err, ShouldBeNil)
			mutMap["FailureReason"] = spanutil.Compressed(frBytes)
		}
		if tr.Properties != nil {
			propertiesBytes, err := proto.Marshal(tr.Properties)
			So(err, ShouldBeNil)
			mutMap["Properties"] = spanutil.Compressed(propertiesBytes)
		}

		ms[i] = spanutil.InsertMap("TestResults", mutMap)
	}
	return ms
}

// TestExonerations returns Spanner mutations to insert test exonerations.
func TestExonerations(invID invocations.ID, testID string, variant *pb.Variant, reasons ...pb.ExonerationReason) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(reasons))
	for i := 0; i < len(reasons); i++ {
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

// MakeTestResults creates test results.
func MakeTestResults(invID, testID string, v *pb.Variant, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)

		var reason *pb.FailureReason
		if status != pb.TestStatus_PASS && status != pb.TestStatus_SKIP {
			reason = &pb.FailureReason{
				PrimaryErrorMessage: "failure reason",
			}
		}
		var skipReason pb.SkipReason
		if status == pb.TestStatus_SKIP {
			skipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
		}

		trs[i] = &pb.TestResult{
			Name:          pbutil.TestResultName(invID, testID, resultID),
			TestId:        testID,
			ResultId:      resultID,
			Variant:       v,
			VariantHash:   pbutil.VariantHash(v),
			Expected:      status == pb.TestStatus_PASS,
			Status:        status,
			Duration:      &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
			SummaryHtml:   "SummaryHtml",
			TestMetadata:  &pb.TestMetadata{Name: "testname"},
			FailureReason: reason,
			Properties:    &structpb.Struct{Fields: map[string]*structpb.Value{"key": {Kind: &structpb.Value_StringValue{StringValue: "value"}}}},
			SkipReason:    skipReason,
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
