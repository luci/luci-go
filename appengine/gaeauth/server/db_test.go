// Copyright 2015 The LUCI Authors.
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

package server

import (
	"context"
	"fmt"
	"testing"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaeauth/server/internal/authdbimpl"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/service"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetAuthDB(t *testing.T) {
	Convey("Unconfigured", t, func() {
		c := gaetesting.TestingContext()
		authDB, err := GetAuthDB(c, nil)
		So(err, ShouldBeNil)
		So(authDB, ShouldHaveSameTypeAs, devServerDB{})
	})

	Convey("Reuses instance if no changes", t, func() {
		c := gaetesting.TestingContext()

		bumpAuthDB(c, 123)
		authDB, err := GetAuthDB(c, nil)
		So(err, ShouldBeNil)
		So(authDB, ShouldHaveSameTypeAs, &authdb.SnapshotDB{})
		So(authDB.(*authdb.SnapshotDB).Rev, ShouldEqual, 123)

		newOne, err := GetAuthDB(c, authDB)
		So(err, ShouldBeNil)
		So(newOne, ShouldEqual, authDB) // exact same pointer

		bumpAuthDB(c, 124)
		anotherOne, err := GetAuthDB(c, authDB)
		So(err, ShouldBeNil)
		So(anotherOne, ShouldHaveSameTypeAs, &authdb.SnapshotDB{})
		So(anotherOne.(*authdb.SnapshotDB).Rev, ShouldEqual, 124)
	})
}

///

func strPtr(s string) *string {
	return &s
}

func bumpAuthDB(c context.Context, rev int64) {
	blob, err := service.DeflateAuthDB(&protocol.AuthDB{
		OauthClientId:     strPtr(fmt.Sprintf("client-id-for-rev-%d", rev)),
		OauthClientSecret: strPtr("secret"),
	})
	if err != nil {
		panic(err)
	}
	info := authdbimpl.SnapshotInfo{
		AuthServiceURL: "https://fake-auth-service",
		Rev:            rev,
	}
	if err = ds.Put(c, &info); err != nil {
		panic(err)
	}
	err = ds.Put(c, &authdbimpl.Snapshot{
		ID:             info.GetSnapshotID(),
		AuthDBDeflated: blob,
	})
	if err != nil {
		panic(err)
	}
}
