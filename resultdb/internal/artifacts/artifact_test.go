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

package artifacts

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMustParseName(t *testing.T) {
	t.Parallel()

	Convey(`MustParseName`, t, func() {
		Convey(`Parse`, func() {
			invID, testID, resultID, artifactID := MustParseName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5/artifacts/a")
			So(invID, ShouldEqual, invocations.ID("a"))
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
				So(func() { MustParseName(name) }, ShouldPanic)
			}
		})
	})
}

func TestParentID(t *testing.T) {
	t.Parallel()
	Convey(`TestParentID`, t, func() {
		Convey(`Invocation level`, func() {
			id := ParentID("", "")
			So(id, ShouldEqual, "")
		})
		Convey(`Test result level`, func() {
			id := ParentID("t t", "r")
			So(id, ShouldEqual, "tr/t t/r")
		})
	})
}

func TestRead(t *testing.T) {
	Convey(`TestRead`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

		Convey(`Does not exist`, func() {
			_, err := Read(ctx, "invocations/i/artifacts/a")
			So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/i/artifacts/a not found")
		})

		Convey(`Exists`, func() {
			testutil.MustApply(ctx, insert.Artifact("inv", "tr/t/r", "a", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
			}))
			const name = "invocations/inv/tests/t/results/r/artifacts/a"
			a, err := Read(ctx, name)
			So(err, ShouldBeNil)
			So(a.Artifact, ShouldResembleProto, &pb.Artifact{
				Name:        name,
				ArtifactId:  "a",
				ContentType: "text/plain",
				SizeBytes:   54,
			})
		})

		Convey(`Exists with GcsURI`, func() {
			testutil.MustApply(ctx, insert.Artifact("inv", "tr/t/r", "b", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
				"GcsURI":      "gs://test",
			}))
			const name = "invocations/inv/tests/t/results/r/artifacts/b"
			a, err := Read(ctx, name)
			So(err, ShouldBeNil)
			So(a.Artifact, ShouldResembleProto, &pb.Artifact{
				Name:        name,
				ArtifactId:  "b",
				ContentType: "text/plain",
				SizeBytes:   54,
				GcsUri:      "gs://test",
			})
		})
	})
}
