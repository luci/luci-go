// Copyright 2023 The LUCI Authors.
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

package changepoints

import (
	"fmt"
	"sort"
	"testing"

	"cloud.google.com/go/spanner"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDuplicateRun(t *testing.T) {
	Convey(`Can generate duplicate map`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		// Insert some values into spanner.
		mutations := []*spanner.Mutation{
			invocationMutation("chromium", "inv-1", 8001),
			invocationMutation("chromeos", "inv-2", 8002),
			invocationMutation("chromium", "inv-3", 8003),
			invocationMutation("chromium", "inv-4", 8000),
		}
		testutil.MustApply(ctx, mutations...)
		tvs := []*rdbpb.TestVariant{
			{
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name: "invocations/inv-1/tests/abc",
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name: "invocations/inv-2/tests/abc",
						},
					},
				},
			},
			{
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name: "invocations/inv-3/tests/abc",
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name: "invocations/inv-4/tests/abc",
						},
					},
				},
			},
		}

		buildResult := &controlpb.BuildResult{
			Id:      8000,
			Project: "chromium",
		}

		m, nonDups, err := readDuplicateInvocations(span.Single(ctx), tvs, buildResult)
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string]bool{
			"inv-1": true,
			"inv-3": true,
		})
		sort.Strings(nonDups)
		So(nonDups, ShouldResemble, []string{"inv-2"})
	})
}

func invocationMutation(project string, invID string, bbid int64) *spanner.Mutation {
	return spanner.Insert(
		"Invocations",
		[]string{"Project", "InvocationID", "IngestedInvocationID", "CreationTime"},
		[]any{
			project,
			invID,
			fmt.Sprintf("build-%d", bbid),
			spanner.CommitTimestamp,
		},
	)
}
