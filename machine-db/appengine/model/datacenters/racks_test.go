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

package datacenters

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRacks(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `^SELECT racks.id, racks.name, racks.description, datacenters.name FROM racks, datacenters WHERE racks.datacenter_id = datacenters.id$`
		columns := []string{"racks.id", "racks.name", "racks.description", "racks.datacenter"}
		rows := sqlmock.NewRows(columns)
		r := &Racks{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(r.fetch(c), ShouldErrLike, "failed to select racks")
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(r.fetch(c), ShouldBeNil)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			rows.AddRow(1, "rack 1", "description 1", "datacenter 1")
			rows.AddRow(2, "rack 2", "description 2", "datacenter 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(r.fetch(c), ShouldBeNil)
			So(len(r.racks), ShouldEqual, 2)
			So(r.racks[0].Id, ShouldEqual, 1)
			So(r.racks[0].Name, ShouldEqual, "rack 1")
			So(r.racks[0].Description, ShouldEqual, "description 1")
			So(r.racks[0].Datacenter, ShouldEqual, "datacenter 1")
			So(r.racks[1].Id, ShouldEqual, 2)
			So(r.racks[1].Name, ShouldEqual, "rack 2")
			So(r.racks[1].Description, ShouldEqual, "description 2")
			So(r.racks[1].Datacenter, ShouldEqual, "datacenter 2")
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		c := context.Background()
		r := &Racks{}

		Convey("empty", func() {
			r.computeChanges(c, nil)
			So(len(r.additions), ShouldEqual, 0)
			So(len(r.updates), ShouldEqual, 0)
			So(len(r.removals), ShouldEqual, 0)
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
			r.computeChanges(c, dcs)
			So(len(r.additions), ShouldEqual, 2)
			So(r.additions[0].Name, ShouldEqual, dcs[0].Rack[0].Name)
			So(r.additions[0].Description, ShouldEqual, dcs[0].Rack[0].Description)
			So(r.additions[1].Name, ShouldEqual, dcs[1].Rack[0].Name)
			So(r.additions[1].Description, ShouldEqual, dcs[1].Rack[0].Description)
			So(len(r.updates), ShouldEqual, 0)
			So(len(r.removals), ShouldEqual, 0)
		})

		Convey("update", func() {
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Id:         1,
				Datacenter: "old datacenter",
			})
			dcs := []*config.DatacenterConfig{
				{
					Name: "new datacenter",
					Rack: []*config.RackConfig{
						{
							Name: "rack",
						},
					},
				},
			}
			r.computeChanges(c, dcs)
			So(len(r.additions), ShouldEqual, 0)
			So(len(r.updates), ShouldEqual, 1)
			So(r.updates[0].Name, ShouldEqual, dcs[0].Rack[0].Name)
			So(r.updates[0].Description, ShouldEqual, dcs[0].Rack[0].Description)
			So(r.updates[0].Id, ShouldEqual, r.racks[0].Id)
			So(len(r.removals), ShouldEqual, 0)
		})

		Convey("removal", func() {
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Id:         1,
				Datacenter: "datacenter",
			})
			r.computeChanges(c, nil)
			So(len(r.additions), ShouldEqual, 0)
			So(len(r.updates), ShouldEqual, 0)
			So(len(r.removals), ShouldEqual, 1)
			So(r.removals[0], ShouldEqual, r.racks[0].Name)
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO racks \(name, description, datacenter_id\) VALUES \(\?, \?, \?\)$`
		r := &Racks{
			datacenters: make(map[string]int64, 0),
		}

		Convey("empty", func() {
			So(r.add(c), ShouldBeNil)
			So(len(r.additions), ShouldEqual, 0)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Datacenter: "datacenter",
			})
			r.datacenters[r.additions[0].Datacenter] = 1
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(r.add(c), ShouldErrLike, "failed to prepare statement")
			So(len(r.additions), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("determine datacenter failed", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Datacenter: "datacenter",
			})
			m.ExpectPrepare(insertStmt)
			So(r.add(c), ShouldErrLike, "failed to determine datacenter")
			So(len(r.additions), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Datacenter: "datacenter",
			})
			r.datacenters[r.additions[0].Datacenter] = 1
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(r.add(c), ShouldErrLike, "failed to add rack")
			So(len(r.additions), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			r.additions = append(r.additions, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Datacenter: "datacenter",
			})
			r.datacenters[r.additions[0].Datacenter] = 1
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(r.add(c), ShouldBeNil)
			So(len(r.additions), ShouldEqual, 0)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("remove", t, func() {
		db, m, _ := sqlmock.New()
		defer func() { So(m.ExpectationsWereMet(), ShouldBeNil) }()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `^DELETE FROM racks WHERE name = \?$`
		r := &Racks{}

		Convey("empty", func() {
			So(r.remove(c), ShouldBeNil)
			So(len(r.removals), ShouldEqual, 0)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			r.removals = append(r.removals, "rack")
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(r.remove(c), ShouldErrLike, "failed to prepare statement")
			So(len(r.removals), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			r.removals = append(r.removals, "rack")
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(r.remove(c), ShouldErrLike, "failed to remove rack")
			So(len(r.removals), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			r.removals = append(r.removals, "rack")
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.removals[0],
				},
				Id: 1,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(r.remove(c), ShouldBeNil)
			So(len(r.removals), ShouldEqual, 0)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("update", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		updateStmt := `^UPDATE racks SET description = \?, datacenter_id = \? WHERE id = \?$`
		r := &Racks{
			datacenters: make(map[string]int64, 0),
		}

		Convey("empty", func() {
			So(r.update(c), ShouldBeNil)
			So(len(r.updates), ShouldEqual, 0)
			So(len(r.racks), ShouldEqual, 0)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Id:         1,
				Datacenter: "new datacenter",
			})
			r.datacenters[r.updates[0].Datacenter] = 2
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:         r.updates[0].Id,
				Datacenter: "old datacenter",
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(r.update(c), ShouldErrLike, "failed to prepare statement")
			So(len(r.updates), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("determine datacenter failed", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Id:         1,
				Datacenter: "datacenter",
			})
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:         r.updates[0].Id,
				Datacenter: "old datacenter",
			})
			m.ExpectPrepare(updateStmt)
			So(r.update(c), ShouldErrLike, "failed to determine datacenter")
			So(len(r.updates), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Id:         1,
				Datacenter: "datacenter",
			})
			r.datacenters[r.updates[0].Datacenter] = 2
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:         r.updates[0].Id,
				Datacenter: "old datacenter",
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(r.update(c), ShouldErrLike, "failed to update rack")
			So(len(r.updates), ShouldEqual, 1)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			r.updates = append(r.updates, &Rack{
				RackConfig: &config.RackConfig{
					Name: "rack",
				},
				Id:         1,
				Datacenter: "datacenter",
			})
			r.datacenters[r.updates[0].Datacenter] = 2
			r.racks = append(r.racks, &Rack{
				RackConfig: &config.RackConfig{
					Name: r.updates[0].Name,
				},
				Id:         r.updates[0].Id,
				Datacenter: "old datacenter",
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(r.update(c), ShouldBeNil)
			So(len(r.updates), ShouldEqual, 0)
			So(len(r.racks), ShouldEqual, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
