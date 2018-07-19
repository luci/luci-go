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
	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"

	"go.chromium.org/luci/machine-db/appengine/database"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeleteHost(t *testing.T) {
	Convey("deleteHost", t, func() {
		db, m, _ := sqlmock.New()
		defer db.Close()
		c := database.With(context.Background(), db)
		deleteStmt := `
			^DELETE FROM hostnames WHERE name = \?$
		`

		Convey("query failed", func() {
			m.ExpectExec(deleteStmt).WithArgs("host").WillReturnError(fmt.Errorf("error"))
			So(deleteHost(c, "host"), ShouldErrLike, "failed to delete host")
		})

		Convey("referenced", func() {
			m.ExpectExec(deleteStmt).WithArgs("host").WillReturnError(&mysql.MySQLError{Number: mysqlerr.ER_ROW_IS_REFERENCED_2, Message: "`physical_host_id`"})
			So(deleteHost(c, "host"), ShouldErrLike, "delete entities referencing this host first")
		})

		Convey("invalid", func() {
			m.ExpectExec(deleteStmt).WithArgs("host").WillReturnResult(sqlmock.NewResult(1, 0))
			So(deleteHost(c, "host"), ShouldErrLike, "does not exist")
		})

		Convey("ok", func() {
			m.ExpectExec(deleteStmt).WithArgs("host").WillReturnResult(sqlmock.NewResult(1, 1))
			So(deleteHost(c, "host"), ShouldBeNil)
		})
	})
}
