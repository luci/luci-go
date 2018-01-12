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

func TestGetVLANs(t *testing.T) {
	Convey("getVLANs", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		selectStmt := `
			^SELECT v.id, v.alias
			FROM vlans v$
		`
		columns := []string{"v.id", "v.alias"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			ids := map[int64]struct{}{0: {}}
			aliases := stringset.NewFromSlice("vlan")
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldErrLike, "failed to fetch VLANs")
			So(vlans, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty", func() {
			ids := map[int64]struct{}{0: {}}
			aliases := stringset.NewFromSlice("vlan")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldBeNil)
			So(vlans, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no matches", func() {
			ids := map[int64]struct{}{0: {}}
			aliases := stringset.NewFromSlice("vlan")
			rows.AddRow(1, "vlan 1")
			rows.AddRow(2, "vlan 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldBeNil)
			So(vlans, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("matches", func() {
			ids := map[int64]struct{}{2: {}}
			aliases := stringset.NewFromSlice("vlan 3")
			rows.AddRow(1, "vlan 1")
			rows.AddRow(2, "vlan 2")
			rows.AddRow(3, "vlan 3")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldBeNil)
			So(vlans, ShouldResemble, []*crimson.VLAN{
				{
					Id:    2,
					Alias: "vlan 2",
				},
				{
					Id:    3,
					Alias: "vlan 3",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no IDs", func() {
			ids := make(map[int64]struct{}, 0)
			aliases := stringset.NewFromSlice("vlan 3")
			rows.AddRow(1, "vlan 1")
			rows.AddRow(2, "vlan 2")
			rows.AddRow(3, "vlan 3")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldBeNil)
			So(vlans, ShouldResemble, []*crimson.VLAN{
				{
					Id:    3,
					Alias: "vlan 3",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("no aliases", func() {
			ids := map[int64]struct{}{2: {}}
			aliases := stringset.New(0)
			rows.AddRow(1, "vlan 1")
			rows.AddRow(2, "vlan 2")
			rows.AddRow(3, "vlan 3")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldBeNil)
			So(vlans, ShouldResemble, []*crimson.VLAN{
				{
					Id:    2,
					Alias: "vlan 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			ids := make(map[int64]struct{}, 0)
			aliases := stringset.New(0)
			rows.AddRow(1, "vlan 1")
			rows.AddRow(2, "vlan 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			vlans, err := getVLANs(c, ids, aliases)
			So(err, ShouldBeNil)
			So(vlans, ShouldResemble, []*crimson.VLAN{
				{
					Id:    1,
					Alias: "vlan 1",
				},
				{
					Id:    2,
					Alias: "vlan 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
