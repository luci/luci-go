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

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFindInheritSourcesDescendants(t *testing.T) {

	Convey(`FindInheritSourcesDescendants`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		sources := spanutil.Compressed(pbutil.MustMarshal(&pb.Sources{
			GitilesCommit: &pb.GitilesCommit{Host: "testHost"},
			IsDirty:       false,
		}))
		Convey(`Includes inherited and non-inherted: a->b, a->c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b", "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, invocations.NewIDSet("a", "b"))
		})

		Convey(`Includes indirectly inherited a->b->c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, invocations.NewIDSet("a", "b", "c"))
		})

		Convey(`Cycle with one node: a->b<->b`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "b"),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, invocations.NewIDSet("a", "b"))
		})

		Convey(`Cycle with two: a->b<->c`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZING, map[string]any{"Sources": sources}, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"InheritSources": true}, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_FINALIZING, map[string]any{"InheritSources": true}, "b"),
			)...)

			invs, err := FindInheritSourcesDescendants(ctx, "a")
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, invocations.NewIDSet("a", "b", "c"))
		})
	})
}

func TestFindRoot(t *testing.T) {

	Convey(`TestFindRoot`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		Convey(`invocation is a root by is_export_root a->b*`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				[]*spanner.Mutation{insert.Invocation("b", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true})},
			)...)

			inv, err := FindRoots(ctx, "b")
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, invocations.NewIDSet("b"))
		})

		Convey(`invocation has no parent a*-> b`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
			)...)

			inv, err := FindRoots(ctx, "a")
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, invocations.NewIDSet("a"))
		})

		Convey(`traverse to root with no parent a->b->c->d*->e`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil, "d"),
				insert.InvocationWithInclusions("d", pb.Invocation_ACTIVE, nil, "e"),
			)...)

			inv, err := FindRoots(ctx, "d")
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, invocations.NewIDSet("a"))
		})

		Convey(`cycle a->b*<->[c, d (r)]`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil, "c", "d"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("d", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true}, "b"),
			)...)

			inv, err := FindRoots(ctx, "b")
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, invocations.NewIDSet("a", "d"))
		})

		Convey(`multiple root [x->a, y->b(r)]-> c*<- d <-e (r)`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("x", pb.Invocation_ACTIVE, nil, "a"),
				insert.InvocationWithInclusions("y", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true}, "c"),
				insert.InvocationWithInclusions("d", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("e", pb.Invocation_ACTIVE, map[string]any{"IsExportRoot": true}, "d"),
				[]*spanner.Mutation{insert.Invocation("c", pb.Invocation_ACTIVE, nil)},
			)...)

			inv, err := FindRoots(ctx, "c")
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, invocations.NewIDSet("x", "b", "e"))
		})
	})
}
