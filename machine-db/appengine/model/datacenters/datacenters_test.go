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
	columns := []string{"id", "name", "descriptions"}

	Convey("EnsureDatacenters: empty", t, func() {
		db, m, e := sqlmock.New()
		defer db.Close()
		So(db, ShouldNotBeNil)
		So(m, ShouldNotBeNil)
		So(e, ShouldBeNil)

		datacenters := []*config.DatacenterConfig{}

		m.ExpectPrepare("UPDATE")
		m.ExpectPrepare("DELETE")
		m.ExpectPrepare("INSERT")
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns))

		e = EnsureDatacenters(database.With(context.Background(), db), datacenters)
		So(e, ShouldBeNil)
	})

	Convey("EnsureDatacenters: insert", t, func() {
		db, m, e := sqlmock.New()
		defer db.Close()
		So(db, ShouldNotBeNil)
		So(m, ShouldNotBeNil)
		So(e, ShouldBeNil)

		datacenters := []*config.DatacenterConfig{
			{
				Name:        "datacenter 1",
				Description: "description",
			},
			{
				Name: "datacenter 2",
			},
		}

		m.ExpectPrepare("UPDATE")
		m.ExpectPrepare("DELETE")
		m.ExpectPrepare("INSERT")
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns))
		m.ExpectExec("INSERT").WithArgs(datacenters[0].Name, datacenters[0].Description).WillReturnResult(sqlmock.NewResult(1, 1))
		m.ExpectExec("INSERT").WithArgs(datacenters[1].Name, datacenters[1].Description).WillReturnResult(sqlmock.NewResult(2, 1))

		e = EnsureDatacenters(database.With(context.Background(), db), datacenters)
		So(e, ShouldBeNil)
	})

	Convey("EnsureDatacenters: update", t, func() {
		db, m, e := sqlmock.New()
		defer db.Close()
		So(db, ShouldNotBeNil)
		So(m, ShouldNotBeNil)
		So(e, ShouldBeNil)

		datacenters := []*config.DatacenterConfig{
			{
				Name: "datacenter",
			},
		}

		m.ExpectPrepare("UPDATE")
		m.ExpectPrepare("DELETE")
		m.ExpectPrepare("INSERT")
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, datacenters[0].Name, "description"))
		m.ExpectPrepare("UPDATE")
		m.ExpectExec("UPDATE").WithArgs(datacenters[0].Description, 1).WillReturnResult(sqlmock.NewResult(1, 1))

		e = EnsureDatacenters(database.With(context.Background(), db), datacenters)
		So(e, ShouldBeNil)
	})

	Convey("EnsureDatacenters: delete", t, func() {
		db, m, e := sqlmock.New()
		defer db.Close()
		So(db, ShouldNotBeNil)
		So(m, ShouldNotBeNil)
		So(e, ShouldBeNil)

		datacenters := []*config.DatacenterConfig{}

		m.ExpectPrepare("UPDATE")
		m.ExpectPrepare("DELETE")
		m.ExpectPrepare("INSERT")
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "datacenter", "description"))
		m.ExpectPrepare("DELETE")
		m.ExpectExec("DELETE").WithArgs(1).WillReturnResult(sqlmock.NewResult(1, 1))

		e = EnsureDatacenters(database.With(context.Background(), db), datacenters)
		So(e, ShouldBeNil)
	})

	Convey("EnsureDatacenters: ok", t, func() {
		db, m, e := sqlmock.New()
		defer db.Close()
		So(db, ShouldNotBeNil)
		So(m, ShouldNotBeNil)
		So(e, ShouldBeNil)

		datacenters := []*config.DatacenterConfig{
			{
				Name:        "datacenter",
				Description: "description",
			},
		}

		m.ExpectPrepare("UPDATE")
		m.ExpectPrepare("DELETE")
		m.ExpectPrepare("INSERT")
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, datacenters[0].Name, datacenters[0].Description))

		e = EnsureDatacenters(database.With(context.Background(), db), datacenters)
		So(e, ShouldBeNil)
	})
}
