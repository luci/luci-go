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

package teams

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/teams/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	Convey("Validate", t, func() {
		Convey("valid", func() {
			err := Validate(NewTeamBuilder().Build())
			So(err, ShouldBeNil)
		})
		Convey("id", func() {
			Convey("must be specified", func() {
				err := Validate(NewTeamBuilder().WithID("").Build())
				So(err, ShouldErrLike, "id: must be specified")
			})
			Convey("must match format", func() {
				err := Validate(NewTeamBuilder().WithID("INVALID").Build())
				So(err, ShouldErrLike, "id: expected format")
			})
		})
	})
}

func TestStatusTable(t *testing.T) {
	Convey("Create", t, func() {
		ctx := testutil.SpannerTestContext(t)
		team := NewTeamBuilder().Build()

		m, err := Create(team)
		So(err, ShouldBeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		team.CreateTime = ts.UTC()

		So(err, ShouldBeNil)
		fetched, err := Read(span.Single(ctx), team.ID)
		So(err, ShouldBeNil)
		So(fetched, ShouldResemble, team)
	})

	Convey("Read", t, func() {
		Convey("Single", func() {
			ctx := testutil.SpannerTestContext(t)
			team, err := NewTeamBuilder().CreateInDB(ctx)
			So(err, ShouldBeNil)

			fetched, err := Read(span.Single(ctx), team.ID)

			So(err, ShouldBeNil)
			So(fetched, ShouldResemble, team)
		})

		Convey("Not exists", func() {
			ctx := testutil.SpannerTestContext(t)

			_, err := Read(span.Single(ctx), "123456")

			So(err, ShouldEqual, NotExistsErr)
		})
	})
}
