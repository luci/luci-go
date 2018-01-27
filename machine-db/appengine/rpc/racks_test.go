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

func TestListRacks(t *testing.T) {
	Convey("listRacks", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT r.name, r.description, r.state, d.name
			FROM racks r, datacenters d
			WHERE r.datacenter_id = d.id$
		`
		columns := []string{"r.name", "r.description", "r.state", "d.name"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			names := stringset.NewFromSlice("rack")
			dcs := stringset.NewFromSlice("datacenter")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			racks, err := listRacks(c, names, dcs)
			So(err, ShouldErrLike, "failed to fetch racks")
			So(racks, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			names := stringset.NewFromSlice("rack")
			dcs := stringset.NewFromSlice("datacenter")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			racks, err := listRacks(c, names, dcs)
			So(err, ShouldBeNil)
			So(racks, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			names := stringset.NewFromSlice("rack")
			dcs := stringset.NewFromSlice("datacenter")
			rows.AddRow("rack 1", "description 1", common.State_FREE, "datacenter 1")
			rows.AddRow("rack 2", "description 2", common.State_SERVING, "datacenter 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			racks, err := listRacks(c, names, dcs)
			So(err, ShouldBeNil)
			So(racks, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			names := stringset.NewFromSlice("rack 2", "rack 3")
			dcs := stringset.NewFromSlice("datacenter 2", "datacenter 3")
			rows.AddRow("rack 1", "description 1", common.State_FREE, "datacenter 1")
			rows.AddRow("rack 2", "description 2", common.State_SERVING, "datacenter 2")
			rows.AddRow("rack 3", "description 3", common.State_REPAIR, "datacenter 3")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			racks, err := listRacks(c, names, dcs)
			So(err, ShouldBeNil)
			So(racks, ShouldResemble, []*crimson.Rack{
				{
					Name:        "rack 2",
					Description: "description 2",
					State:       common.State_SERVING,
					Datacenter:  "datacenter 2",
				},
				{
					Name:        "rack 3",
					Description: "description 3",
					State:       common.State_REPAIR,
					Datacenter:  "datacenter 3",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			names := stringset.New(0)
			dcs := stringset.New(0)
			rows.AddRow("rack 1", "description 1", common.State_FREE, "datacenter 1")
			rows.AddRow("rack 2", "description 2", common.State_SERVING, "datacenter 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			racks, err := listRacks(c, names, dcs)
			So(err, ShouldBeNil)
			So(racks, ShouldResemble, []*crimson.Rack{
				{
					Name:        "rack 1",
					Description: "description 1",
					State:       common.State_FREE,
					Datacenter:  "datacenter 1",
				},
				{
					Name:        "rack 2",
					Description: "description 2",
					State:       common.State_SERVING,
					Datacenter:  "datacenter 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
