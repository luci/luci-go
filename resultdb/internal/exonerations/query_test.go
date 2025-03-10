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
	"context"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQueryTestExonerations(t *testing.T) {
	ftt.Run(`QueryTestExonerations`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.FinalizedInvocationWithInclusions("a", nil),
			insert.FinalizedInvocationWithInclusions("b", nil),
			insert.TestExonerations("a", ":module!junit:org.chromium.foo:BarTest#A", pbutil.Variant("v", "a"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS, pb.ExonerationReason_NOT_CRITICAL),
			insert.TestExonerations("b", ":module!junit:org.chromium.foo:BarTest#C", pbutil.Variant("v", "c"), pb.ExonerationReason_UNEXPECTED_PASS),
		)...)

		q := &Query{
			InvocationIDs: invocations.NewIDSet("a", "b"),
			PageSize:      100,
		}
		actual, _, err := q.Fetch(span.Single(ctx))
		assert.Loosely(t, err, should.BeNil)
		sort.Slice(actual, func(i, j int) bool {
			return actual[i].Name < actual[j].Name
		})
		assert.Loosely(t, actual, should.Match([]*pb.TestExoneration{
			{
				Name:   "invocations/a/tests/:module%21junit:org.chromium.foo:BarTest%23A/exonerations/0",
				TestId: ":module!junit:org.chromium.foo:BarTest#A",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("v", "a"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("v", "a")),
					CoarseName:        "org.chromium.foo",
					FineName:          "BarTest",
					CaseName:          "A",
				},
				Variant:         pbutil.Variant("v", "a"),
				ExonerationId:   "0",
				ExplanationHtml: "explanation 0",
				VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "a")),
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			},
			{
				Name:   "invocations/a/tests/:module%21junit:org.chromium.foo:BarTest%23A/exonerations/1",
				TestId: ":module!junit:org.chromium.foo:BarTest#A",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("v", "a"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("v", "a")),
					CoarseName:        "org.chromium.foo",
					FineName:          "BarTest",
					CaseName:          "A",
				},
				Variant:         pbutil.Variant("v", "a"),
				ExonerationId:   "1",
				ExplanationHtml: "explanation 1",
				VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "a")),
				Reason:          pb.ExonerationReason_NOT_CRITICAL,
			},
			{
				Name:   "invocations/b/tests/:module%21junit:org.chromium.foo:BarTest%23C/exonerations/0",
				TestId: ":module!junit:org.chromium.foo:BarTest#C",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("v", "c"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("v", "c")),
					CoarseName:        "org.chromium.foo",
					FineName:          "BarTest",
					CaseName:          "C",
				},
				Variant:         pbutil.Variant("v", "c"),
				ExonerationId:   "0",
				ExplanationHtml: "explanation 0",
				VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "c")),
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			},
		}))
	})
}

func TestToLimitedData(t *testing.T) {
	ctx := context.Background()

	ftt.Run(`ToLimitedData masks fields`, t, func(t *ftt.Test) {
		invocationID := "inv0"
		testID := "://chrome/test:foo_tests!junit:org.chromium.foo:BarTest#Foo"
		exonerationID := "123"
		name := pbutil.TestExonerationName(invocationID, testID, exonerationID)
		variant := pbutil.Variant("v", "a")
		variantHash := pbutil.VariantHash(variant)

		testExoneration := &pb.TestExoneration{
			Name:   name,
			TestId: testID,
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:        "//chrome/test:foo_tests",
				ModuleScheme:      "junit",
				ModuleVariant:     pbutil.Variant("v", "a"),
				ModuleVariantHash: variantHash,
				CoarseName:        "org.chromium.foo",
				FineName:          "BarTest",
				CaseName:          "Foo",
			},
			Variant:         variant,
			ExonerationId:   exonerationID,
			ExplanationHtml: "explanation 0",
			VariantHash:     variantHash,
			Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
		}

		expected := &pb.TestExoneration{
			Name:   name,
			TestId: testID,
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:        "//chrome/test:foo_tests",
				ModuleScheme:      "junit",
				ModuleVariantHash: variantHash,
				CoarseName:        "org.chromium.foo",
				FineName:          "BarTest",
				CaseName:          "Foo",
			},
			ExonerationId:   exonerationID,
			ExplanationHtml: "explanation 0",
			VariantHash:     variantHash,
			Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			IsMasked:        true,
		}

		err := ToLimitedData(ctx, testExoneration)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, testExoneration, should.Match(expected))
	})
}
