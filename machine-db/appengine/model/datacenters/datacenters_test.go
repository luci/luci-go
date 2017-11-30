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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateDatacenters(t *testing.T) {
	Convey("EnsureDatacenters", t, func() {
		db, m, _ := sqlmock.New()
		defer func() {
			So(m.ExpectationsWereMet(), ShouldBeNil)
		}()
		defer db.Close()
		c := database.With(context.Background(), db)

		updateStmt := `^UPDATE datacenters SET description = \? WHERE id = \?$`
		deleteStmt := `^DELETE FROM datacenters WHERE id = \?$`
		insertStmt := `^INSERT INTO datacenters \(name, description\) VALUES \(\?, \?\)$`
		selectStmt := `^SELECT id, name, description FROM datacenters$`
		m.ExpectPrepare(updateStmt)
		m.ExpectPrepare(deleteStmt)
		m.ExpectPrepare(insertStmt)
		query := m.ExpectQuery(selectStmt)
		rows := sqlmock.NewRows([]string{"id", "name", "description"})

		test := func(datacenters []*config.DatacenterConfig) {
			query.WillReturnRows(rows)
			So(EnsureDatacenters(c, datacenters), ShouldBeNil)
		}

		Convey("empty", func() {
			test(nil)
		})

		Convey("insert", func() {
			datacenters := []*config.DatacenterConfig{
				{
					Name:        "datacenter 1",
					Description: "description",
				},
				{
					Name:        "datacenter 2",
					Description: "description",
				},
			}
			m.ExpectExec(insertStmt).WithArgs(datacenters[0].Name, datacenters[0].Description).WillReturnResult(sqlmock.NewResult(1, 1))
			m.ExpectExec(insertStmt).WithArgs(datacenters[1].Name, datacenters[1].Description).WillReturnResult(sqlmock.NewResult(2, 1))
			test(datacenters)
		})

		Convey("update", func() {
			datacenters := []*config.DatacenterConfig{
				{
					Name: "datacenter",
				},
			}
			rows.AddRow(1, datacenters[0].Name, "description")
			m.ExpectPrepare(updateStmt)
			m.ExpectExec(updateStmt).WithArgs(datacenters[0].Description, 1).WillReturnResult(sqlmock.NewResult(1, 1))
			test(datacenters)
		})

		Convey("delete", func() {
			datacenters := []*config.DatacenterConfig{}
			rows.AddRow(1, "datacenter", "description")
			m.ExpectPrepare(deleteStmt)
			m.ExpectExec(deleteStmt).WithArgs(1).WillReturnResult(sqlmock.NewResult(1, 1))
			test(datacenters)
		})

		Convey("ok", func() {
			datacenters := []*config.DatacenterConfig{
				{
					Name:        "datacenter",
					Description: "description",
				},
			}
			rows.AddRow(1, datacenters[0].Name, datacenters[0].Description)
			test(datacenters)
		})
	})
}
