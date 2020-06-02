// Copyright 2020 The LUCI Authors.
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
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"

	. "github.com/smartystreets/goconvey/convey"
)

func updateDict(dest, source map[string]interface{}) {
	for k, v := range source {
		dest[k] = v
	}
}

// Invocation returns a spanner mutation that inserts an invocation.
func Invocation(id span.InvocationID, state pb.Invocation_State, extraValues map[string]interface{}) *spanner.Mutation {
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
		"TestResultCount":                   0,
	}

	if state == pb.Invocation_FINALIZED {
		values["FinalizeTime"] = spanner.CommitTimestamp
	}
	updateDict(values, extraValues)
	return span.InsertMap("Invocations", values)
}

// FinalizedInvocationWithInclusions returns mutations to insert a finalized invocation with inclusions.
func FinalizedInvocationWithInclusions(id span.InvocationID, included ...span.InvocationID) []*spanner.Mutation {
	return InvocationWithInclusions(id, pb.Invocation_FINALIZED, included...)
}

// InvocationWithInclusions returns mutations to insert an invocation with inclusions.
func InvocationWithInclusions(id span.InvocationID, state pb.Invocation_State, included ...span.InvocationID) []*spanner.Mutation {
	ms := []*spanner.Mutation{Invocation(id, state, nil)}
	for _, incl := range included {
		ms = append(ms, Inclusion(id, incl))
	}
	return ms
}

// Inclusion returns a spanner mutation that inserts an inclusion.
func Inclusion(including, included span.InvocationID) *spanner.Mutation {
	return span.InsertMap("IncludedInvocations", map[string]interface{}{
		"InvocationId":         including,
		"IncludedInvocationId": included,
	})
}

// TestResults returns spanner mutations to insert test results
func TestResults(invID, testID string, v *typepb.Variant, statuses ...pb.TestStatus) []*spanner.Mutation {
	return TestResultMessages(MakeTestResults(invID, testID, v, statuses...))
}

// TestResultMessages returns spanner mutations to insert test results
func TestResultMessages(trs []*pb.TestResult) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testID, resultID, err := pbutil.ParseTestResultName(tr.Name)
		So(err, ShouldBeNil)
		mutMap := map[string]interface{}{
			"InvocationId":    span.InvocationID(invID),
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         trs[i].Variant,
			"VariantHash":     pbutil.VariantHash(trs[i].Variant),
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          tr.Status,
			"RunDurationUsec": 1e6*i + 234567,
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}

		ms[i] = span.InsertMap("TestResults", mutMap)
	}
	return ms
}

// TestExonerations returns Spanner mutations to insert test exonerations.
func TestExonerations(invID span.InvocationID, testID string, variant *typepb.Variant, count int) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, count)
	for i := 0; i < count; i++ {
		ms[i] = span.InsertMap("TestExonerations", map[string]interface{}{
			"InvocationId":    invID,
			"TestId":          testID,
			"ExonerationId":   strconv.Itoa(i),
			"Variant":         variant,
			"VariantHash":     pbutil.VariantHash(variant),
			"ExplanationHTML": span.Compressed(fmt.Sprintf("explanation %d", i)),
		})
	}
	return ms
}

// Artifact returns a Spanner mutation to insert an artifact.
func Artifact(invID span.InvocationID, parentID, artID string, extraValues map[string]interface{}) *spanner.Mutation {
	values := map[string]interface{}{
		"InvocationId": invID,
		"ParentID":     parentID,
		"ArtifactId":   artID,
	}
	updateDict(values, extraValues)
	return span.InsertMap("Artifacts", values)
}

// MakeTestResults creates test results.
func MakeTestResults(invID, testID string, v *typepb.Variant, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	for i, status := range statuses {
		resultID := fmt.Sprintf("%d", i)
		trs[i] = &pb.TestResult{
			Name:     pbutil.TestResultName(invID, testID, resultID),
			TestId:   testID,
			ResultId: resultID,
			Variant:  v,
			Expected: status == pb.TestStatus_PASS,
			Status:   status,
			Duration: &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
		}
	}
	return trs
}
