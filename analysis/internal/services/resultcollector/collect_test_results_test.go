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

package resultcollector

import (
	"fmt"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal"
	"go.chromium.org/luci/analysis/internal/resultdb"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testutil/insert"
	"go.chromium.org/luci/analysis/pbutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
)

type verdict struct {
	realm           string
	testID          string
	variantHash     string
	invID           string
	exonerated      bool
	status          internal.VerdictStatus
	unexpectedCount int64
	totalCount      int64
}

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
		ctx, skdr := tq.TestingContext(testutil.TestingContext(), nil)
		RegisterTaskClass()

		inv := &rdbpb.Invocation{
			Name:  "invocations/build-87654321",
			Realm: "chromium:ci",
		}
		task := &taskspb.CollectTestResults{
			Resultdb: &taskspb.ResultDB{
				Invocation: inv,
				Host:       "results.api.cr.dev",
			},
			Project:                   "Project",
			Builder:                   "Linux Tests",
			IsPreSubmit:               false,
			ContributedToClSubmission: false,
		}

		expectedTask := proto.Clone(task).(*taskspb.CollectTestResults)
		So(Schedule(ctx, task), ShouldBeNil)
		So(skdr.Tasks().Payloads()[0], ShouldResembleProto, expectedTask)
	})
}

func TestSaveVerdicts(t *testing.T) {
	Convey(`TestSaveVerdicts`, t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()

		mrc := resultdb.NewMockedClient(testutil.IntegrationTestContext(t), ctl)
		ctx := mrc.Ctx

		realm := "chromium:ci"
		builder := "builder"
		vh := "variant_hash"
		builderField := map[string]interface{}{
			"Builder": builder,
		}
		// Prepare some analyzed test variants to query.
		ms := []*spanner.Mutation{
			insert.AnalyzedTestVariant(realm, "ninja://test_known_flake", vh, atvpb.Status_FLAKY, builderField),
			insert.AnalyzedTestVariant(realm, "ninja://test_has_unexpected", vh, atvpb.Status_HAS_UNEXPECTED_RESULTS, builderField),
			insert.AnalyzedTestVariant(realm, "ninja://test_consistent_failure", vh, atvpb.Status_CONSISTENTLY_UNEXPECTED, builderField),
			// Stale test variant has new failure.
			insert.AnalyzedTestVariant(realm, "ninja://test_no_new_results", vh, atvpb.Status_NO_NEW_RESULTS, builderField),
			// Flaky test variant on another builder.
			insert.AnalyzedTestVariant(realm, "ninja://test_known_flake", "another_hash", atvpb.Status_FLAKY, map[string]interface{}{
				"Builder": "another_builder",
			}),
		}
		testutil.MustApply(ctx, ms...)

		invID := "build-87654321"
		invName := fmt.Sprintf("invocations/%s", invID)
		req := &rdbpb.BatchGetTestVariantsRequest{
			Invocation: invName,
			TestVariants: []*rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier{
				{
					TestId:      "ninja://test_consistent_failure",
					VariantHash: vh,
				},
				{
					TestId:      "ninja://test_has_unexpected",
					VariantHash: vh,
				},
				{
					TestId:      "ninja://test_known_flake",
					VariantHash: vh,
				},
			},
		}
		mrc.BatchGetTestVariants(req, mockedBatchGetTestVariantsResponse())

		inv := &rdbpb.Invocation{
			Name:       invName,
			Realm:      realm,
			CreateTime: pbutil.MustTimestampProto(testclock.TestRecentTimeUTC),
		}
		task := &taskspb.CollectTestResults{
			Resultdb: &taskspb.ResultDB{
				Invocation: inv,
				Host:       "results.api.cr.dev",
			},
			Builder:                   builder,
			IsPreSubmit:               false,
			ContributedToClSubmission: false,
		}
		err := collectTestResults(ctx, task)
		So(err, ShouldBeNil)

		// Read verdicts to confirm they are saved.
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		ks := spanner.KeySets(
			spanner.Key{realm, "ninja://test_known_flake", vh, invID},
			spanner.Key{realm, "ninja://test_consistent_failure", vh, invID},
			spanner.Key{realm, "ninja://test_has_unexpected", vh, invID},
		)
		expected := map[string]verdict{
			"ninja://test_known_flake": {
				realm:           realm,
				testID:          "ninja://test_known_flake",
				variantHash:     vh,
				invID:           invID,
				exonerated:      false,
				status:          internal.VerdictStatus_VERDICT_FLAKY,
				unexpectedCount: 1,
				totalCount:      2,
			},
			"ninja://test_consistent_failure": {
				realm:           realm,
				testID:          "ninja://test_consistent_failure",
				variantHash:     vh,
				invID:           invID,
				exonerated:      true,
				status:          internal.VerdictStatus_UNEXPECTED,
				unexpectedCount: 1,
				totalCount:      1,
			},
			"ninja://test_has_unexpected": {
				realm:           realm,
				testID:          "ninja://test_has_unexpected",
				variantHash:     vh,
				invID:           invID,
				exonerated:      false,
				status:          internal.VerdictStatus_EXPECTED,
				unexpectedCount: 0,
				totalCount:      1,
			},
		}

		fields := []string{"Realm", "TestId", "VariantHash", "InvocationId", "Exonerated", "Status", "UnexpectedResultCount", "TotalResultCount"}
		total := 0
		var b spanutil.Buffer
		err = span.Read(ctx, "Verdicts", ks, fields).Do(
			func(row *spanner.Row) error {
				var v verdict
				err = b.FromSpanner(row, &v.realm, &v.testID, &v.variantHash, &v.invID, &v.exonerated, &v.status, &v.unexpectedCount, &v.totalCount)
				So(err, ShouldBeNil)
				total++

				exp, ok := expected[v.testID]
				So(ok, ShouldBeTrue)
				So(v, ShouldResemble, exp)
				return nil
			},
		)
		So(err, ShouldBeNil)
		So(total, ShouldEqual, 3)

	})
}
