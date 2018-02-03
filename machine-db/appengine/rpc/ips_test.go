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

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListFreeIPs(t *testing.T) {
	Convey("listFreeIPs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT ipv4
			FROM ips
			WHERE vlan_id = \? AND hostname_id IS NULL
			LIMIT \?$
		`
		columns := []string{"ipv4"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			ips, err := listFreeIPs(c, 1, 10)
			So(err, ShouldErrLike, "failed to fetch IP addresses")
			So(ips, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			m.ExpectQuery(selectStmt).WithArgs(1, 10).WillReturnRows(rows)
			ips, err := listFreeIPs(c, 1, 10)
			So(err, ShouldBeNil)
			So(ips, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			rows.AddRow(1)
			rows.AddRow(2)
			m.ExpectQuery(selectStmt).WithArgs(1, 10).WillReturnRows(rows)
			ips, err := listFreeIPs(c, 1, 10)
			So(err, ShouldBeNil)
			So(ips, ShouldResemble, []*crimson.IP{
				{
					Ipv4: "0.0.0.1",
					Vlan: 1,
				},
				{
					Ipv4: "0.0.0.2",
					Vlan: 1,
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
