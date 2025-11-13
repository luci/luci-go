// Copyright 2025 The LUCI Authors.
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

package workunits

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run("Query", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("test-root-inv")
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(
			rootinvocations.NewBuilder(rootInvID).Build(),
		)...)

		wuRoot := NewBuilder(rootInvID, "root").Build()
		wuGrandparent := NewBuilder(rootInvID, "grandparent").WithParentWorkUnitID("root").Build()
		wuParent := NewBuilder(rootInvID, "parent").WithParentWorkUnitID("grandparent").Build()
		wuChild := NewBuilder(rootInvID, "child").WithParentWorkUnitID("parent").Build()

		ms := InsertForTesting(wuRoot)
		ms = append(ms, InsertForTesting(wuGrandparent)...)
		ms = append(ms, InsertForTesting(wuParent)...)
		ms = append(ms, InsertForTesting(wuChild)...)
		testutil.MustApply(ctx, t, ms...)

		wuRoot.ChildWorkUnits = []ID{wuGrandparent.ID}
		wuGrandparent.ChildWorkUnits = []ID{wuParent.ID}
		wuParent.ChildWorkUnits = []ID{wuChild.ID}

		q := &Query{
			RootInvocationID: rootInvID,
			Predicate:        &pb.WorkUnitPredicate{},
			Mask:             AllFields,
		}

		mustQuery := func(q *Query) ([]*WorkUnitRow, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			res, err := q.Query(ctx)
			return res, err
		}

		t.Run("AncestorsOf", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				q.Predicate.AncestorsOf = wuChild.ID.Name()
				wus, err := mustQuery(q)
				assert.Loosely(t, err, should.BeNil)

				assert.That(t, wus, should.Match([]*WorkUnitRow{
					wuParent,      // Closest parent
					wuGrandparent, // Middle ancestor
					wuRoot,        // Furthest ancestor (root work unit)
				}))
			})

			t.Run("start at root", func(t *ftt.Test) {
				q.Predicate.AncestorsOf = wuRoot.ID.Name()
				wus, err := mustQuery(q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wus, should.BeEmpty)
			})

			t.Run("start at non-existent", func(t *ftt.Test) {
				q.Predicate.AncestorsOf = ID{RootInvocationID: rootInvID, WorkUnitID: "non-existent"}.Name()
				wus, err := mustQuery(q)
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLike(`"rootInvocations/test-root-inv/workUnits/non-existent" not found`))
				assert.Loosely(t, wus, should.BeNil)
			})

			t.Run("masking works", func(t *ftt.Test) {
				extraProps, _ := structpb.NewStruct(map[string]any{"key": "value"})
				wuExpensive := NewBuilder(rootInvID, "expensive").
					WithParentWorkUnitID("root").
					WithExtendedProperties(map[string]*structpb.Struct{"ns": extraProps}).
					Build()
				testutil.MustApply(ctx, t, InsertForTesting(wuExpensive)...)

				// Update expected root to have this new child.
				wuRoot.ChildWorkUnits = append(wuRoot.ChildWorkUnits, wuExpensive.ID)

				q.Predicate.AncestorsOf = wuExpensive.ID.Name()
				q.Mask = ExcludeExtendedProperties
				wus, err := mustQuery(q)
				assert.Loosely(t, err, should.BeNil)

				// Ancestor is wuRoot. Check that extended properties are nil due to mask.
				assert.Loosely(t, wus, should.HaveLength(1))
				assert.Loosely(t, wus[0].ID, should.Equal(wuRoot.ID))
				assert.Loosely(t, wus[0].ExtendedProperties, should.BeNil)
			})
		})

		t.Run("Unimplemented predicate", func(t *ftt.Test) {
			q.Predicate = &pb.WorkUnitPredicate{}
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			_, err := q.Query(ctx)
			assert.That(t, err, should.ErrLike("predicate not implemented"))
		})
	})
}
