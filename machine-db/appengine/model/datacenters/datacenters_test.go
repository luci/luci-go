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
	"database/sql"
	"testing"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"

	"golang.org/x/net/context"

	_ "github.com/mattn/go-sqlite3"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateDatacenters(t *testing.T) {
	Convey("Ensure", t, func() {
		type datacenter struct {
			Name, Description string
		}

		test := func(datacenters []*config.DatacenterConfig, preexisting []datacenter, expected []datacenter) {
			// Set up a new database.
			db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
			So(err, ShouldBeNil)
			defer db.Close()
			_, err = db.Exec("CREATE TABLE datacenters (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(255), description VARCHAR(255))")
			So(err, ShouldBeNil)
			for _, row := range preexisting {
				_, err = db.Exec("INSERT INTO datacenters (name, description) VALUES (?, ?)", row.Name, row.Description)
				So(err, ShouldBeNil)
			}

			// Manipulate the database.
			So(EnsureDatacenters(database.With(context.Background(), db), datacenters), ShouldBeNil)

			// Make assertions about the state of the database.
			actual, _ := db.Query("SELECT name, description FROM datacenters")
			i := 0
			defer actual.Close()
			for actual.Next() {
				So(i, ShouldBeLessThan, len(expected))
				var name, description string
				err = actual.Scan(&name, &description)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, expected[i].Name)
				So(description, ShouldEqual, expected[i].Description)
				i += 1
			}
			So(i, ShouldEqual, len(expected))
		}

		Convey("empty", func() {
			test(nil, nil, nil)
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
			expected := []datacenter{
				{
					Name:        datacenters[0].Name,
					Description: datacenters[0].Description,
				},
				{
					Name:        datacenters[1].Name,
					Description: datacenters[1].Description,
				},
			}
			test(datacenters, nil, expected)
		})

		Convey("update", func() {
			datacenters := []*config.DatacenterConfig{
				{
					Name:        "datacenter",
					Description: "description 1",
				},
			}
			preexisting := []datacenter{
				{
					Name:        datacenters[0].Name,
					Description: "description 2",
				},
			}
			expected := []datacenter{
				{
					Name:        datacenters[0].Name,
					Description: datacenters[0].Description,
				},
			}
			test(datacenters, preexisting, expected)
		})

		Convey("delete", func() {
			preexisting := []datacenter{
				{
					Name: "datacenter",
				},
			}
			test(nil, preexisting, nil)
		})

		Convey("ok", func() {
			datacenters := []*config.DatacenterConfig{
				{
					Name:        "datacenter",
					Description: "description",
				},
			}
			preexisting := []datacenter{
				{
					Name:        datacenters[0].Name,
					Description: datacenters[0].Description,
				},
			}
			expected := []datacenter{
				{
					Name:        datacenters[0].Name,
					Description: datacenters[0].Description,
				},
			}
			test(datacenters, preexisting, expected)
		})

	})
}
