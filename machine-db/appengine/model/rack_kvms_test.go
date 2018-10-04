// Copyright 2018 The LUCI Authors.
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
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRackKVMs(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `^SELECT id, name, kvm_id FROM racks$`
		columns := []string{"id", "name", "kvm_id"}
		rows := sqlmock.NewRows(columns)
		table := &RackKVMsTable{}

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
			rows.AddRow(1, "rack 1", 1)
			rows.AddRow(2, "rack 2", nil)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldResemble, []*RackKVM{
				{
					Rack: config.Rack{
						Name: "rack 1",
					},
					Id: 1,
					KVMId: sql.NullInt64{
						Int64: 1,
						Valid: true,
					},
				},
				{
					Rack: config.Rack{
						Name: "rack 2",
					},
					Id: 2,
					KVMId: sql.NullInt64{
						Valid: false,
					},
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		c := context.Background()
		table := &RackKVMsTable{
			kvms: make(map[string]int64, 0),
		}

		Convey("empty", func() {
			table.computeChanges(c, nil)
			So(table.updates, ShouldBeEmpty)
		})

		Convey("id lookup failure", func() {
			dcs := []*config.Datacenter{
				{
					Rack: []*config.Rack{
						{
							Name: "rack",
							Kvm:  "kvm",
						},
					},
				},
			}
			So(table.computeChanges(c, dcs), ShouldErrLike, "failed to determine KVM ID")
			So(table.updates, ShouldBeEmpty)
		})

		Convey("update", func() {
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: "rack 1",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: "rack 2",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Int64: 2,
					Valid: true,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: "rack 3",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Int64: 3,
					Valid: true,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: "rack 4",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			dcs := []*config.Datacenter{
				{
					Rack: []*config.Rack{
						{
							Name: "rack 1",
							Kvm:  "kvm 1",
						},
						{
							Name: "rack 2",
						},
						{
							Name: "rack 3",
							Kvm:  "kvm 3",
						},
						{
							Name: "rack 4",
						},
					},
				},
			}
			table.kvms[dcs[0].Rack[0].Kvm] = 1
			table.kvms[dcs[0].Rack[2].Kvm] = 3
			table.computeChanges(c, dcs)
			So(table.updates, ShouldResemble, []*RackKVM{
				{
					Rack: config.Rack{
						Name: dcs[0].Rack[0].Name,
					},
					Id: table.current[0].Id,
					KVMId: sql.NullInt64{
						Int64: table.kvms[dcs[0].Rack[0].Kvm],
						Valid: true,
					},
				},
				{
					Rack: config.Rack{
						Name: dcs[0].Rack[1].Name,
					},
					Id: table.current[1].Id,
					KVMId: sql.NullInt64{
						Valid: false,
					},
				},
			})
		})
	})

	Convey("update", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateStmt := `^UPDATE racks SET kvm_id = \? WHERE id = \?$`
		table := &RackKVMsTable{}

		Convey("empty", func() {
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.updates = append(table.updates, &RackKVM{
				Rack: config.Rack{
					Name: "rack 1",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Int64: 1,
					Valid: true,
				},
			})
			table.updates = append(table.updates, &RackKVM{
				Rack: config.Rack{
					Name: "rack 2",
				},
				Id: 2,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: table.updates[0].Name,
				},
				Id: table.updates[0].Id,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: table.updates[1].Name,
				},
				Id: table.updates[1].Id,
				KVMId: sql.NullInt64{
					Int64: 2,
					Valid: true,
				},
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to prepare statement")
			So(table.updates, ShouldHaveLength, 2)
			So(table.updates[0].KVMId, ShouldResemble, sql.NullInt64{
				Int64: 1,
				Valid: true,
			})
			So(table.updates[1].KVMId, ShouldResemble, sql.NullInt64{
				Valid: false,
			})
			So(table.current, ShouldHaveLength, 2)
			So(table.current[0].KVMId, ShouldResemble, sql.NullInt64{
				Valid: false,
			})
			So(table.current[1].KVMId, ShouldResemble, sql.NullInt64{
				Int64: 2,
				Valid: true,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.updates = append(table.updates, &RackKVM{
				Rack: config.Rack{
					Name: "rack 1",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Int64: 1,
					Valid: true,
				},
			})
			table.updates = append(table.updates, &RackKVM{
				Rack: config.Rack{
					Name: "rack 2",
				},
				Id: 2,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: table.updates[0].Name,
				},
				Id: table.updates[0].Id,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: table.updates[1].Name,
				},
				Id: table.updates[1].Id,
				KVMId: sql.NullInt64{
					Int64: 2,
					Valid: true,
				},
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WithArgs(table.updates[0].KVMId, table.updates[0].Id).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateStmt).WithArgs(table.updates[1].KVMId, table.updates[1].Id).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to update rack")
			So(table.updates, ShouldHaveLength, 1)
			So(table.updates[0].KVMId, ShouldResemble, sql.NullInt64{
				Valid: false,
			})
			So(table.current, ShouldHaveLength, 2)
			So(table.current[0].KVMId, ShouldResemble, sql.NullInt64{
				Int64: 1,
				Valid: true,
			})
			So(table.current[1].KVMId, ShouldResemble, sql.NullInt64{
				Int64: 2,
				Valid: true,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.updates = append(table.updates, &RackKVM{
				Rack: config.Rack{
					Name: "rack 1",
				},
				Id: 1,
				KVMId: sql.NullInt64{
					Int64: 1,
					Valid: true,
				},
			})
			table.updates = append(table.updates, &RackKVM{
				Rack: config.Rack{
					Name: "rack 2",
				},
				Id: 2,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: table.updates[0].Name,
				},
				Id: table.updates[0].Id,
				KVMId: sql.NullInt64{
					Valid: false,
				},
			})
			table.current = append(table.current, &RackKVM{
				Rack: config.Rack{
					Name: table.updates[1].Name,
				},
				Id: table.updates[1].Id,
				KVMId: sql.NullInt64{
					Int64: 2,
					Valid: true,
				},
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WithArgs(table.updates[0].KVMId, table.updates[0].Id).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(updateStmt).WithArgs(table.updates[1].KVMId, table.updates[1].Id).WillReturnResult(sqlmock.NewResult(2, 1))
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 2)
			So(table.current[0].KVMId, ShouldResemble, sql.NullInt64{
				Int64: 1,
				Valid: true,
			})
			So(table.current[1].KVMId, ShouldResemble, sql.NullInt64{
				Valid: false,
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
