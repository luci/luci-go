// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package realmsinternals

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/filter/txndefer"
	gaemem "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	testModifiedTS = time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
)

func testAuthVersionedEntityMixin() model.AuthVersionedEntityMixin {
	return model.AuthVersionedEntityMixin{
		ModifiedTS:    testModifiedTS,
		ModifiedBy:    "user:test-modifier@example.com",
		AuthDBRev:     1337,
		AuthDBPrevRev: 1336,
	}
}

func TestGetConfigs(t *testing.T) {
	projectRealmsKey := func(ctx context.Context, project string) *datastore.Key {
		return datastore.NewKey(ctx, "AuthProjectRealms", project, 0, model.RootKey(ctx))
	}

	Convey("projects with config", t, func() {
		ctx := gaemem.Use(context.Background())
		ctx = cfgclient.Use(ctx, &fakeCfgClient{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		datastore.Put(ctx, &model.AuthProjectRealms{
			AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
			Kind:                     "AuthProjectRealms",
			ID:                       "test",
			Parent:                   model.RootKey(ctx),
			Realms:                   []byte{},
			PermsRev:                 "123",
			ConfigRev:                "1234",
		})
		datastore.Put(ctx, &model.AuthProjectRealmsMeta{
			Kind:         "AuthProjectRealmsMeta",
			ID:           "meta",
			Parent:       projectRealmsKey(ctx, "test"),
			PermsRev:     "123",
			ConfigRev:    "1234",
			ConfigDigest: "test-digest",
			ModifiedTS:   testModifiedTS,
		})

		latestExpected := []*RealmsCfgRev{
			{
				projectID:    "@internal",
				configRev:    testRevision,
				configDigest: testContentHash,
				configBody:   []byte{},
				permsRev:     "",
			},
			{
				projectID:    "test-project-a",
				configRev:    testRevision,
				configDigest: testContentHash,
				configBody:   []byte{},
				permsRev:     "",
			},

			{
				projectID:    "test-project-d",
				configRev:    testRevision,
				configDigest: testContentHash,
				configBody:   []byte{},
				permsRev:     "",
			},
		}

		storedExpected := []*RealmsCfgRev{
			{
				projectID:    "meta",
				configRev:    "1234",
				permsRev:     "123",
				configDigest: "test-digest",
			},
		}

		latest, stored, err := GetConfigs(ctx)

		sortRevsByID(latest)
		sortRevsByID(latestExpected)

		So(err, ShouldBeNil)
		So(latest, ShouldResemble, latestExpected)
		So(stored, ShouldResemble, storedExpected)
	})

	Convey("no projects with config", t, func() {
		ctx := gaemem.Use(context.Background())
		ctx = cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{}))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		datastore.Put(ctx, &model.AuthProjectRealms{
			AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
			Kind:                     "AuthProjectRealms",
			ID:                       "test",
			Parent:                   model.RootKey(ctx),
			Realms:                   []byte{},
			PermsRev:                 "123",
			ConfigRev:                "1234",
		})
		datastore.Put(ctx, &model.AuthProjectRealmsMeta{
			Kind:         "AuthProjectRealmsMeta",
			ID:           "meta",
			Parent:       projectRealmsKey(ctx, "test"),
			PermsRev:     "123",
			ConfigRev:    "1234",
			ConfigDigest: "test-digest",
			ModifiedTS:   testModifiedTS,
		})
		_, _, err := GetConfigs(ctx)
		So(err, ShouldErrLike, config.ErrNoConfig)
	})

	Convey("no entity in ds", t, func() {
		ctx := gaemem.Use(context.Background())
		ctx = cfgclient.Use(ctx, &fakeCfgClient{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		_, stored, err := GetConfigs(ctx)
		So(err, ShouldBeNil)
		So(stored, ShouldHaveLength, 0)
	})

}

func sortRevsByID(revs []*RealmsCfgRev) {
	sort.Slice(revs, func(i, j int) bool {
		return revs[i].projectID < revs[j].projectID
	})
}
