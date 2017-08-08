// Copyright 2016 The LUCI Authors.
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

package tumble

import (
	"testing"

	ds "go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
)

func TestAddToJournal(t *testing.T) {
	t.Parallel()

	Convey("Test AddToJournal", t, func() {
		ttest := &Testing{}
		c := ttest.Context()
		ml := logging.Get(c).(*memlogger.MemLogger)

		Convey("with no mutations", func() {
			So(AddToJournal(c), ShouldBeNil)
			So(ml.HasFunc(func(e *memlogger.LogEntry) bool {
				return e.Msg == "tumble.AddToJournal" && e.Data["count"].(int) == 0
			}), ShouldBeTrue)
		})

		Convey("with some mutations", func() {
			from := &User{Name: "fromUser"}
			So(ds.Put(c, from), ShouldBeNil)
			So(AddToJournal(c, &WriteMessage{
				from.MakeOutgoingMessage(c, "hey there", "a", "b")}), ShouldBeNil)

			qry := ds.NewQuery("tumble.Mutation").Ancestor(ds.MakeKey(c, "tumble.temp", "8c60aac4ffd6e66142bef4e745d9d91546c115d18cc8283723699d964422a47a"))
			pmaps := []ds.PropertyMap{}
			So(ds.GetAll(c, qry, &pmaps), ShouldBeNil)
			So(len(pmaps), ShouldEqual, 1)
			So(pmaps[0].Slice("$key")[0].Value().(*ds.Key).String(), ShouldEqual,
				`dev~app::/tumble.temp,"8c60aac4ffd6e66142bef4e745d9d91546c115d18cc8283723699d964422a47a"/tumble.Mutation,"0000000000000000_00000000_00000000"`)
		})
	})
}
