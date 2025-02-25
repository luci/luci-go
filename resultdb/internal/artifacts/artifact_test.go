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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestMustParseName(t *testing.T) {
	t.Parallel()

	ftt.Run(`MustParseName`, t, func(t *ftt.Test) {
		t.Run(`Parse`, func(t *ftt.Test) {
			invID, testID, resultID, artifactID := MustParseName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5/artifacts/a")
			assert.Loosely(t, invID, should.Equal(invocations.ID("a")))
			assert.Loosely(t, testID, should.Equal("ninja://chrome/test:foo_tests/BarTest.DoBaz"))
			assert.Loosely(t, resultID, should.Equal("result5"))
			assert.Loosely(t, artifactID, should.Equal("a"))
		})

		t.Run(`Invalid`, func(t *ftt.Test) {
			invalidNames := []string{
				"invocations/a/tests/b",
				"invocations/a/tests/b/exonerations/c",
			}
			for _, name := range invalidNames {
				name := name
				assert.Loosely(t, func() { MustParseName(name) }, should.Panic)
			}
		})
	})
}

func TestParentID(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestParentID`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			id := ParentID("", "")
			assert.Loosely(t, id, should.BeEmpty)
		})
		t.Run(`Test result level`, func(t *ftt.Test) {
			id := ParentID("t t", "r")
			assert.Loosely(t, id, should.Equal("tr/t t/r"))
		})
	})
}

func TestRead(t *testing.T) {
	ftt.Run(`TestRead`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

		t.Run(`Does not exist`, func(t *ftt.Test) {
			_, err := Read(ctx, "invocations/i/artifacts/a")
			as, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, as.Code(), should.Equal(codes.NotFound))
			assert.That(t, as.Message(), should.ContainSubstring("invocations/i/artifacts/a not found"))
		})

		t.Run(`Exists`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Artifact("inv", "tr/t/r", "a", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
			}))
			const name = "invocations/inv/tests/t/results/r/artifacts/a"
			a, err := Read(ctx, name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Artifact, should.Match(&pb.Artifact{
				Name:        name,
				ArtifactId:  "a",
				ContentType: "text/plain",
				SizeBytes:   54,
			}))
		})

		t.Run(`Exists with GcsURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Artifact("inv", "tr/t/r", "b", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
				"GcsURI":      "gs://test",
			}))
			const name = "invocations/inv/tests/t/results/r/artifacts/b"
			a, err := Read(ctx, name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Artifact, should.Match(&pb.Artifact{
				Name:        name,
				ArtifactId:  "b",
				ContentType: "text/plain",
				SizeBytes:   54,
				GcsUri:      "gs://test",
			}))
		})
	})
}
