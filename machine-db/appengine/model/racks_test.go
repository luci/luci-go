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

package model

import (
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRacks(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `^SELECT id, name, description, datacenter_id FROM racks$`
		columns := []string{"id", "name", "description", "datacenter_id"}
		rows := sqlmock.NewRows(columns)
		r := &RacksTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(r.fetch(c), ShouldErrLike, "failed to select racks")
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(r.fetch(c), ShouldBeNil)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			rows.AddRow(1, "rack 1", "description 1", 1)
			rows.AddRow(2, "rack 2", "description 2", 2)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(r.fetch(c), ShouldBeNil)
			So(r.current, ShouldResemble, []*Rack{
				{
					RackConfig: config.RackConfig{
						Name:        "rack 1",
						Description: "description 1",
					},
					Id:           1,
					DatacenterId: 1,
				},
				{
					RackConfig: config.RackConfig{
						Name:        "rack 2",
						Description: "description 2",
					},
					Id:           2,
					DatacenterId: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		c := context.Background()
		r := &RacksTable{
			datacenters: make(map[string]int64, 0),
		}

		Convey("empty", func() {
			r.computeChanges(c, nil)
			So(r.additions, ShouldBeEmpty)
			So(r.updates, ShouldBeEmpty)
			So(r.removals, ShouldBeEmpty)
		})

		Convey("id lookup failure", func() {
			dcs := []*config.DatacenterConfig{
				{
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
						},
					},
				},
			}
			So(r.computeChanges(c, dcs), ShouldErrLike, "failed to determine datacenter ID")
			So(r.additions, ShouldBeEmpty)
			So(r.updates, ShouldBeEmpty)
			So(r.removals, ShouldBeEmpty)
		})

		Convey("addition", func() {
			dcs := []*config.DatacenterConfig{
				{
					Name: "datacenter 1",
					Rack: []*config.RackConfig{
						{
							Name:        "rack 1",
							Description: "description 1",
						},
					},
				},
				{
					Name: "datacenter 2",
					Rack: []*config.RackConfig{
						{
							Name:        "rack 2",
							Description: "description 2",
						},
					},
				},
			}
			r.datacenters[dcs[0].Name] = 1
			r.datacenters[dcs[1].Name] = 2
			r.computeChanges(c, dcs)
			So(r.additions, ShouldResemble, []*Rack{
				{
					RackConfig: config.RackConfig{
						Name:        "rack 1",
						Description: "description 1",
					},
					DatacenterId: r.datacenters[dcs[0].Name],
				},
				{
					RackConfig: config.RackConfig{
						Name:        "rack 2",
						Description: "description 2",
					},
					DatacenterId: r.datacenters[dcs[1].Name],
				},
			})
			So(r.updates, ShouldBeEmpty)
			So(r.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				Id:           1,
				DatacenterId: 1,
			})
			dcs := []*config.DatacenterConfig{
				{
					Name: "datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
						},
					},
				},
			}
			r.datacenters[dcs[0].Name] = 2
			r.computeChanges(c, dcs)
			So(r.additions, ShouldBeEmpty)
			So(r.updates, ShouldResemble, []*Rack{
				{
					RackConfig: config.RackConfig{
						Name:        r.current[0].Name,
						Description: r.current[0].Description,
					},
					Id:           r.current[0].Id,
					DatacenterId: r.datacenters[dcs[0].Name],
				},
			})
			So(r.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				Id:           1,
				DatacenterId: 1,
			})
			r.computeChanges(c, nil)
			So(r.additions, ShouldBeEmpty)
			So(r.updates, ShouldBeEmpty)
			So(r.removals, ShouldResemble, []string{
				r.current[0].Name,
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO racks \(name, description, datacenter_id\) VALUES \(\?, \?, \?\)$`
		r := &RacksTable{}

		Convey("empty", func() {
			So(r.add(c), ShouldBeNil)
			So(r.additions, ShouldBeEmpty)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				DatacenterId: 1,
			})
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(r.add(c), ShouldErrLike, "failed to prepare statement")
			So(r.additions, ShouldHaveLength, 1)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				DatacenterId: 1,
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(r.add(c), ShouldErrLike, "failed to add rack")
			So(r.additions, ShouldHaveLength, 1)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				DatacenterId: 1,
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(r.add(c), ShouldBeNil)
			So(r.additions, ShouldBeEmpty)
			So(r.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("remove", t, func() {
		db, m, _ := sqlmock.New()
		defer func() { So(m.ExpectationsWereMet(), ShouldBeNil) }()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `^DELETE FROM racks WHERE name = \?$`
		r := &RacksTable{}

		Convey("empty", func() {
			So(r.remove(c), ShouldBeNil)
			So(r.removals, ShouldBeEmpty)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			r.removals = append(r.removals, "rack")
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: r.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(r.remove(c), ShouldErrLike, "failed to prepare statement")
			So(r.removals, ShouldHaveLength, 1)
			So(r.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			r.removals = append(r.removals, "rack")
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: r.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(r.remove(c), ShouldErrLike, "failed to remove rack")
			So(r.removals, ShouldHaveLength, 1)
			So(r.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			r.removals = append(r.removals, "rack")
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: r.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(r.remove(c), ShouldBeNil)
			So(r.removals, ShouldBeEmpty)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("update", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateStmt := `^UPDATE racks SET description = \?, datacenter_id = \? WHERE id = \?$`
		r := &RacksTable{}

		Convey("empty", func() {
			So(r.update(c), ShouldBeNil)
			So(r.updates, ShouldBeEmpty)
			So(r.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				Id:           1,
				DatacenterId: 2,
			})
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:           r.updates[0].Id,
				DatacenterId: 1,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(r.update(c), ShouldErrLike, "failed to prepare statement")
			So(r.updates, ShouldHaveLength, 1)
			So(r.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				Id:           1,
				DatacenterId: 2,
			})
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:           r.updates[0].Id,
				DatacenterId: 1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(r.update(c), ShouldErrLike, "failed to update rack")
			So(r.updates, ShouldHaveLength, 1)
			So(r.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: config.RackConfig{
					Name: "rack",
				},
				Id:           1,
				DatacenterId: 2,
			})
			r.current = append(r.current, &Rack{
				RackConfig: config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:           r.updates[0].Id,
				DatacenterId: 1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(r.update(c), ShouldBeNil)
			So(r.updates, ShouldBeEmpty)
			So(r.current, ShouldHaveLength, 1)
			So(r.current[0].DatacenterId, ShouldEqual, 2)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
