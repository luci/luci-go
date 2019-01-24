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
	sv1 "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/buildsource/swarming"
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
					Name:             "luci.infra.foobucket",
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
		c = caching.WithRequestCache(c)
		Convey(`Unit Tests`, func() {
			Convey(`Strip empty dimensions`, func() {
				dims := []string{"foo", "foo:", "bar:baz"}
				newDims := stripEmptyDimensions(dims)
				So(newDims, ShouldResemble, []string{"bar:baz"})
			})
			Convey(`Parsing Builders`, func() {
				descriptors, err := processBuilders(c, getBuilderMsg)
				So(err, ShouldBeNil)
				So(len(descriptors), ShouldEqual, 2)
				Convey(`And builders should be there`, func() {
					b1 := model.BuilderPool{
						BuilderID: datastore.MakeKey(c, "BuilderSummary", "buildbucket/luci.infra.foobucket/foobuilder"),
					}
					So(datastore.Get(c, &b1), ShouldBeNil)
					So(b1.PoolKey.StringID(), ShouldEqual, md1.PoolID())
					b2 := model.BuilderPool{
						BuilderID: datastore.MakeKey(c, "BuilderSummary", "buildbucket/luci.infra.foobucket/foobuilder3"),
					}
					So(datastore.Get(c, &b2), ShouldBeNil)
					So(b2.PoolKey.StringID(), ShouldEqual, md2.PoolID())
				})
				// TODO(hinoka): Test that swarming pools load and save correctly.
			})

			Convey(`Parsing Bot Status`, func() {
				fakeHost := "fakehost.com"
				bot := &sv1.SwarmingRpcsBotInfo{
					LastSeenTs: RefTime.Format(swarming.SwarmingTimeLayout),
				}
				Convey(`Empty is Idle`, func() {
					s, err := parseBot(c, fakeHost, bot)
					So(err, ShouldBeNil)
					So(s.Status, ShouldEqual, model.Idle)
				})
				Convey(`With TaskID is Busy`, func() {
					bot.TaskId = "someID"
					s, err := parseBot(c, fakeHost, bot)
					So(err, ShouldBeNil)
					So(s.Status, ShouldEqual, model.Busy)
				})
				Convey(`Died is Offline`, func() {
					bot.IsDead = true
					s, err := parseBot(c, fakeHost, bot)
					So(err, ShouldBeNil)
					So(s.Status, ShouldEqual, model.Offline)
				})
				Convey(`Quarantined is Offline`, func() {
					bot.Quarantined = true
					s, err := parseBot(c, fakeHost, bot)
					So(err, ShouldBeNil)
					So(s.Status, ShouldEqual, model.Offline)
				})
			})
		})
	})
}
