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
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestMustParseName(t *testing.T) {
	t.Parallel()

	ftt.Run(`MustParseName`, t, func(t *ftt.Test) {
		t.Run(`Parse`, func(t *ftt.Test) {
			invID, testID, resultID, artifactID := MustParseLegacyName(
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
				assert.Loosely(t, func() { MustParseLegacyName(name) }, should.Panic)
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

		wuID := workunits.ID{RootInvocationID: "root-inv-id", WorkUnitID: "work-unit-id"}
		wu := workunits.NewBuilder("root-inv-id", "work-unit-id").Build()
		ms := insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder("root-inv-id").Build())
		ms = append(ms, insert.WorkUnit(wu)...)
		ms = append(ms, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
		testutil.MustApply(ctx, t, ms...)

		t.Run(`Does not exist`, func(t *ftt.Test) {
			_, err := Read(ctx, "rootInvocations/i/workUnits/wu1/artifacts/a")
			as, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, as.Code(), should.Equal(codes.NotFound))
			assert.That(t, as.Message(), should.ContainSubstring("rootInvocations/i/workUnits/wu1/artifacts/a not found"))
		})

		t.Run(`Does not exist (legacy name)`, func(t *ftt.Test) {
			_, err := Read(ctx, "invocations/i/artifacts/a")
			as, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, as.Code(), should.Equal(codes.NotFound))
			assert.That(t, as.Message(), should.ContainSubstring("invocations/i/artifacts/a not found"))
		})

		t.Run(`Exists`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Artifact(wuID.LegacyInvocationID(), "tr/t/r", "a", map[string]any{
				"ContentType":  "text/plain",
				"Size":         "54",
				"ArtifactType": "COVERAGE_REPORT",
			}))
			const name = "rootInvocations/root-inv-id/workUnits/work-unit-id/tests/t/results/r/artifacts/a"
			a, err := Read(ctx, name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Artifact, should.Match(&pb.Artifact{
				Name: name,
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "legacy",
					ModuleScheme:      "legacy",
					ModuleVariant:     &pb.Variant{},
					ModuleVariantHash: pbutil.VariantHash(&pb.Variant{}),
					CaseName:          "t",
				},
				TestId:       "t",
				ResultId:     "r",
				ArtifactId:   "a",
				ContentType:  "text/plain",
				SizeBytes:    54,
				ArtifactType: "COVERAGE_REPORT",
				HasLines:     true,
			}))
		})

		t.Run(`Exists with GcsURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Artifact(wuID.LegacyInvocationID(), "", "b", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
				"GcsURI":      "gs://test",
			}))
			const name = "rootInvocations/root-inv-id/workUnits/work-unit-id/artifacts/b"
			a, err := Read(ctx, name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Artifact, should.Match(&pb.Artifact{
				Name:        name,
				ArtifactId:  "b",
				ContentType: "text/plain",
				SizeBytes:   54,
				GcsUri:      "gs://test",
				HasLines:    true,
			}))
		})

		t.Run(`Exists with RbeURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Artifact(wuID.LegacyInvocationID(), "", "c", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
				"RBEURI":      "rbe://cas/default/blobs/deadbeef/54",
			}))
			const name = "rootInvocations/root-inv-id/workUnits/work-unit-id/artifacts/c"
			a, err := Read(ctx, name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Artifact, should.Match(&pb.Artifact{
				Name:        name,
				ArtifactId:  "c",
				ContentType: "text/plain",
				SizeBytes:   54,
				RbeUri:      "rbe://cas/default/blobs/deadbeef/54",
				HasLines:    true,
			}))
		})

		t.Run(`Exists (legacy invocation)`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Artifact("inv", "", "a", map[string]any{
				"ContentType": "text/plain",
				"Size":        "54",
			}))
			const name = "invocations/inv/artifacts/a"
			a, err := Read(ctx, name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Artifact, should.Match(&pb.Artifact{
				Name:        name,
				ArtifactId:  "a",
				ContentType: "text/plain",
				SizeBytes:   54,
				HasLines:    true,
			}))
		})
	})
}
