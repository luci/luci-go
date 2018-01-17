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
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIPs(t *testing.T) {
	Convey("fetch", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `^SELECT id, ipv4, vlan_id FROM ips$`
		columns := []string{"id", "ipv4", "vlan_id"}
		rows := sqlmock.NewRows(columns)
		table := &IPsTable{}

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			So(table.fetch(c), ShouldErrLike, "failed to select IP addresses")
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
			rows.AddRow(1, 100, 10)
			rows.AddRow(2, 200, 20)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			So(table.fetch(c), ShouldBeNil)
			So(table.current, ShouldResemble, []*IP{
				{
					Id:     1,
					IPv4:   100,
					VLANId: 10,
				},
				{
					Id:     2,
					IPv4:   200,
					VLANId: 20,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})

	Convey("computeChanges", t, func() {
		table := &IPsTable{}
		c := context.Background()

		Convey("empty", func() {
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("addition", func() {
			vlans := []*config.VLAN{
				{
					Id: 1,
					CidrBlock: []string{
						"127.0.0.1/31",
					},
				},
				{
					Id: 2,
					CidrBlock: []string{
						"192.168.0.0/32",
						"192.169.0.0/31",
					},
				},
			}
			table.computeChanges(c, vlans)
			So(table.additions, ShouldResemble, []*IP{
				{
					IPv4:   2130706432,
					VLANId: 1,
				},
				{
					IPv4:   2130706433,
					VLANId: 1,
				},
				{
					IPv4:   3232235520,
					VLANId: 2,
				},
				{
					IPv4:   3232301056,
					VLANId: 2,
				},
				{
					IPv4:   3232301057,
					VLANId: 2,
				},
			})
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldBeEmpty)
		})

		Convey("update", func() {
			table.current = append(table.current, &IP{
				Id:     1,
				IPv4:   common.IPv4(2130706432),
				VLANId: 10,
			})
			vlans := []*config.VLAN{
				{
					Id: 20,
					CidrBlock: []string{
						"127.0.0.0/32",
					},
				},
			}
			table.computeChanges(c, vlans)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldHaveLength, 1)
			So(table.updates, ShouldResemble, []*IP{
				{
					Id:     table.current[0].Id,
					IPv4:   table.current[0].IPv4,
					VLANId: 20,
				},
			})
			So(table.removals, ShouldBeEmpty)
		})

		Convey("removal", func() {
			table.current = append(table.current, &IP{
				Id: 1,
			})
			table.computeChanges(c, nil)
			So(table.additions, ShouldBeEmpty)
			So(table.updates, ShouldBeEmpty)
			So(table.removals, ShouldResemble, []*IP{
				{
					Id: 1,
				},
			})
		})
	})

	Convey("add", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		insertStmt := `^INSERT INTO ips \(ipv4, vlan_id\) VALUES \(\?, \?\)$`
		table := &IPsTable{}

		Convey("empty", func() {
			So(table.add(c), ShouldBeNil)
			So(table.additions, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.additions = append(table.additions, &IP{
				IPv4:   100,
				VLANId: 1,
			})
			m.ExpectPrepare(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to prepare statement")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.additions = append(table.additions, &IP{
				IPv4:   100,
				VLANId: 1,
			})
			m.ExpectPrepare(insertStmt)
			m.ExpectExec(insertStmt).WillReturnError(fmt.Errorf("error"))
			So(table.add(c), ShouldErrLike, "failed to add IP address")
			So(table.additions, ShouldHaveLength, 1)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.additions = append(table.additions, &IP{
				IPv4:   100,
				VLANId: 1,
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
		deleteStmt := `^DELETE FROM ips WHERE id = \?$`
		table := &IPsTable{}

		Convey("empty", func() {
			So(table.remove(c), ShouldBeNil)
			So(table.removals, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.removals = append(table.removals, &IP{
				Id: 1,
			})
			table.current = append(table.current, &IP{
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to prepare statement")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.removals = append(table.removals, &IP{
				Id: 1,
			})
			table.current = append(table.current, &IP{
				Id: table.removals[0].Id,
			})
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WillReturnError(fmt.Errorf("error"))
			So(table.remove(c), ShouldErrLike, "failed to remove IP address")
			So(table.removals, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.removals = append(table.removals, &IP{
				Id: 1,
			})
			table.current = append(table.current, &IP{
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
		updateStmt := `^UPDATE ips SET vlan_id = \? WHERE id = \?$`
		table := &IPsTable{}

		Convey("empty", func() {
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("prepare failed", func() {
			table.updates = append(table.updates, &IP{
				Id:     1,
				VLANId: 200,
			})
			table.current = append(table.current, &IP{
				Id:     table.updates[0].Id,
				VLANId: 100,
			})
			m.ExpectPrepare(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to prepare statement")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("exec failed", func() {
			table.updates = append(table.updates, &IP{
				Id:     1,
				VLANId: 200,
			})
			table.current = append(table.current, &IP{
				Id:     table.updates[0].Id,
				VLANId: 100,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnError(fmt.Errorf("error"))
			So(table.update(c), ShouldErrLike, "failed to update IP address")
			So(table.updates, ShouldHaveLength, 1)
			So(table.current, ShouldHaveLength, 1)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			table.updates = append(table.updates, &IP{
				Id:     1,
				VLANId: 200,
			})
			table.current = append(table.current, &IP{
				Id:     table.updates[0].Id,
				VLANId: 100,
			})
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WillReturnResult(sqlmock.NewResult(1, 1))
			So(table.update(c), ShouldBeNil)
			So(table.updates, ShouldBeEmpty)
			So(table.current, ShouldHaveLength, 1)
			So(table.current[0].VLANId, ShouldEqual, 200)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
