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

package spantest

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMustParseArtifactName(t *testing.T) {
	t.Parallel()

	Convey(`MustParseArtifactName`, t, func() {
		Convey(`Parse`, func() {
			invID, testID, resultID, artifactID := span.MustParseArtifactName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5/artifacts/a")
			So(invID, ShouldEqual, "a")
			So(testID, ShouldEqual, "ninja://chrome/test:foo_tests/BarTest.DoBaz")
			So(resultID, ShouldEqual, "result5")
			So(artifactID, ShouldEqual, "a")
		})

		Convey(`Invalid`, func() {
			invalidNames := []string{
				"invocations/a/tests/b",
				"invocations/a/tests/b/exonerations/c",
			}
			for _, name := range invalidNames {
				name := name
				So(func() { span.MustParseArtifactName(name) }, ShouldPanic)
			}
		})
	})
}

func TestArtifactParentID(t *testing.T) {
	t.Parallel()
	Convey(`TestArtifactParentID`, t, func() {
		Convey(`Invocation level`, func() {
			id := span.ArtifactParentID("", "")
			So(id, ShouldEqual, "")
		})
		Convey(`Test result level`, func() {
			id := span.ArtifactParentID("t t", "r")
			So(id, ShouldEqual, "tr/t t/r")
		})
	})
}

func TestReadArtifact(t *testing.T) {
	Convey(`TestReadArtifact`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_FINALIZED, nil))

		Convey(`Does not exist`, func() {
			_, err := span.ReadArtifact(ctx, txn, "invocations/i/artifacts/a")
			So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/i/artifacts/a not found")
		})

		Convey(`Exists`, func() {
			testutil.MustApply(ctx, testutil.InsertTestResultArtifact("inv", "t", "r", "a", map[string]interface{}{
				"ContentType": "text/plain",
				"Size":        "54",
			}))
			const name = "invocations/inv/tests/t/results/r/artifacts/a"
			a, err := span.ReadArtifact(ctx, txn, name)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &pb.Artifact{
				Name:        name,
				ArtifactId:  "a",
				ContentType: "text/plain",
				SizeBytes:   54,
			})
		})
	})
}
