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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMaster(t *testing.T) {
	c := gaetesting.TestingContextWithAppID("dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	datastore.GetTestable(c).Consistent(true)
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).CatchupIndexes()

	Convey(`Tests for master`, t, func() {
		So(putDSMasterJSON(c, &buildbotMaster{
			Name:     "fake",
			Builders: map[string]*buildbotBuilder{"fake": {}},
		}, false), ShouldBeNil)
		So(putDSMasterJSON(c, &buildbotMaster{
			Name:     "fake internal",
			Builders: map[string]*buildbotBuilder{"fake": {}},
		}, true), ShouldBeNil)

		Convey(`GetAllBuilders()`, func() {
			cs, err := GetAllBuilders(c)
			So(err, ShouldBeNil)
			So(len(cs.BuilderGroups), ShouldEqual, 1)
		})
	})
}
