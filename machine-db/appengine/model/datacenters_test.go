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

func TestDatacenters(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `^SELECT id, name, description FROM datacenters$`
		columns := []string{"id", "name", "description"}
		rows := sqlmock.NewRows(columns)
		table := &DatacentersTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(table.fetch(c), ShouldErrLike, "failed to select datacenters")
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
			rows.AddRow(1, "datacenter 1", "description 1")
			rows.AddRow(2, "datacenter 2", "description 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldResemble, []*Datacenter{
				{
					DatacenterConfig: config.DatacenterConfig{
						Name:        "datacenter 1",
						Description: "description 1",
					},
					Id: 1,
				},
				{
					DatacenterConfig: config.DatacenterConfig{
						Name:        "datacenter 2",
						Description: "description 2",
					},
					Id: 2,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		table := &DatacentersTable{}
		c := context.Background()

		Convey("empty", func() {
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("addition", func() {
			dcs := []*config.DatacenterConfig{
				{
					Name:        "datacenter 1",
					Description: "description 1",
				},
				{
					Name:        "datacenter 2",
					Description: "description 2",
				},
			}
			table.computeChanges(c, dcs)
			So(table.additions, ShouldResemble, []*Datacenter{
				{
					DatacenterConfig: config.DatacenterConfig{
						Name:        "datacenter 1",
						Description: "description 1",
					},
				},
				{
					DatacenterConfig: config.DatacenterConfig{
						Name:        "datacenter 2",
						Description: "description 2",
					},
				},
			})
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "old description",
				},
				Id: 1,
			})
			dcs := []*config.DatacenterConfig{
				{
					Name:        table.current[0].Name,
					Description: "new description",
				},
			}
			table.computeChanges(c, dcs)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldHaveLength, 1)
			So(table.updates, ShouldResemble, []*Datacenter{
				{
					DatacenterConfig: config.DatacenterConfig{
						Name:        table.current[0].Name,
						Description: dcs[0].Description,
					},
					Id: table.current[0].Id,
				},
			})
			So(table.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
				Id: 1,
			})
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldResemble, []*Datacenter{
				{
					DatacenterConfig: config.DatacenterConfig{
						Name: table.current[0].Name,
					},
					Id: 1,
				},
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO datacenters \(name, description\) VALUES \(\?, \?\)$`
		table := &DatacentersTable{}

		Convey("empty", func() {
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.additions = append(table.additions, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
			})
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to prepare statement")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.additions = append(table.additions, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to add datacenter")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.additions = append(table.additions, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
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
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `^DELETE FROM datacenters WHERE id = \?$`
		table := &DatacentersTable{}

		Convey("empty", func() {
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.removals = append(table.removals, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
				Id: 1,
			})
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
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
			table.removals = append(table.removals, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
				Id: 1,
			})
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: table.removals[0].Name,
				},
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to remove datacenter")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.removals = append(table.removals, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
				Id: 1,
			})
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
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
		updateStmt := `^UPDATE datacenters SET description = \? WHERE id = \?$`
		table := &DatacentersTable{}

		Convey("empty", func() {
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.updates = append(table.updates, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "new description",
				},
				Id: 1,
			})
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        table.updates[0].Name,
					Description: "old description",
				},
				Id: table.updates[0].Id,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to prepare statement")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.updates = append(table.updates, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "new description",
				},
				Id: 1,
			})
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        table.updates[0].Name,
					Description: "old description",
				},
				Id: table.updates[0].Id,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to update datacenter")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.updates = append(table.updates, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "new description",
				},
				Id: 1,
			})
			table.current = append(table.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        table.updates[0].Name,
					Description: "old description",
				},
				Id: table.updates[0].Id,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(table.current[0].Description, ShouldEqual, "new description")
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
