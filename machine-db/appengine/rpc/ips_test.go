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

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListIPs(t *testing.T) {
	Convey("listIPs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT i.ipv4, i.vlan_id, h.name
			FROM ips i
			LEFT OUTER JOIN hostnames h ON i.hostname_id = h.id$
		`
		columns := []string{"i.ipv4", "i.vlan_id", "h.name"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			ipv4s := stringset.NewFromSlice("0.0.0.3")
			vlans := map[int64]struct{}{30: {}}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			ips, err := listIPs(c, ipv4s, vlans)
			So(err, ShouldErrLike, "failed to fetch IP addresses")
			So(ips, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			ipv4s := stringset.NewFromSlice("0.0.0.3")
			vlans := map[int64]struct{}{30: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			ips, err := listIPs(c, ipv4s, vlans)
			So(err, ShouldBeNil)
			So(ips, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			ipv4s := stringset.NewFromSlice("0.0.0.3")
			vlans := map[int64]struct{}{3: {}}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			rows.AddRow(1, 10, "host 1")
			rows.AddRow(2, 20, "host 2")
			ips, err := listIPs(c, ipv4s, vlans)
			So(err, ShouldBeNil)
			So(ips, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			ipv4s := stringset.NewFromSlice("0.0.0.3")
			vlans := map[int64]struct{}{30: {}}
			rows.AddRow(1, 10, "host 1")
			rows.AddRow(2, 20, "host 2")
			rows.AddRow(3, 30, "host 3")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			ips, err := listIPs(c, ipv4s, vlans)
			So(err, ShouldBeNil)
			So(ips, ShouldResemble, []*crimson.IP{
				{
					Ipv4:     "0.0.0.3",
					Vlan:     30,
					Hostname: "host 3",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			ipv4s := stringset.New(0)
			vlans := map[int64]struct{}{}
			rows.AddRow(1, 10, "host 1")
			rows.AddRow(2, 20, nil)
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			ips, err := listIPs(c, ipv4s, vlans)
			So(err, ShouldBeNil)
			So(ips, ShouldResemble, []*crimson.IP{
				{
					Ipv4:     "0.0.0.1",
					Vlan:     10,
					Hostname: "host 1",
				},
				{
					Ipv4:     "0.0.0.2",
					Vlan:     20,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
