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
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListPlatforms(t *testing.T) {
	Convey("listPlatforms", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		columns := []string{"name", "description", "manufacturer"}
		rows := sqlmock.NewRows(columns)

		Convey("query failed", func() {
			selectStmt := `
				^SELECT name, description, manufacturer
				FROM platforms
				WHERE name IN \(\?\)$
			`
			req := &crimson.ListPlatformsRequest{
				Names: []string{"platform"},
			}
			m.ExpectQuery(selectStmt).WillReturnError(fmt.Errorf("error"))
			platforms, err := listPlatforms(c, req)
			So(err, ShouldErrLike, "failed to fetch platforms")
			So(platforms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty request", func() {
			selectStmt := `
				^SELECT name, description, manufacturer
				FROM platforms$
			`
			req := &crimson.ListPlatformsRequest{}
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			platforms, err := listPlatforms(c, req)
			So(err, ShouldBeNil)
			So(platforms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("empty response", func() {
			selectStmt := `
				^SELECT name, description, manufacturer
				FROM platforms
				WHERE name IN \(\?\)$
			`
			req := &crimson.ListPlatformsRequest{
				Names: []string{"platform"},
			}
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0]).WillReturnRows(rows)
			platforms, err := listPlatforms(c, req)
			So(err, ShouldBeNil)
			So(platforms, ShouldBeEmpty)
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("non-empty", func() {
			selectStmt := `
				^SELECT name, description, manufacturer
				FROM platforms
				WHERE name IN \(\?,\?\)$
			`
			req := &crimson.ListPlatformsRequest{
				Names: []string{"platform 1", "platform 2"},
			}
			rows.AddRow("platform 1", "description 1", "manufacturer 1")
			rows.AddRow("platform 2", "description 2", "manufacturer 2")
			m.ExpectQuery(selectStmt).WithArgs(req.Names[0], req.Names[1]).WillReturnRows(rows)
			platforms, err := listPlatforms(c, req)
			So(err, ShouldBeNil)
			So(platforms, ShouldResemble, []*crimson.Platform{
				{
					Name:         "platform 1",
					Description:  "description 1",
					Manufacturer: "manufacturer 1",
				},
				{
					Name:         "platform 2",
					Description:  "description 2",
					Manufacturer: "manufacturer 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})

		Convey("ok", func() {
			selectStmt := `
				^SELECT name, description, manufacturer
				FROM platforms$
			`
			req := &crimson.ListPlatformsRequest{}
			rows.AddRow("platform 1", "description 1", "manufacturer 1")
			rows.AddRow("platform 2", "description 2", "manufacturer 2")
			m.ExpectQuery(selectStmt).WillReturnRows(rows)
			platforms, err := listPlatforms(c, req)
			So(err, ShouldBeNil)
			So(platforms, ShouldResemble, []*crimson.Platform{
				{
					Name:         "platform 1",
					Description:  "description 1",
					Manufacturer: "manufacturer 1",
				},
				{
					Name:         "platform 2",
					Description:  "description 2",
					Manufacturer: "manufacturer 2",
				},
			})
			So(m.ExpectationsWereMet(), ShouldBeNil)
		})
	})
}
