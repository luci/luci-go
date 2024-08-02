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

	"go.chromium.org/luci/appengine/gaeauth/server/internal/authdbimpl"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/service"
	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestGetAuthDB(t *testing.T) {
	ftt.Run("Unconfigured", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		authDB, err := GetAuthDB(ctx, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, authDB, should.HaveType[authdb.DevServerDB])
	})

	ftt.Run("Reuses instance if no changes", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()

		bumpAuthDB(ctx, 123)
		authDB, err := GetAuthDB(ctx, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, authDB, should.HaveType[*authdb.SnapshotDB])
		assert.Loosely(t, authDB.(*authdb.SnapshotDB).Rev, should.Equal(123))

		newOne, err := GetAuthDB(ctx, authDB)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, newOne, should.Equal(authDB)) // exact same pointer

		bumpAuthDB(ctx, 124)
		anotherOne, err := GetAuthDB(ctx, authDB)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, anotherOne, should.HaveType[*authdb.SnapshotDB])
		assert.Loosely(t, anotherOne.(*authdb.SnapshotDB).Rev, should.Equal(124))
	})
}

///

func bumpAuthDB(ctx context.Context, rev int64) {
	blob, err := service.DeflateAuthDB(&protocol.AuthDB{
		OauthClientId:     fmt.Sprintf("client-id-for-rev-%d", rev),
		OauthClientSecret: "secret",
	})
	if err != nil {
		panic(err)
	}
	info := authdbimpl.SnapshotInfo{
		AuthServiceURL: "https://fake-auth-service",
		Rev:            rev,
	}
	if err = ds.Put(ctx, &info); err != nil {
		panic(err)
	}
	err = ds.Put(ctx, &authdbimpl.Snapshot{
		ID:             info.GetSnapshotID(),
		AuthDBDeflated: blob,
	})
	if err != nil {
		panic(err)
	}
}
