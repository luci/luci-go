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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRead(t *testing.T) {
	ftt.Run(`Read`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		invID := invocations.ID("inv")

		t.Run("Full", func(t *ftt.Test) {
			// Insert a TestExoneration.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
				spanutil.InsertMap("TestExonerations", map[string]any{
					"InvocationId":    invID,
					"TestId":          "t t",
					"ExonerationId":   "id",
					"Variant":         pbutil.Variant("k1", "v1", "k2", "v2"),
					"VariantHash":     "deadbeef",
					"ExplanationHTML": spanutil.Compressed("broken"),
					"Reason":          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				}))

			const name = "invocations/inv/tests/t%20t/exonerations/id"
			ex, err := Read(span.Single(ctx), name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ex, should.Resemble(&pb.TestExoneration{
				Name:            name,
				ExonerationId:   "id",
				TestId:          "t t",
				Variant:         pbutil.Variant("k1", "v1", "k2", "v2"),
				ExplanationHtml: "broken",
				VariantHash:     "deadbeef",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			}))
		})
		t.Run("Minimal", func(t *ftt.Test) {
			// Insert a TestExoneration without reason. This was only possible
			// prior to May 2022. This test case can be deleted from November 2023.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
				spanutil.InsertMap("TestExonerations", map[string]any{
					"InvocationId":  invID,
					"TestId":        "t t",
					"ExonerationId": "id",
					"Variant":       pbutil.Variant("k1", "v1", "k2", "v2"),
					"VariantHash":   "deadbeef",
					"Reason":        pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED,
					// Leave ExplanationHTML as null.
				}))

			const name = "invocations/inv/tests/t%20t/exonerations/id"
			ex, err := Read(span.Single(ctx), name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ex, should.Resemble(&pb.TestExoneration{
				Name:            name,
				ExonerationId:   "id",
				TestId:          "t t",
				Variant:         pbutil.Variant("k1", "v1", "k2", "v2"),
				ExplanationHtml: "",
				VariantHash:     "deadbeef",
				Reason:          pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED,
			}))
		})
	})
}
