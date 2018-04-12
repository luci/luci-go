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

		md1 := model.NewDimensions("swarming.example.com", dim1)
		md2 := model.NewDimensions("swarming.example.com", dim2)
		fakeInfo := map[string]model.MachinePool{}
		fakeInfo[md1.SHA1] = []model.Machine{
			{Name: "bot1", Status: model.Offline},
			{Name: "bot2", Status: model.Busy},
		}
		fakeInfo[md2.SHA1] = []model.Machine{}
		c = caching.WithRequestCache(c)
		Convey(`Unit Tests`, func() {
			Convey(`Parsing Builders`, func() {
				pools := parseBuilders(c, getBuilderMsg)
				So(len(pools), ShouldEqual, 3)
				So(pools[0].Dimensions.Host, ShouldEqual, "swarming.example.com")
				Convey(`Unique Dimensions`, func() {
					dims := uniqueDimensions(pools)
					So(len(dims), ShouldEqual, 2)
				})

				Convey(`Save into datastore`, func() {
					So(savePoolInfo(c, pools, fakeInfo), ShouldBeNil)
					b1 := model.BuilderPool{
						BuilderKey: datastore.MakeKey(c, "BuilderSummary", "buildbucket/foobucket/foobuilder"),
					}
					So(datastore.Get(c, &b1), ShouldBeNil)
					So(b1.Dimensions.SHA1, ShouldEqual, md1.SHA1)
					So(len(b1.Machines), ShouldEqual, 2)
					So(b1.Machines[0].Name, ShouldEqual, "bot1")
				})
			})
		})
	})
}
