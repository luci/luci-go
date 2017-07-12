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
	"context"
	"strconv"
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/milo/api/resp"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConsole(t *testing.T) {
	c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	fakeTime := float64(123)
	fakeResult := int(0)

	// Seed a builder with 8 builds.
	for i := 1; i < 10; i++ {
		datastore.Put(c, &buildbotBuild{
			Master:      "fake",
			Buildername: "fake",
			Number:      i,
			Internal:    false,
			Times:       []*float64{&fakeTime, &fakeTime},
			Sourcestamp: &buildbotSourceStamp{
				Revision: strconv.Itoa(i),
			},
			Results:  &fakeResult,
			Finished: true,
		})
	}
	putDSMasterJSON(c, &buildbotMaster{
		Name:     "fake",
		Builders: map[string]*buildbotBuilder{"fake": {}},
	}, false)
	datastore.GetTestable(c).Consistent(true)
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).CatchupIndexes()

	Convey(`Console tests for buildbot`, t, func() {
		Convey(`Empty request`, func() {
			cb, err := GetConsoleBuilds(c, []resp.BuilderRef{}, []string{})
			So(err, ShouldBeNil)
			So(cb, ShouldResemble, [][]*resp.ConsoleBuild{})
		})
		Convey(`Basic`, func() {
			ref := []resp.BuilderRef{
				{
					Name:      "buildbot/fake/fake",
					Category:  []string{"np"},
					ShortName: "np",
				},
			}
			cb, err := GetConsoleBuilds(c, ref, []string{"2", "3", "5"})
			So(err, ShouldBeNil)
			So(len(cb), ShouldEqual, 3)
			So(len(cb[0]), ShouldEqual, 1)
		})
	})
}
