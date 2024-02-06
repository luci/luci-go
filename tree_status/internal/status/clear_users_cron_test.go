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
	"testing"
	"time"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClearStatusUsers(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {

		ctx := testutil.IntegrationTestContext(t)

		Convey(`Status older than 30 days should have their CreateUser cleared`, func() {
			NewStatusBuilder().
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				CreateInDB(ctx)

			rows, err := clearCreateUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 1)

			status, err := ReadLatest(span.Single(ctx), "chromium")
			So(err, ShouldBeNil)
			So(status.CreateUser, ShouldEqual, "")
		})

		Convey(`Status created less than 30 days ago should not change`, func() {
			NewStatusBuilder().
				WithCreateTime(time.Now()).
				WithCreateUser("user@example.com").
				CreateInDB(ctx)

			rows, err := clearCreateUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 0)

			status, err := ReadLatest(span.Single(ctx), "chromium")
			So(err, ShouldBeNil)
			So(status.CreateUser, ShouldEqual, "user@example.com")
		})

		Convey(`ClearStatusUsers clears CreateUser`, func() {
			NewStatusBuilder().
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				CreateInDB(ctx)

			err := ClearStatusUsers(ctx)
			So(err, ShouldBeNil)

			status, err := ReadLatest(span.Single(ctx), "chromium")
			So(err, ShouldBeNil)
			So(status.CreateUser, ShouldEqual, "")
		})
	})
}
