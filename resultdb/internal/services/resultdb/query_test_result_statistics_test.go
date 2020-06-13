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

package resultdb

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQueryTestResultStatistics(t *testing.T) {
	Convey(`QueryTestResultStatistics`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx,
			insert.Invocation("a", pb.Invocation_FINALIZED, nil),
			insert.Invocation(
				"b", pb.Invocation_FINALIZED, map[string]interface{}{
					"TestResultCount": 5,
				}),
			insert.Invocation(
				"c", pb.Invocation_FINALIZED, map[string]interface{}{
					"TestResultCount": 10,
				}),
			insert.Invocation(
				"d", pb.Invocation_FINALIZED, map[string]interface{}{
					"TestResultCount": 20,
				}),
			insert.Inclusion("a", "b"),
			insert.Inclusion("a", "c"),
			// To test no double count if an invocation is included by multiple
			// invocations.
			insert.Inclusion("b", "d"),
			insert.Inclusion("c", "d"),
		)

		srv := newTestResultDBService()
		res, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
			Invocations: []string{"invocations/a"},
		})
		So(err, ShouldBeNil)
		So(res.TotalTestResults, ShouldEqual, 35)
	})

}
