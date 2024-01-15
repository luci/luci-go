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
	"context"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/testutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStatusTable(t *testing.T) {
	Convey("Create", t, func() {
		ctx := testutil.SpannerTestContext(t)
		status := newStatusBuilder().Build()

		ts, err := span.Apply(ctx, []*spanner.Mutation{
			Create(ctx, status, status.CreateUser),
		})
		status.CreateTime = ts.UTC()

		So(err, ShouldBeNil)
		fetched, err := ReadLatest(span.Single(ctx), "chromium")
		So(err, ShouldBeNil)
		So(fetched, ShouldEqual, status)
	})

	Convey("Read", t, func() {
		Convey("Single", func() {
			ctx := testutil.SpannerTestContext(t)
			status := newStatusBuilder().CreateInDB(ctx)

			fetched, err := Read(span.Single(ctx), "chromium", status.StatusID)

			So(err, ShouldBeNil)
			So(fetched, ShouldEqual, status)
		})

		Convey("NotPresent", func() {
			ctx := testutil.SpannerTestContext(t)
			_ = newStatusBuilder().CreateInDB(ctx)

			_, err := Read(span.Single(ctx), "chromium", "1234")

			So(err, ShouldEqual, NotExistsErr)
		})
	})

	Convey("ReadLatest", t, func() {
		Convey("Exists", func() {
			ctx := testutil.SpannerTestContext(t)
			_ = newStatusBuilder().WithMessage("older").CreateInDB(ctx)
			expected := newStatusBuilder().WithMessage("newer").CreateInDB(ctx)

			fetched, err := ReadLatest(span.Single(ctx), "chromium")

			So(err, ShouldBeNil)
			So(fetched, ShouldEqual, expected)
		})

		Convey("NotPresent", func() {
			ctx := testutil.SpannerTestContext(t)

			_, err := ReadLatest(span.Single(ctx), "chromium")

			So(err, ShouldEqual, NotExistsErr)
		})
	})

	Convey("List", t, func() {
		Convey("Empty", func() {
			ctx := testutil.SpannerTestContext(t)

			actual, hasNextPage, err := List(span.Single(ctx), "chromium", nil)

			So(err, ShouldBeNil)
			So(actual, ShouldHaveLength, 0)
			So(hasNextPage, ShouldBeFalse)
		})

		Convey("Single page", func() {
			ctx := testutil.SpannerTestContext(t)
			older := newStatusBuilder().WithMessage("older").CreateInDB(ctx)
			newer := newStatusBuilder().WithMessage("newer").CreateInDB(ctx)

			actual, hasNextPage, err := List(span.Single(ctx), "chromium", nil)

			So(err, ShouldBeNil)
			So(actual, ShouldHaveLength, 2)
			So(actual, ShouldEqual, []*Status{newer, older})
			So(hasNextPage, ShouldBeFalse)
		})

		Convey("Paginated", func() {
			ctx := testutil.SpannerTestContext(t)
			older := newStatusBuilder().WithMessage("older").CreateInDB(ctx)
			newer := newStatusBuilder().WithMessage("newer").CreateInDB(ctx)

			firstPage, hasSecondPage, err1 := List(span.Single(ctx), "chromium", &ListOptions{Offset: 0, Limit: 1})
			secondPage, hasThirdPage, err2 := List(span.Single(ctx), "chromium", &ListOptions{Offset: 1, Limit: 1})

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(firstPage, ShouldEqual, []*Status{newer})
			So(secondPage, ShouldEqual, []*Status{older})
			So(hasSecondPage, ShouldBeTrue)
			So(hasThirdPage, ShouldBeFalse)
		})
	})
}

type StatusBuilder struct {
	status Status
}

func newStatusBuilder() *StatusBuilder {
	id, err := GenerateID()
	So(err, ShouldBeNil)
	return &StatusBuilder{status: Status{
		TreeName:      "chromium",
		StatusID:      id,
		GeneralStatus: pb.GeneralState_OPEN,
		Message:       "Tree is open!",
		CreateUser:    "user1",
		CreateTime:    spanner.CommitTimestamp,
	}}
}

func (b *StatusBuilder) WithMessage(message string) *StatusBuilder {
	b.status.Message = message
	return b
}

func (b *StatusBuilder) Build() *Status {
	s := b.status
	return &s
}

func (b *StatusBuilder) CreateInDB(ctx context.Context) *Status {
	status := b.Build()
	ts, err := span.Apply(ctx, []*spanner.Mutation{
		Create(ctx, status, status.CreateUser),
	})
	So(err, ShouldBeNil)
	status.CreateTime = ts.UTC()
	return status
}
