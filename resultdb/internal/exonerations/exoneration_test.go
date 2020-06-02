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

package exonerations

import (
	"testing"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRead(t *testing.T) {
	Convey(`Read`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		invID := span.InvocationID("inv")
		// Insert a TestExoneration.
		testutil.MustApply(ctx,
			insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
			span.InsertMap("TestExonerations", map[string]interface{}{
				"InvocationId":    invID,
				"TestId":          "t t",
				"ExonerationId":   "id",
				"Variant":         pbutil.Variant("k1", "v1", "k2", "v2"),
				"VariantHash":     "deadbeef",
				"ExplanationHTML": span.Compressed("broken"),
			}))

		const name = "invocations/inv/tests/t%20t/exonerations/id"
		ex, err := Read(ctx, span.Client(ctx).Single(), name)
		So(err, ShouldBeNil)
		So(ex, ShouldResembleProto, &pb.TestExoneration{
			Name:            name,
			ExonerationId:   "id",
			TestId:          "t t",
			Variant:         pbutil.Variant("k1", "v1", "k2", "v2"),
			ExplanationHtml: "broken",
		})
	})
}
