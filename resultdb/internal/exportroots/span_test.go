// Copyright 2024 The LUCI Authors.
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

package exportroots

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSpan(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		_, err := span.Apply(ctx, []*spanner.Mutation{
			insert.Invocation("inv-a", resultpb.Invocation_ACTIVE, map[string]any{}),
			insert.Invocation("inv-b", resultpb.Invocation_ACTIVE, map[string]any{}),
			insert.Invocation("inv-c", resultpb.Invocation_ACTIVE, map[string]any{}),
		})
		So(err, ShouldBeNil)

		Convey(`ReadForInvocation`, func() {
			roots := []ExportRoot{
				NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("root-a").Build(),
				NewBuilder(2).WithInvocation("inv-b").WithRootInvocation("root-a").Build(),
				NewBuilder(3).WithInvocation("inv-a").WithRootInvocation("root-b").Build(),
				NewBuilder(4).WithInvocation("inv-b").WithRootInvocation("root-b").Build(),
			}
			err = SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

			Convey(`Invocation with roots, no root restriction`, func() {
				got, err := ReadForInvocation(span.Single(ctx), "inv-a", RootRestriction{UseRestriction: false})
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{
					roots[0],
					roots[2],
				})
			})
			Convey(`Invocation with roots, root restriction`, func() {
				got, err := ReadForInvocation(span.Single(ctx), "inv-a", RootRestriction{UseRestriction: true, InvocationIDs: invocations.NewIDSet("root-a")})
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{
					roots[0],
				})
			})
			Convey(`Empty invocation`, func() {
				got, err := ReadForInvocation(span.Single(ctx), "inv-empty", RootRestriction{UseRestriction: false})
				So(err, ShouldBeNil)
				So(got, ShouldBeEmpty)
			})
		})
		Convey(`ReadForInvocations`, func() {
			roots := []ExportRoot{
				NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("root-a").Build(),
				NewBuilder(2).WithInvocation("inv-b").WithRootInvocation("root-a").Build(),
				NewBuilder(3).WithInvocation("inv-a").WithRootInvocation("root-b").Build(),
				NewBuilder(4).WithInvocation("inv-b").WithRootInvocation("root-b").Build(),
				NewBuilder(5).WithInvocation("inv-b").WithRootInvocation("root-c").Build(),
				NewBuilder(6).WithInvocation("inv-c").WithRootInvocation("root-a").Build(),
			}
			err = SetForTesting(ctx, roots)
			So(err, ShouldBeNil)

			Convey(`Nominated invocations and roots`, func() {
				got, err := ReadForInvocations(span.Single(ctx), invocations.NewIDSet("inv-a", "inv-b"), invocations.NewIDSet("root-a", "root-b"))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, map[invocations.ID]map[invocations.ID]ExportRoot{
					"inv-a": {
						"root-a": roots[0],
						"root-b": roots[2],
					},
					"inv-b": {
						"root-a": roots[1],
						"root-b": roots[3],
					},
				})
			})
			Convey(`Missing export roots`, func() {
				results, err := ReadForInvocations(span.Single(ctx), invocations.NewIDSet("inv-empty"), invocations.NewIDSet("root-1", "root-2"))
				So(err, ShouldBeNil)
				So(results, ShouldResembleProto, map[invocations.ID]map[invocations.ID]ExportRoot{})
			})
		})
		Convey(`Create`, func() {
			entry := ExportRoot{
				Invocation:            "inv-a",
				RootInvocation:        "root-new",
				IsInheritedSourcesSet: false,
				IsNotified:            false,
			}
			Convey(`Without sources`, func() {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, Create(entry))
					return nil
				})
				So(err, ShouldBeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{entry})
			})
			Convey(`With sources, non-nil`, func() {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = testutil.TestSources()

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, Create(entry))
					return nil
				})
				So(err, ShouldBeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{entry})
			})
			Convey(`With sources, nil`, func() {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = nil

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, Create(entry))
					return nil
				})
				So(err, ShouldBeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{entry})
			})
		})
		Convey(`SetInheritedSources`, func() {
			entry := ExportRoot{
				Invocation:            "inv-a",
				RootInvocation:        "root-new",
				IsInheritedSourcesSet: false,
				IsNotified:            false,
			}
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				span.BufferWrite(ctx, Create(entry))
				return nil
			})
			So(err, ShouldBeNil)

			Convey(`Non-nil sources`, func() {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = testutil.TestSources()

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, SetInheritedSources(entry))
					return nil
				})
				So(err, ShouldBeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{entry})
			})
			Convey(`Nil sources`, func() {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = nil

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, SetInheritedSources(entry))
					return nil
				})
				So(err, ShouldBeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(got, ShouldResembleProto, []ExportRoot{entry})
			})
		})
		Convey(`SetNotified`, func() {
			entry := ExportRoot{
				Invocation:            "inv-a",
				RootInvocation:        "root-new",
				IsInheritedSourcesSet: false,
				IsNotified:            false,
			}
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				span.BufferWrite(ctx, Create(entry))
				return nil
			})
			So(err, ShouldBeNil)

			commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entry.IsNotified = true
				span.BufferWrite(ctx, SetNotified(entry))
				return nil
			})
			So(err, ShouldBeNil)

			expected := entry
			expected.NotifiedTime = commitTime.In(time.UTC)

			got, err := ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(got, ShouldResembleProto, []ExportRoot{expected})
		})
	})
}
