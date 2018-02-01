// Copyright 2017 The LUCI Authors.
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

package rpc

import (
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListPlatforms(t *testing.T) {
	Convey("listPlatforms", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT p.name, p.description, p.state
			FROM platforms p$
		`
		columns := []string{"p.name", "p.description", "p.state"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("platform")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			platforms, err := listPlatforms(c, names)
			So(err, ShouldErrLike, "failed to fetch platforms")
			So(platforms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("platform")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			platforms, err := listPlatforms(c, names)
			So(err, ShouldBeNil)
			So(platforms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("platform")
			rows.AddRow("platform 1", "description 1", common.State_FREE)
			rows.AddRow("platform 2", "description 2", common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			platforms, err := listPlatforms(c, names)
			So(err, ShouldBeNil)
			So(platforms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("platform 2", "platform 3")
			rows.AddRow("platform 1", "description 1", common.State_FREE)
			rows.AddRow("platform 2", "description 2", common.State_SERVING)
			rows.AddRow("platform 3", "description 3", common.State_REPAIR)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			platforms, err := listPlatforms(c, names)
			So(err, ShouldBeNil)
			So(platforms, ShouldResemble, []*crimson.Platform{
				{
					Name:        "platform 2",
					Description: "description 2",
					State:       common.State_SERVING,
				},
				{
					Name:        "platform 3",
					Description: "description 3",
					State:       common.State_REPAIR,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			rows.AddRow("platform 1", "description 1", common.State_FREE)
			rows.AddRow("platform 2", "description 2", common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			platforms, err := listPlatforms(c, names)
			So(err, ShouldBeNil)
			So(platforms, ShouldResemble, []*crimson.Platform{
				{
					Name:        "platform 1",
					Description: "description 1",
					State:       common.State_FREE,
				},
				{
					Name:        "platform 2",
					Description: "description 2",
					State:       common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
