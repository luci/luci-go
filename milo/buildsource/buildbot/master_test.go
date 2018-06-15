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

package buildbot

import (
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMaster(t *testing.T) {
	c := gaetesting.TestingContextWithAppID("dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	cExpired, _ := testclock.UseTime(c, testclock.TestTimeUTC.Add(-buildstore.MasterExpiry))
	cNotExpired, _ := testclock.UseTime(c, testclock.TestTimeUTC.Add(-buildstore.MasterExpiry+time.Second))
	datastore.GetTestable(c).Consistent(true)
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).CatchupIndexes()

	Convey(`Tests for master`, t, func() {

		So(buildstore.SaveMaster(cExpired, &buildbot.Master{
			Name:     "fake expired",
			Builders: map[string]*buildbot.Builder{"fake expired builder": {}},
		}, false, nil), ShouldBeNil)

		So(buildstore.SaveMaster(cNotExpired, &buildbot.Master{
			Name:     "fake not expired",
			Builders: map[string]*buildbot.Builder{"fake recent builder": {}},
		}, false, nil), ShouldBeNil)

		So(buildstore.SaveMaster(cNotExpired, &buildbot.Master{
			Name:     "fake internal",
			Builders: map[string]*buildbot.Builder{"fake": {}},
		}, true, nil), ShouldBeNil)

		Convey(`GetAllBuilders() should return all public builders`, func() {
			cs, err := CIService(c)
			So(err, ShouldBeNil)
			So(len(cs.BuilderGroups), ShouldEqual, 2)
			Convey(`Expired master should have no builders`, func() {
				So(cs.BuilderGroups[0].Name, ShouldEqual, "fake expired")
				So(len(cs.BuilderGroups[0].Builders), ShouldEqual, 0)
			})
			Convey(`Non-expired master should have builders`, func() {
				So(cs.BuilderGroups[1].Name, ShouldEqual, "fake not expired")
				So(len(cs.BuilderGroups[1].Builders), ShouldEqual, 1)
				So(cs.BuilderGroups[1].Builders[0].Label, ShouldEqual, "fake recent builder")
			})
		})
	})
}
