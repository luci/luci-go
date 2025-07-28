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

// Package graph contains methods to explore reachable invocations.
package graph

import (
	"testing"

	"cloud.google.com/go/spanner"

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

func TestFindInheritSourcesDescendants(t *testing.T) {
	ftt.Run(`FindInheritSourcesDescendants`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		sources := spanutil.Compressed(pbutil.MustMarshal(&pb.Sources{
			GitilesCommit: &pb.GitilesCommit{Host: "testHost"},
			IsDirty:       false,
		}))
		t.Run(`Includes inherited and non-inherted: a->b, a->c`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Match(invocations.NewIDSet("a", "b")))
		})

		t.Run(`Includes indirectly inherited a->b->c`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Match(invocations.NewIDSet("a", "b", "c")))
		})

		t.Run(`Cycle with one node: a->b<->b`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "b"),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Match(invocations.NewIDSet("a", "b")))
		})

		t.Run(`Cycle with two: a->b<->c`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}, "b"),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Match(invocations.NewIDSet("a", "b", "c")))
		})
	})
}

func TestFindRoot(t *testing.T) {
	ftt.Run(`TestFindRoot`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run(`invocation is a root by is_export_root a->b*`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				[]*spanner.Mutation{insert.Invocation("b", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true})},
			)...)

			inv, err := FindRoots(ctx, "b")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Match(invocations.NewIDSet("b")))
		})

		t.Run(`invocation has no parent a*-> b`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
			)...)

			inv, err := FindRoots(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Match(invocations.NewIDSet("a")))
		})

		t.Run(`traverse to root with no parent a->b->c->d*->e`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil, "d"),
				insert.InvocationWithInclusions("d", pb.Invocation_ACTIVE, nil, "e"),
			)...)

			inv, err := FindRoots(ctx, "d")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Match(invocations.NewIDSet("a")))
		})

		t.Run(`cycle a->b*<->[c, d (r)]`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil, "c", "d"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("d", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true}, "b"),
			)...)

			inv, err := FindRoots(ctx, "b")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Match(invocations.NewIDSet("a", "d")))
		})

		t.Run(`multiple root [x->a, y->b(r)]-> c*<- d <-e (r)`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("x", pb.Invocation_ACTIVE, nil, "a"),
				insert.InvocationWithInclusions("y", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true}, "c"),
				insert.InvocationWithInclusions("d", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("e", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true}, "d"),
				[]*spanner.Mutation{insert.Invocation("c", pb.Invocation_ACTIVE, nil)},
			)...)

			inv, err := FindRoots(ctx, "c")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Match(invocations.NewIDSet("x", "b", "e")))
		})
	})
}
