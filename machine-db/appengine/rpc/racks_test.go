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
		columns := []string{"r.name", "r.description", "r.state", "d.name", "h.name"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT r.name, r.description, r.state, d.name, h.name
				FROM \(racks r, datacenters d\)
				LEFT JOIN kvms k ON r.kvm_id = k.id
				LEFT JOIN hostnames h ON k.hostname_id = h.id
				WHERE r.datacenter_id = d.id AND r.name IN \(\?\)$
			`
			req := &crimson.ListRacksRequest{
				Names: []string{"rack"},
			}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			racks, err := listRacks(c, req)
			So(err, ShouldErrLike, "failed to fetch racks")
			So(racks, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty request", func() {
			selectStmt := `
				^SELECT r.name, r.description, r.state, d.name, h.name
				FROM \(racks r, datacenters d\)
				LEFT JOIN kvms k ON r.kvm_id = k.id
				LEFT JOIN hostnames h ON k.hostname_id = h.id
				WHERE r.datacenter_id = d.id$
			`
			req := &crimson.ListRacksRequest{}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			racks, err := listRacks(c, req)
			So(err, ShouldBeNil)
			So(racks, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty response", func() {
			selectStmt := `
				^SELECT r.name, r.description, r.state, d.name, h.name
				FROM \(racks r, datacenters d\)
				LEFT JOIN kvms k ON r.kvm_id = k.id
				LEFT JOIN hostnames h ON k.hostname_id = h.id
				WHERE r.datacenter_id = d.id AND r.name IN \(\?\)$
			`
			req := &crimson.ListRacksRequest{
				Names: []string{"rack"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			racks, err := listRacks(c, req)
			So(err, ShouldBeNil)
			So(racks, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT r.name, r.description, r.state, d.name, h.name
				FROM \(racks r, datacenters d\)
				LEFT JOIN kvms k ON r.kvm_id = k.id
				LEFT JOIN hostnames h ON k.hostname_id = h.id
				WHERE r.datacenter_id = d.id AND r.name IN \(\?,\?\)$
			`
			req := &crimson.ListRacksRequest{
				Names: []string{"rack 1", "rack 2"},
			}
			rows.AddRow("rack 1", "description 1", common.State_FREE, "datacenter 1", "kvm 1")
			rows.AddRow("rack 2", "description 2", common.State_SERVING, "datacenter 2", nil)
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1]).WillReturnRows(rows)
			racks, err := listRacks(c, req)
			So(err, ShouldBeNil)
			So(racks, ShouldResemble, []*crimson.Rack{
				{
					Name:        "rack 1",
					Description: "description 1",
					State:       common.State_FREE,
					Datacenter:  "datacenter 1",
					Kvm:         "kvm 1",
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

		Convey("ok", func() {
			selectStmt := `
				^SELECT r.name, r.description, r.state, d.name, h.name
				FROM \(racks r, datacenters d\)
				LEFT JOIN kvms k ON r.kvm_id = k.id
				LEFT JOIN hostnames h ON k.hostname_id = h.id
				WHERE r.datacenter_id = d.id$
			`
			req := &crimson.ListRacksRequest{}
			rows.AddRow("rack 1", "description 1", common.State_FREE, "datacenter 1", "kvm 1")
			rows.AddRow("rack 2", "description 2", common.State_SERVING, "datacenter 2", nil)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			racks, err := listRacks(c, req)
			So(err, ShouldBeNil)
			So(racks, ShouldResemble, []*crimson.Rack{
				{
					Name:        "rack 1",
					Description: "description 1",
					State:       common.State_FREE,
					Datacenter:  "datacenter 1",
					Kvm:         "kvm 1",
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
