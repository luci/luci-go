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
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTryClaimInvocations(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		Convey(`Claims unclaimed invocations and invocations already claimed by this root invocation`, func() {
			// Insert some values into spanner.
			mutations := []*spanner.Mutation{
				invocationMutation("chromium", "inv-1", "build-8001"),
				invocationMutation("chromeos", "inv-2", "build-8002"),
				invocationMutation("chromium", "inv-3", "build-8003"),
				invocationMutation("chromium", "inv-4", "build-8000"),
			}
			testutil.MustApply(ctx, mutations...)

			claimed, err := tryClaimInvocations(span.Single(ctx), "chromium", "build-8000", []string{"inv-1", "inv-2", "inv-3", "inv-4"})
			So(err, ShouldBeNil)
			So(claimed, ShouldResemble, map[string]bool{
				"inv-2": true,
				"inv-4": true,
			})
		})
		Convey(`Claims an empty list of invocations`, func() {
			claimed, err := tryClaimInvocations(span.Single(ctx), "chromium", "build-8000", []string{})
			So(err, ShouldBeNil)
			So(claimed, ShouldResemble, map[string]bool{})
		})
	})
}

func invocationMutation(project string, invID string, rootInvID string) *spanner.Mutation {
	return spanner.Insert(
		"Invocations",
		[]string{"Project", "InvocationID", "IngestedInvocationID", "CreationTime"},
		[]any{
			project,
			invID,
			rootInvID,
			spanner.CommitTimestamp,
		},
	)
}
