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

package status

import (
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/testutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestValidation(t *testing.T) {
	ftt.Run("Validate", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			err := Validate(NewStatusBuilder().Build())
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("tree_name", func(t *ftt.Test) {
			t.Run("must be specified", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithTreeName("").Build())
				assert.Loosely(t, err, should.ErrLike("tree: must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithTreeName("INVALID").Build())
				assert.Loosely(t, err, should.ErrLike("tree: expected format"))
			})
		})
		t.Run("id", func(t *ftt.Test) {
			t.Run("must be specified", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithStatusID("").Build())
				assert.Loosely(t, err, should.ErrLike("id: must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithStatusID("INVALID").Build())
				assert.Loosely(t, err, should.ErrLike("id: expected format"))
			})
		})
		t.Run("general_state", func(t *ftt.Test) {
			t.Run("must be specified", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithGeneralStatus(pb.GeneralState_GENERAL_STATE_UNSPECIFIED).Build())
				assert.Loosely(t, err, should.ErrLike("general_state: must be specified"))
			})
			t.Run("must be a valid enum value", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithGeneralStatus(pb.GeneralState(100)).Build())
				assert.Loosely(t, err, should.ErrLike("general_state: invalid enum value"))
			})
		})
		t.Run("message", func(t *ftt.Test) {
			t.Run("must be specified", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithMessage("").Build())
				assert.Loosely(t, err, should.ErrLike("message: must be specified"))
			})
			t.Run("must not exceed length", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithMessage(strings.Repeat("a", 1025)).Build())
				assert.Loosely(t, err, should.ErrLike("message: longer than 1024 bytes"))
			})
			t.Run("invalid utf-8 string", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithMessage("\xbd").Build())
				assert.Loosely(t, err, should.ErrLike("message: not a valid utf8 string"))
			})
			// TODO: unicode tests

		})
		t.Run("closing builder name", func(t *ftt.Test) {
			t.Run("ignored if status is not closed", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithClosingBuilderName("some name").Build())
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithGeneralStatus(pb.GeneralState_CLOSED).WithClosingBuilderName("some name").Build())
				assert.Loosely(t, err, should.ErrLike("closing_builder_name: expected format"))
			})
			t.Run("empty closing builder is OK", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithGeneralStatus(pb.GeneralState_CLOSED).WithClosingBuilderName("").Build())
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("valid", func(t *ftt.Test) {
				err := Validate(NewStatusBuilder().WithGeneralStatus(pb.GeneralState_CLOSED).WithClosingBuilderName("projects/chromium-m100/buckets/ci.shadow/builders/Linux 123").Build())
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestStatusTable(t *testing.T) {
	ftt.Run("Create", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		status := NewStatusBuilder().WithGeneralStatus(pb.GeneralState_CLOSED).WithClosingBuilderName("projects/chromium-m100/buckets/ci.shadow/builders/Linux 123").Build()

		m, err := Create(status, status.CreateUser)
		assert.Loosely(t, err, should.BeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		status.CreateTime = ts.UTC()

		assert.Loosely(t, err, should.BeNil)
		fetched, err := ReadLatest(span.Single(ctx), "chromium")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fetched, should.Match(status))
	})

	ftt.Run("Read", t, func(t *ftt.Test) {
		t.Run("Single", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			status := NewStatusBuilder().CreateInDB(ctx)

			fetched, err := Read(span.Single(ctx), "chromium", status.StatusID)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched, should.Match(status))
		})

		t.Run("With closing builder", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			status := NewStatusBuilder().WithGeneralStatus(pb.GeneralState_CLOSED).WithClosingBuilderName("projects/chromium-m100/buckets/ci.shadow/builders/Linux 123").CreateInDB(ctx)

			fetched, err := Read(span.Single(ctx), "chromium", status.StatusID)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched, should.Match(status))
		})

		t.Run("NotPresent", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			_ = NewStatusBuilder().CreateInDB(ctx)

			_, err := Read(span.Single(ctx), "chromium", "1234")

			assert.Loosely(t, err, should.Equal(NotExistsErr))
		})
	})

	ftt.Run("ReadLatest", t, func(t *ftt.Test) {
		t.Run("Exists", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			_ = NewStatusBuilder().WithMessage("older").CreateInDB(ctx)
			expected := NewStatusBuilder().WithMessage("newer").CreateInDB(ctx)

			fetched, err := ReadLatest(span.Single(ctx), "chromium")

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched, should.Match(expected))
		})

		t.Run("NotPresent", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)

			_, err := ReadLatest(span.Single(ctx), "chromium")

			assert.Loosely(t, err, should.Equal(NotExistsErr))
		})
	})

	ftt.Run("List", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)

			actual, hasNextPage, err := List(span.Single(ctx), "chromium", nil)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(0))
			assert.Loosely(t, hasNextPage, should.BeFalse)
		})

		t.Run("Single page", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			older := NewStatusBuilder().WithMessage("older").CreateInDB(ctx)
			newer := NewStatusBuilder().WithMessage("newer").CreateInDB(ctx)

			actual, hasNextPage, err := List(span.Single(ctx), "chromium", nil)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(2))
			assert.Loosely(t, actual, should.Resemble([]*Status{newer, older}))
			assert.Loosely(t, hasNextPage, should.BeFalse)
		})

		t.Run("Paginated", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			older := NewStatusBuilder().WithMessage("older").CreateInDB(ctx)
			newer := NewStatusBuilder().WithMessage("newer").CreateInDB(ctx)

			firstPage, hasSecondPage, err1 := List(span.Single(ctx), "chromium", &ListOptions{Offset: 0, Limit: 1})
			secondPage, hasThirdPage, err2 := List(span.Single(ctx), "chromium", &ListOptions{Offset: 1, Limit: 1})

			assert.Loosely(t, err1, should.BeNil)
			assert.Loosely(t, err2, should.BeNil)
			assert.Loosely(t, firstPage, should.Resemble([]*Status{newer}))
			assert.Loosely(t, secondPage, should.Resemble([]*Status{older}))
			assert.Loosely(t, hasSecondPage, should.BeTrue)
			assert.Loosely(t, hasThirdPage, should.BeFalse)
		})
	})

	ftt.Run("ListAfter", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			results, err := ListAfter(span.Single(ctx), time.Unix(1000, 0))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.HaveLength(0))
		})

		t.Run("Have data", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			NewStatusBuilder().WithMessage("mes1").WithCreateTime(time.Unix(300, 0).UTC()).CreateInDB(ctx)
			mes2 := NewStatusBuilder().WithMessage("mes2").WithCreateTime(time.Unix(500, 0).UTC()).CreateInDB(ctx)
			NewStatusBuilder().WithMessage("mes3").WithCreateTime(time.Unix(200, 0).UTC()).CreateInDB(ctx)
			mes4 := NewStatusBuilder().WithMessage("mes3").WithCreateTime(time.Unix(400, 0).UTC()).CreateInDB(ctx)

			results, err := ListAfter(span.Single(ctx), time.Unix(350, 0))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Resemble([]*Status{mes4, mes2}))
		})
	})
}
