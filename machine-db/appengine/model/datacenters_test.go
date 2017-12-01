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
		d := &DatacentersTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(d.fetch(c), ShouldErrLike, "failed to select datacenters")
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(d.fetch(c), ShouldBeNil)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			rows.AddRow(1, "datacenter 1", "description 1")
			rows.AddRow(2, "datacenter 2", "description 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(d.fetch(c), ShouldBeNil)
			So(d.current, ShouldResemble, []*Datacenter{
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
		d := &DatacentersTable{}
		c := context.Background()

		Convey("empty", func() {
			d.computeChanges(c, nil)
			So(d.additions, ShouldBeEmpty)
			So(d.updates, ShouldBeEmpty)
			So(d.removals, ShouldBeEmpty)
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
			d.computeChanges(c, dcs)
			So(d.additions, ShouldResemble, []*config.DatacenterConfig{
				{
					Name:        "datacenter 1",
					Description: "description 1",
				},
				{
					Name:        "datacenter 2",
					Description: "description 2",
				},
			})
			So(d.updates, ShouldBeEmpty)
			So(d.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "old description",
				},
				Id: 1,
			})
			dcs := []*config.DatacenterConfig{
				{
					Name:        d.current[0].Name,
					Description: "new description",
				},
			}
			d.computeChanges(c, dcs)
			So(d.additions, ShouldBeEmpty)
			So(d.updates, ShouldHaveLength, 1)
			So(d.updates, ShouldResemble, []*Datacenter{
				{
					DatacenterConfig: config.DatacenterConfig{
						Name:        d.current[0].Name,
						Description: dcs[0].Description,
					},
					Id: d.current[0].Id,
				},
			})
			So(d.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: "datacenter",
				},
				Id: 1,
			})
			d.computeChanges(c, nil)
			So(d.additions, ShouldBeEmpty)
			So(d.updates, ShouldBeEmpty)
			So(d.removals, ShouldResemble, []string{
				d.current[0].Name,
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO datacenters \(name, description\) VALUES \(\?, \?\)$`
		d := &DatacentersTable{}

		Convey("empty", func() {
			So(d.add(c), ShouldBeNil)
			So(d.additions, ShouldBeEmpty)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			d.additions = append(d.additions, &config.DatacenterConfig{
				Name: "datacenter",
			})
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(d.add(c), ShouldErrLike, "failed to prepare statement")
			So(d.additions, ShouldHaveLength, 1)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			d.additions = append(d.additions, &config.DatacenterConfig{
				Name: "datacenter",
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(d.add(c), ShouldErrLike, "failed to add datacenter")
			So(d.additions, ShouldHaveLength, 1)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			d.additions = append(d.additions, &config.DatacenterConfig{
				Name: "datacenter",
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(d.add(c), ShouldBeNil)
			So(d.additions, ShouldBeEmpty)
			So(d.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("remove", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `^DELETE FROM datacenters WHERE name = \?$`
		d := &DatacentersTable{}

		Convey("empty", func() {
			So(d.remove(c), ShouldBeNil)
			So(d.removals, ShouldBeEmpty)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			d.removals = append(d.removals, "datacenter")
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: d.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(d.remove(c), ShouldErrLike, "failed to prepare statement")
			So(d.removals, ShouldHaveLength, 1)
			So(d.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			d.removals = append(d.removals, "datacenter")
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: d.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(d.remove(c), ShouldErrLike, "failed to remove datacenter")
			So(d.removals, ShouldHaveLength, 1)
			So(d.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			d.removals = append(d.removals, "datacenter")
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name: d.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(d.remove(c), ShouldBeNil)
			So(d.removals, ShouldBeEmpty)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("update", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateStmt := `^UPDATE datacenters SET description = \? WHERE id = \?$`
		d := &DatacentersTable{}

		Convey("empty", func() {
			So(d.update(c), ShouldBeNil)
			So(d.updates, ShouldBeEmpty)
			So(d.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			d.updates = append(d.updates, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "new description",
				},
				Id: 1,
			})
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        d.updates[0].Name,
					Description: "old description",
				},
				Id: d.updates[0].Id,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(d.update(c), ShouldErrLike, "failed to prepare statement")
			So(d.updates, ShouldHaveLength, 1)
			So(d.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			d.updates = append(d.updates, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "new description",
				},
				Id: 1,
			})
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        d.updates[0].Name,
					Description: "old description",
				},
				Id: d.updates[0].Id,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(d.update(c), ShouldErrLike, "failed to update datacenter")
			So(d.updates, ShouldHaveLength, 1)
			So(d.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			d.updates = append(d.updates, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        "datacenter",
					Description: "new description",
				},
				Id: 1,
			})
			d.current = append(d.current, &Datacenter{
				DatacenterConfig: config.DatacenterConfig{
					Name:        d.updates[0].Name,
					Description: "old description",
				},
				Id: d.updates[0].Id,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(d.update(c), ShouldBeNil)
			So(d.updates, ShouldBeEmpty)
			So(d.current, ShouldHaveLength, 1)
			So(d.current[0].Description, ShouldEqual, "new description")
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
