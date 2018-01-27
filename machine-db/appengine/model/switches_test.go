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

func TestSwitches(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `^SELECT id, name, description, ports, state, rack_id FROM switches$`
		columns := []string{"id", "name", "description", "ports", "state", "rack_id"}
		rows := sqlmock.NewRows(columns)
		table := &SwitchesTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(table.fetch(c), ShouldErrLike, "failed to select switches")
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
			rows.AddRow(1, "switch 1", "description 1", 10, common.State_FREE, 1)
			rows.AddRow(2, "switch 2", "description 2", 20, common.State_SERVING, 2)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldResemble, []*Switch{
				{
					Switch: config.Switch{
						Name:        "switch 1",
						Description: "description 1",
						Ports:       10,
						State:       common.State_FREE,
					},
					Id:     1,
					RackId: 1,
				},
				{
					Switch: config.Switch{
						Name:        "switch 2",
						Description: "description 2",
						Ports:       20,
						State:       common.State_SERVING,
					},
					Id:     2,
					RackId: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		c := context.Background()
		table := &SwitchesTable{
			racks: make(map[string]int64, 0),
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
							Switch: []*config.Switch{
								{
									Name: "switch",
								},
							},
						},
					},
				},
			}
			So(table.computeChanges(c, dcs), ShouldErrLike, "failed to determine rack ID")
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
							Name: "rack 1",
							Switch: []*config.Switch{
								{
									Name:        "switch 1",
									Description: "description 1",
									Ports:       10,
									State:       common.State_FREE,
								},
								{
									Name:        "switch 2",
									Description: "description 2",
									Ports:       20,
									State:       common.State_SERVING,
								},
							},
						},
						{
							Name: "rack 2",
							Switch: []*config.Switch{
								{
									Name:        "switch 3",
									Description: "description 3",
									Ports:       30,
									State:       common.State_REPAIR,
								},
							},
						},
					},
				},
				{
					Name: "datacenter 2",
					Rack: []*config.Rack{
						{
							Name: "rack 3",
							Switch: []*config.Switch{
								{
									Name:        "switch 4",
									Description: "description 4",
									Ports:       40,
									State:       common.State_TEST,
								},
							},
						},
					},
				},
			}
			table.racks[dcs[0].Rack[0].Name] = 1
			table.racks[dcs[0].Rack[1].Name] = 2
			table.racks[dcs[1].Rack[0].Name] = 3
			table.computeChanges(c, dcs)
			So(table.additions, ShouldResemble, []*Switch{
				{
					Switch: config.Switch{
						Name:        "switch 1",
						Description: "description 1",
						Ports:       10,
						State:       common.State_FREE,
					},
					RackId: table.racks[dcs[0].Rack[0].Name],
				},
				{
					Switch: config.Switch{
						Name:        "switch 2",
						Description: "description 2",
						Ports:       20,
						State:       common.State_SERVING,
					},
					RackId: table.racks[dcs[0].Rack[0].Name],
				},
				{
					Switch: config.Switch{
						Name:        "switch 3",
						Description: "description 3",
						Ports:       30,
						State:       common.State_REPAIR,
					},
					RackId: table.racks[dcs[0].Rack[1].Name],
				},
				{
					Switch: config.Switch{
						Name:        "switch 4",
						Description: "description 4",
						Ports:       40,
						State:       common.State_TEST,
					},
					RackId: table.racks[dcs[1].Rack[0].Name],
				},
			})
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name:        "switch 1",
					Description: "old description",
					Ports:       10,
					State:       common.State_FREE,
				},
				Id:     1,
				RackId: 1,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name: "switch 2",
				},
				Id:     1,
				RackId: 1,
			})
			dcs := []*config.Datacenter{
				{
					Name: "datacenter",
					Rack: []*config.Rack{
						{
							Name: "rack 1",
							Switch: []*config.Switch{
								{
									Name:        table.current[0].Name,
									Description: "new description",
									Ports:       20,
									State:       common.State_SERVING,
								},
							},
						},
						{
							Name: "rack 2",
							Switch: []*config.Switch{
								{
									Name: table.current[1].Name,
								},
							},
						},
					},
				},
			}
			table.racks[dcs[0].Rack[0].Name] = 1
			table.racks[dcs[0].Rack[1].Name] = 2
			table.computeChanges(c, dcs)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldResemble, []*Switch{
				{
					Switch: config.Switch{
						Name:        dcs[0].Rack[0].Switch[0].Name,
						Description: dcs[0].Rack[0].Switch[0].Description,
						Ports:       dcs[0].Rack[0].Switch[0].Ports,
						State:       dcs[0].Rack[0].Switch[0].State,
					},
					Id:     table.current[0].Id,
					RackId: table.racks[dcs[0].Rack[0].Name],
				},
				{
					Switch: config.Switch{
						Name:        dcs[0].Rack[1].Switch[0].Name,
						Description: dcs[0].Rack[1].Switch[0].Description,
						Ports:       dcs[0].Rack[1].Switch[0].Ports,
						State:       dcs[0].Rack[1].Switch[0].State,
					},
					Id:     table.current[0].Id,
					RackId: table.racks[dcs[0].Rack[1].Name],
				},
			})
			So(table.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				Id:     1,
				RackId: 1,
			})
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldResemble, []*Switch{
				{
					Switch: config.Switch{
						Name: table.current[0].Name,
					},
					Id:     table.current[0].Id,
					RackId: table.current[0].RackId,
				},
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO switches \(name, description, ports, state, rack_id\) VALUES \(\?, \?, \?, \?, \?\)$`
		table := &SwitchesTable{}

		Convey("empty", func() {
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.additions = append(table.additions, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				RackId: 1,
			})
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to prepare statement")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.additions = append(table.additions, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				RackId: 1,
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to add switch")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.additions = append(table.additions, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				RackId: 1,
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
		deleteStmt := `^DELETE FROM switches WHERE id = \?$`
		table := &SwitchesTable{}

		Convey("empty", func() {
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.removals = append(table.removals, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				Id: 1,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
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
			table.removals = append(table.removals, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				Id: 1,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name: table.removals[0].Name,
				},
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to remove switch")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.removals = append(table.removals, &Switch{
				Switch: config.Switch{
					Name: "switch",
				},
				Id: 1,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
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
		updateStmt := `^UPDATE switches SET description = \?, ports = \?, state = \?, rack_id = \? WHERE id = \?$`
		table := &SwitchesTable{}

		Convey("empty", func() {
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.updates = append(table.updates, &Switch{
				Switch: config.Switch{
					Name:        "switch",
					Description: "new description",
					Ports:       20,
					State:       common.State_SERVING,
				},
				Id:     1,
				RackId: 2,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name:        table.updates[0].Name,
					Description: "old description",
					Ports:       10,
					State:       common.State_FREE,
				},
				Id:     table.updates[0].Id,
				RackId: 1,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to prepare statement")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.updates = append(table.updates, &Switch{
				Switch: config.Switch{
					Name:        "switch",
					Description: "new description",
					Ports:       20,
					State:       common.State_SERVING,
				},
				Id:     1,
				RackId: 2,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name:        table.updates[0].Name,
					Description: "old description",
					Ports:       10,
					State:       common.State_FREE,
				},
				Id:     table.updates[0].Id,
				RackId: 1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to update switch")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.updates = append(table.updates, &Switch{
				Switch: config.Switch{
					Name:        "switch",
					Description: "new description",
					Ports:       20,
					State:       common.State_SERVING,
				},
				Id:     1,
				RackId: 2,
			})
			table.current = append(table.current, &Switch{
				Switch: config.Switch{
					Name:        table.updates[0].Name,
					Description: "old description",
					Ports:       10,
					State:       common.State_FREE,
				},
				Id:     table.updates[0].Id,
				RackId: 1,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(table.current[0].Description, ShouldEqual, "new description")
			So(table.current[0].Ports, ShouldEqual, 20)
			So(table.current[0].State, ShouldEqual, common.State_SERVING)
			So(table.current[0].RackId, ShouldEqual, 2)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
