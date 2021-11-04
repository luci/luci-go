// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authdb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAuthDBProvider(t *testing.T) {
	t.Parallel()

	Convey("AuthDBProvider works", t, func() {
		ctx := memory.Use(context.Background())
		authDB := &AuthDBProvider{}

		putRev := func(rev int64, members []string) {
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				globals := &model.AuthGlobalConfig{}
				state := &model.AuthReplicationState{
					AuthDBRev: rev,
					Parent:    model.RootKey(ctx),
				}
				group := &model.AuthGroup{
					ID:      "test-group",
					Parent:  model.RootKey(ctx),
					Members: members,
				}
				return datastore.Put(ctx, globals, state, group)
			}, nil)
			So(err, ShouldBeNil)
		}

		// Initial revision.
		putRev(1000, []string{"user:a@example.com"})

		// Got it.
		db1, err := authDB.GetAuthDB(ctx)
		So(err, ShouldBeNil)
		So(authdb.Revision(db1), ShouldEqual, 1000)

		// Works.
		yes, err := db1.IsMember(ctx, "user:a@example.com", []string{"test-group"})
		So(err, ShouldBeNil)
		So(yes, ShouldBeTrue)

		// Calling again returns the exact same object.
		db2, err := authDB.GetAuthDB(ctx)
		So(err, ShouldBeNil)
		So(db2, ShouldEqual, db1)

		// Updated.
		putRev(1001, nil)

		// Got the new one.
		db3, err := authDB.GetAuthDB(ctx)
		So(err, ShouldBeNil)
		So(authdb.Revision(db3), ShouldEqual, 1001)

		// The group there is updated too.
		yes, err = db3.IsMember(ctx, "user:a@example.com", []string{"test-group"})
		So(err, ShouldBeNil)
		So(yes, ShouldBeFalse)
	})
}
