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
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/machine-db/api/common/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListDatacenters(t *testing.T) {
	Convey("listDatacenters", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT name, description, state
			FROM datacenters$
		`
		columns := []string{"name", "description", "state"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("datacenter")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			dcs, err := listDatacenters(c, names)
			So(err, ShouldErrLike, "failed to fetch datacenters")
			So(dcs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("datacenter")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := listDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("datacenter")
			rows.AddRow("datacenter 1", "description 1", common.State_FREE)
			rows.AddRow("datacenter 2", "description 2", common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := listDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("datacenter 2", "datacenter 3")
			rows.AddRow("datacenter 1", "description 1", common.State_FREE)
			rows.AddRow("datacenter 2", "description 2", common.State_SERVING)
			rows.AddRow("datacenter 3", "description 3", common.State_REPAIR)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := listDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldResemble, []*crimson.Datacenter{
				{
					Name:        "datacenter 2",
					Description: "description 2",
					State:       common.State_SERVING,
				},
				{
					Name:        "datacenter 3",
					Description: "description 3",
					State:       common.State_REPAIR,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			rows.AddRow("datacenter 1", "description 1", common.State_FREE)
			rows.AddRow("datacenter 2", "description 2", common.State_SERVING)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := listDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldResemble, []*crimson.Datacenter{
				{
					Name:        "datacenter 1",
					Description: "description 1",
					State:       common.State_FREE,
				},
				{
					Name:        "datacenter 2",
					Description: "description 2",
					State:       common.State_SERVING,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
