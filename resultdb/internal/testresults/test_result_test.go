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

package testresults

import (
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	durpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

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

func TestMustParseName(t *testing.T) {
	t.Parallel()

	ftt.Run("MustParseName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			invID, testID, resultID := MustParseName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
			assert.Loosely(t, invID, should.Equal(invocations.ID("a")))
			assert.Loosely(t, testID, should.Equal("ninja://chrome/test:foo_tests/BarTest.DoBaz"))
			assert.Loosely(t, resultID, should.Equal("result5"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			invalidNames := []string{
				"invocations/a/tests/b",
				"invocations/a/tests/b/exonerations/c",
			}
			for _, name := range invalidNames {
				assert.Loosely(t, func() { MustParseName(name) }, should.Panic)
			}
		})
	})
}

func TestRead(t *testing.T) {
	ftt.Run(`Read`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		invID := invocations.ID("inv")
		propertiesBytes, err := proto.Marshal(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		})
		assert.Loosely(t, err, should.BeNil)

		// Insert a TestResult.
		testutil.MustApply(ctx, t,
			insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
			spanutil.InsertMap("TestResults", map[string]any{
				"InvocationId":    invID,
				"TestId":          "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
				"ResultId":        "r",
				"Variant":         pbutil.Variant("k1", "v1", "k2", "v2"),
				"VariantHash":     "deadbeef",
				"CommitTimestamp": spanner.CommitTimestamp,
				"IsUnexpected":    true,
				"Status":          pb.TestStatus_CRASH,
				"StatusV2":        pb.TestResult_FAILED,
				"RunDurationUsec": 1234567,
				"Properties":      spanutil.Compressed(propertiesBytes),
			}),
		)

		const name = "invocations/inv/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/r"
		tr, err := Read(span.Single(ctx), name)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tr, should.Match(&pb.TestResult{
			Name: name,
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:        "//infra/junit_tests",
				ModuleScheme:      "junit",
				ModuleVariant:     pbutil.Variant("k1", "v1", "k2", "v2"),
				ModuleVariantHash: "68d82cb978092fc7",
				CoarseName:        "org.chromium.go.luci",
				FineName:          "ValidationTests",
				CaseName:          "FooBar",
			},
			TestId:      "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
			ResultId:    "r",
			Variant:     pbutil.Variant("k1", "v1", "k2", "v2"),
			Expected:    false,
			Status:      pb.TestStatus_CRASH,
			StatusV2:    pb.TestResult_FAILED,
			Duration:    &durpb.Duration{Seconds: 1, Nanos: 234567000},
			VariantHash: "deadbeef",
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("value"),
				},
			},
		}))
	})
}
