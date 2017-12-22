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

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetDatacenters(t *testing.T) {
	Convey("getDatacenters", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT d.name, d.description
			FROM datacenters d$
		`
		columns := []string{"d.name", "d.description"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("datacenter")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			dcs, err := getDatacenters(c, names)
			So(err, ShouldErrLike, "failed to fetch datacenters")
			So(dcs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("datacenter")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := getDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("datacenter")
			rows.AddRow("datacenter 1", "description 1")
			rows.AddRow("datacenter 2", "description 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := getDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("datacenter 2", "datacenter 3")
			rows.AddRow("datacenter 1", "description 1")
			rows.AddRow("datacenter 2", "description 2")
			rows.AddRow("datacenter 3", "description 3")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := getDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldResemble, []*crimson.Datacenter{
				{
					Name:        "datacenter 2",
					Description: "description 2",
				},
				{
					Name:        "datacenter 3",
					Description: "description 3",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			rows.AddRow("datacenter 1", "description 1")
			rows.AddRow("datacenter 2", "description 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			dcs, err := getDatacenters(c, names)
			So(err, ShouldBeNil)
			So(dcs, ShouldResemble, []*crimson.Datacenter{
				{
					Name:        "datacenter 1",
					Description: "description 1",
				},
				{
					Name:        "datacenter 2",
					Description: "description 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
