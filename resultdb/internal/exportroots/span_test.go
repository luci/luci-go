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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestSpan(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		_, err := span.Apply(ctx, []*spanner.Mutation{
			insert.Invocation("inv-a", resultpb.Invocation_ACTIVE, map[string]any{}),
			insert.Invocation("inv-b", resultpb.Invocation_ACTIVE, map[string]any{}),
			insert.Invocation("inv-c", resultpb.Invocation_ACTIVE, map[string]any{}),
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`ReadForInvocation`, func(t *ftt.Test) {
			roots := []ExportRoot{
				NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("root-a").Build(),
				NewBuilder(2).WithInvocation("inv-b").WithRootInvocation("root-a").Build(),
				NewBuilder(3).WithInvocation("inv-a").WithRootInvocation("root-b").Build(),
				NewBuilder(4).WithInvocation("inv-b").WithRootInvocation("root-b").Build(),
			}
			err = SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`Invocation with roots, no root restriction`, func(t *ftt.Test) {
				got, err := ReadForInvocation(span.Single(ctx), "inv-a", RootRestriction{UseRestriction: false})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{
					roots[0],
					roots[2],
				}))
			})
			t.Run(`Invocation with roots, root restriction`, func(t *ftt.Test) {
				got, err := ReadForInvocation(span.Single(ctx), "inv-a", RootRestriction{UseRestriction: true, InvocationIDs: invocations.NewIDSet("root-a")})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{
					roots[0],
				}))
			})
			t.Run(`Empty invocation`, func(t *ftt.Test) {
				got, err := ReadForInvocation(span.Single(ctx), "inv-empty", RootRestriction{UseRestriction: false})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.BeEmpty)
			})
		})
		t.Run(`ReadForInvocations`, func(t *ftt.Test) {
			roots := []ExportRoot{
				NewBuilder(1).WithInvocation("inv-a").WithRootInvocation("root-a").Build(),
				NewBuilder(2).WithInvocation("inv-b").WithRootInvocation("root-a").Build(),
				NewBuilder(3).WithInvocation("inv-a").WithRootInvocation("root-b").Build(),
				NewBuilder(4).WithInvocation("inv-b").WithRootInvocation("root-b").Build(),
				NewBuilder(5).WithInvocation("inv-b").WithRootInvocation("root-c").Build(),
				NewBuilder(6).WithInvocation("inv-c").WithRootInvocation("root-a").Build(),
			}
			err = SetForTesting(ctx, roots)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`Nominated invocations and roots`, func(t *ftt.Test) {
				got, err := ReadForInvocations(span.Single(ctx), invocations.NewIDSet("inv-a", "inv-b"), invocations.NewIDSet("root-a", "root-b"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match(map[invocations.ID]map[invocations.ID]ExportRoot{
					"inv-a": {
						"root-a": roots[0],
						"root-b": roots[2],
					},
					"inv-b": {
						"root-a": roots[1],
						"root-b": roots[3],
					},
				}))
			})
			t.Run(`Missing export roots`, func(t *ftt.Test) {
				results, err := ReadForInvocations(span.Single(ctx), invocations.NewIDSet("inv-empty"), invocations.NewIDSet("root-1", "root-2"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(map[invocations.ID]map[invocations.ID]ExportRoot{}))
			})
		})
		t.Run(`Create`, func(t *ftt.Test) {
			entry := ExportRoot{
				Invocation:            "inv-a",
				RootInvocation:        "root-new",
				IsInheritedSourcesSet: false,
				IsNotified:            false,
			}
			t.Run(`Without sources`, func(t *ftt.Test) {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, Create(entry))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{entry}))
			})
			t.Run(`With sources, non-nil`, func(t *ftt.Test) {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = testutil.TestSources()

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, Create(entry))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{entry}))
			})
			t.Run(`With sources, nil`, func(t *ftt.Test) {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = nil

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, Create(entry))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{entry}))
			})
		})
		t.Run(`SetInheritedSources`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			t.Run(`Non-nil sources`, func(t *ftt.Test) {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = testutil.TestSources()

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, SetInheritedSources(entry))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{entry}))
			})
			t.Run(`Nil sources`, func(t *ftt.Test) {
				entry.IsInheritedSourcesSet = true
				entry.InheritedSources = nil

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, SetInheritedSources(entry))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)

				got, err := ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got, should.Match([]ExportRoot{entry}))
			})
		})
		t.Run(`SetNotified`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entry.IsNotified = true
				span.BufferWrite(ctx, SetNotified(entry))
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			expected := entry
			expected.NotifiedTime = commitTime.In(time.UTC)

			got, err := ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Match([]ExportRoot{expected}))
		})
	})
}
