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
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/spanner"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func updateDict(dest, source map[string]interface{}) {
	for k, v := range source {
		dest[k] = v
	}
}

// Invocation returns a spanner mutation that inserts an invocation.
func Invocation(id invocations.ID, state pb.Invocation_State, extraValues map[string]interface{}) *spanner.Mutation {
	future := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	values := map[string]interface{}{
		"InvocationId":                      id,
		"ShardId":                           0,
		"State":                             state,
		"Realm":                             "",
		"InvocationExpirationTime":          future,
		"ExpectedTestResultsExpirationTime": future,
		"CreateTime":                        spanner.CommitTimestamp,
		"Deadline":                          future,
	}

	if state == pb.Invocation_FINALIZED {
		values["FinalizeTime"] = spanner.CommitTimestamp
	}
	updateDict(values, extraValues)
	return spanutil.InsertMap("Invocations", values)
}

// FinalizedInvocationWithInclusions returns mutations to insert a finalized invocation with inclusions.
func FinalizedInvocationWithInclusions(id invocations.ID, extraValues map[string]interface{}, included ...invocations.ID) []*spanner.Mutation {
	return InvocationWithInclusions(id, pb.Invocation_FINALIZED, extraValues, included...)
}

// InvocationWithInclusions returns mutations to insert an invocation with inclusions.
func InvocationWithInclusions(id invocations.ID, state pb.Invocation_State, extraValues map[string]interface{}, included ...invocations.ID) []*spanner.Mutation {
	ms := []*spanner.Mutation{Invocation(id, state, extraValues)}
	for _, incl := range included {
		ms = append(ms, Inclusion(id, incl))
	}
	return ms
}

// Inclusion returns a spanner mutation that inserts an inclusion.
func Inclusion(including, included invocations.ID) *spanner.Mutation {
	return spanutil.InsertMap("IncludedInvocations", map[string]interface{}{
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
		mutMap := map[string]interface{}{
			"InvocationId":    invocations.ID(invID),
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         trs[i].Variant,
			"VariantHash":     pbutil.VariantHash(trs[i].Variant),
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          tr.Status,
			"RunDurationUsec": 1e6*i + 234567,
			"SummaryHtml":     spanutil.Compressed("SummaryHtml"),
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

		ms[i] = spanutil.InsertMap("TestResults", mutMap)
	}
	return ms
}

// TestExonerations returns Spanner mutations to insert test exonerations.
func TestExonerations(invID invocations.ID, testID string, variant *pb.Variant, reasons ...pb.ExonerationReason) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(reasons))
	for i := 0; i < len(reasons); i++ {
		ms[i] = spanutil.InsertMap("TestExonerations", map[string]interface{}{
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

// TestExonerationsLegacy returns Spanner mutations to insert test exonerations
// without reason (all exonerations inserted prior to ~May 2022).
func TestExonerationsLegacy(invID invocations.ID, testID string, variant *pb.Variant, count int) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, count)
	for i := 0; i < count; i++ {
		ms[i] = spanutil.InsertMap("TestExonerations", map[string]interface{}{
			"InvocationId":    invID,
			"TestId":          testID,
			"ExonerationId":   "legacy:" + strconv.Itoa(i),
			"Variant":         variant,
			"VariantHash":     pbutil.VariantHash(variant),
			"ExplanationHTML": spanutil.Compressed(fmt.Sprintf("legacy explanation %d", i)),
		})
	}
	return ms
}

// Artifact returns a Spanner mutation to insert an artifact.
func Artifact(invID invocations.ID, parentID, artID string, extraValues map[string]interface{}) *spanner.Mutation {
	values := map[string]interface{}{
		"InvocationId": invID,
		"ParentID":     parentID,
		"ArtifactId":   artID,
	}
	updateDict(values, extraValues)
	return spanutil.InsertMap("Artifacts", values)
}

// MakeTestResults creates test results.
func MakeTestResults(invID, testID string, v *pb.Variant, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)
		trs[i] = &pb.TestResult{
			Name:        pbutil.TestResultName(invID, testID, resultID),
			TestId:      testID,
			ResultId:    resultID,
			Variant:     v,
			VariantHash: pbutil.VariantHash(v),
			Expected:    status == pb.TestStatus_PASS,
			Status:      status,
			Duration:    &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
			SummaryHtml: "SummaryHtml",
		}
	}
	return trs
}
