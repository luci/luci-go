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

package baselinetestvariant

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestRead(t *testing.T) {
	Convey(`Invalid`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Not Found`, func() {
			_, err := Read(span.Single(ctx), "chromium", "try:linux-rel", "ninja://some/test:test_id", "12345")
			So(err, ShouldErrLike, NotFound)
		})

	})

	Convey(`Valid`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Exists`, func() {
			exp := &BaselineTestVariant{
				Project:     "chromium",
				BaselineID:  "try:linux-rel",
				TestID:      "ninja://some/test:test_id",
				VariantHash: "12345",
			}
			commitTime := testutil.MustApply(ctx, Create(exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash))

			res, err := Read(span.Single(ctx), exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash)
			So(err, ShouldBeNil)
			So(res.Project, ShouldEqual, exp.Project)
			So(res.BaselineID, ShouldEqual, exp.BaselineID)
			So(res.TestID, ShouldEqual, exp.TestID)
			So(res.VariantHash, ShouldEqual, exp.VariantHash)
			So(res.LastUpdated, ShouldEqual, commitTime)
		})
	})
}
