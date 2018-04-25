// Copyright 2018 The LUCI Authors.
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

package buildbucket

import (
	"testing"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	swarmbucket "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPools(t *testing.T) {
	t.Parallel()
	Convey(`Test Pool Information`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		c, _ = testclock.UseTime(c, RefTime)
		c = testconfig.WithCommonClient(c, memcfg.New(bktConfigFull))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		})
		dim1 := []string{"egg:white", "pants:green"}
		dim2 := []string{"egg:blue", "pants:green"}
		getBuilderMsg := &swarmbucket.SwarmingSwarmbucketApiGetBuildersResponseMessage{
			Buckets: []*swarmbucket.SwarmingSwarmbucketApiBucketMessage{
				{
					SwarmingHostname: "swarming.example.com",
					Name:             "foobucket",
					Builders: []*swarmbucket.SwarmingSwarmbucketApiBuilderMessage{
						{
							Name:               "foobuilder",
							SwarmingDimensions: dim1,
						},
						{
							Name:               "foobuilder2",
							SwarmingDimensions: dim1,
						},
						{
							Name:               "foobuilder3",
							SwarmingDimensions: dim2,
						},
					},
				},
			},
		}

		md1 := model.NewPoolDescriptor("swarming.example.com", dim1)
		md2 := model.NewPoolDescriptor("swarming.example.com", dim2)
		fakeBotPool := []*model.BotPool{
			{
				Bots: []model.Bot{
					{Name: "bot1", Status: model.Offline},
					{Name: "bot2", Status: model.Busy},
				},
				Descriptor: md1,
				PoolKey:    md1.PoolKey(),
			},
			{
				Descriptor: md2,
				PoolKey:    md2.PoolKey(),
			},
		}
		c = caching.WithRequestCache(c)
		Convey(`Unit Tests`, func() {
			Convey(`Parsing Builders`, func() {
				pools := parseBuilders(c, RefTime, getBuilderMsg)
				So(len(pools), ShouldEqual, 3)
				So(pools[0].descriptor.Host(), ShouldEqual, "swarming.example.com")
				Convey(`Unique Dimensions`, func() {
					descs := uniqueDescriptors(pools)
					So(len(descs), ShouldEqual, 2)
				})

				Convey(`Save into datastore`, func() {
					So(savePools(c, pools, fakeBotPool), ShouldBeNil)
					Convey(`Load Builder from datastore`, func() {
						b1 := model.BuilderPool{
							BuilderID: datastore.MakeKey(c, "BuilderSummary", "buildbucket/foobucket/foobuilder"),
						}
						So(datastore.Get(c, &b1), ShouldBeNil)
						So(b1.PoolKey.StringID(), ShouldEqual, md1.PoolKey())
					})
					Convey(`Load Bots from datastore`, func() {
						p1 := model.BotPool{PoolKey: md1.PoolKey()}
						So(datastore.Get(c, &p1), ShouldBeNil)
						So(len(p1.Bots), ShouldEqual, 2)
						So(p1.Bots[0].Name, ShouldEqual, "bot1")
					})
				})
			})
		})
	})
}
