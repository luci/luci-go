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
