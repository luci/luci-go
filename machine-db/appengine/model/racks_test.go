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

	"go.chromium.org/luci/machine-db/api/common/v1"
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
		selectStmt := `^SELECT id, name, description, state, datacenter_id FROM racks$`
		columns := []string{"id", "name", "description", "state", "datacenter_id"}
		rows := sqlmock.NewRows(columns)
		table := &RacksTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(table.fetch(c), ShouldErrLike, "failed to select racks")
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			rows.AddRow(1, "rack 1", "description 1", common.State_FREE, 1)
			rows.AddRow(2, "rack 2", "description 2", common.State_SERVING, 2)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldResemble, []*Rack{
				{
					Rack: config.Rack{
						Name:        "rack 1",
						Description: "description 1",
						State:       common.State_FREE,
					},
					Id:           1,
					DatacenterId: 1,
				},
				{
					Rack: config.Rack{
						Name:        "rack 2",
						Description: "description 2",
						State:       common.State_SERVING,
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
		table := &RacksTable{
			datacenters: make(map[string]int64, 0),
		}

		Convey("empty", func() {
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("id lookup failure", func() {
			dcs := []*config.Datacenter{
				{
					Name: "datacenter",
					Rack: []*config.Rack{
						{
							Name: "rack",
						},
					},
				},
			}
			So(table.computeChanges(c, dcs), ShouldErrLike, "failed to determine datacenter ID")
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("addition", func() {
			dcs := []*config.Datacenter{
				{
					Name: "datacenter 1",
					Rack: []*config.Rack{
						{
							Name:        "rack 1",
							Description: "description 1",
							State:       common.State_FREE,
						},
					},
				},
				{
					Name: "datacenter 2",
					Rack: []*config.Rack{
						{
							Name:        "rack 2",
							Description: "description 2",
							State:       common.State_SERVING,
						},
					},
				},
			}
			table.datacenters[dcs[0].Name] = 1
			table.datacenters[dcs[1].Name] = 2
			table.computeChanges(c, dcs)
			So(table.additions, ShouldResemble, []*Rack{
				{
					Rack: config.Rack{
						Name:        "rack 1",
						Description: "description 1",
						State:       common.State_FREE,
					},
					DatacenterId: table.datacenters[dcs[0].Name],
				},
				{
					Rack: config.Rack{
						Name:        "rack 2",
						Description: "description 2",
						State:       common.State_SERVING,
					},
					DatacenterId: table.datacenters[dcs[1].Name],
				},
			})
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name:        "rack 1",
					Description: "old description",
					State:       common.State_FREE,
				},
				Id:           1,
				DatacenterId: 1,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name: "rack 2",
				},
				Id:           1,
				DatacenterId: 1,
			})
			dcs := []*config.Datacenter{
				{
					Name: "datacenter 1",
					Rack: []*config.Rack{
						{
							Name:        "rack 1",
							Description: "new description",
							State:       common.State_SERVING,
						},
					},
				},
				{
					Name: "datacenter 2",
					Rack: []*config.Rack{
						{
							Name: "rack 2",
						},
					},
				},
			}
			table.datacenters[dcs[0].Name] = 1
			table.datacenters[dcs[1].Name] = 2
			table.computeChanges(c, dcs)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldResemble, []*Rack{
				{
					Rack: config.Rack{
						Name:        dcs[0].Rack[0].Name,
						Description: dcs[0].Rack[0].Description,
						State:       dcs[0].Rack[0].State,
					},
					Id:           table.current[0].Id,
					DatacenterId: table.datacenters[dcs[0].Name],
				},
				{
					Rack: config.Rack{
						Name:        dcs[1].Rack[0].Name,
						Description: dcs[1].Rack[0].Description,
						State:       dcs[1].Rack[0].State,
					},
					Id:           table.current[1].Id,
					DatacenterId: table.datacenters[dcs[1].Name],
				},
			})
			So(table.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				Id:           1,
				DatacenterId: 1,
			})
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldResemble, []*Rack{
				{
					Rack: config.Rack{
						Name: table.current[0].Name,
					},
					Id:           table.current[0].Id,
					DatacenterId: table.current[0].DatacenterId,
				},
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO racks \(name, description, state, datacenter_id\) VALUES \(\?, \?, \?, \?\)$`
		table := &RacksTable{}

		Convey("empty", func() {
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.additions = append(table.additions, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				DatacenterId: 1,
			})
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to prepare statement")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.additions = append(table.additions, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				DatacenterId: 1,
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to add rack")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.additions = append(table.additions, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				DatacenterId: 1,
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("remove", t, func() {
		db, m, _ := sqlmock.New()
		defer func() { So(m.ExpectationsWereMet(), ShouldBeNil) }()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `^DELETE FROM racks WHERE id = \?$`
		table := &RacksTable{}

		Convey("empty", func() {
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.removals = append(table.removals, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				Id: 1,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name: table.removals[0].Name,
				},
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to prepare statement")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.removals = append(table.removals, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				Id: 1,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name: table.removals[0].Name,
				},
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to remove rack")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.removals = append(table.removals, &Rack{
				Rack: config.Rack{
					Name: "rack",
				},
				Id: 1,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name: table.removals[0].Name,
				},
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("update", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateStmt := `^UPDATE racks SET description = \?, state = \?, datacenter_id = \? WHERE id = \?$`
		table := &RacksTable{}

		Convey("empty", func() {
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.updates = append(table.updates, &Rack{
				Rack: config.Rack{
					Name:        "rack",
					Description: "new description",
					State:       common.State_SERVING,
				},
				Id:           1,
				DatacenterId: 2,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name:        table.updates[0].Name,
					Description: "old description",
					State:       common.State_FREE,
				},
				Id:           table.updates[0].Id,
				DatacenterId: 1,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to prepare statement")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.updates = append(table.updates, &Rack{
				Rack: config.Rack{
					Name:        "rack",
					Description: "new description",
					State:       common.State_SERVING,
				},
				Id:           1,
				DatacenterId: 2,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name:        table.updates[0].Name,
					Description: "old description",
					State:       common.State_FREE,
				},
				Id:           table.updates[0].Id,
				DatacenterId: 1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to update rack")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.updates = append(table.updates, &Rack{
				Rack: config.Rack{
					Name:        "rack",
					Description: "new description",
					State:       common.State_SERVING,
				},
				Id:           1,
				DatacenterId: 2,
			})
			table.current = append(table.current, &Rack{
				Rack: config.Rack{
					Name:        table.updates[0].Name,
					Description: "old description",
					State:       common.State_FREE,
				},
				Id:           table.updates[0].Id,
				DatacenterId: 1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(table.current[0].Description, ShouldEqual, "new description")
			So(table.current[0].State, ShouldEqual, common.State_SERVING)
			So(table.current[0].DatacenterId, ShouldEqual, 2)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
