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

package model

import (
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock/testclock"
	google_pb "go.chromium.org/luci/common/proto/google"
	. "go.chromium.org/luci/common/testing/assertions"

	dm "go.chromium.org/luci/dm/api/service/v1"
)

func TestQuest(t *testing.T) {
	t.Parallel()

	Convey("Quest", t, func() {
		c := memory.Use(context.Background())
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

		Convey("QuestDescriptor", func() {
			Convey("good", func() {
				Convey("normal (normalized)", func() {
					qd := dm.NewQuestDesc("swarming", `{  "key"  :  ["value"]}`, "{  }", nil)
					So(qd.Normalize(), ShouldBeNil)
					So(NewQuest(c, qd), ShouldResemble, &Quest{
						"1258phYs8GW6qM5AQopQ_L3A5cZhO7iaYQZyFkNusVw",
						*qd,
						nil,
						testclock.TestTimeUTC,
					})
				})

				Convey("extra data", func() {
					qd := dm.NewQuestDesc("swarming", `{"key":["value"]} foof`, "{  }", nil)
					So(qd.Normalize(), ShouldErrLike, "extra junk")
				})

				Convey("data ordering", func() {
					qd := dm.NewQuestDesc("swarming", `{"key":["value"], "abc": true}`, "{  }", nil)
					So(qd.Normalize(), ShouldBeNil)
					So(NewQuest(c, qd), ShouldResemble, &Quest{
						"IMTBeXfkZgGntgNfWMuLa_YQA62o9dzxi0EoLCYXbsE",
						*qd,
						nil,
						testclock.TestTimeUTC,
					})
				})

			})

			Convey("bad", func() {
				Convey("payload too large", func() {
					payload := make([]byte, 512*1000)
					qd := dm.NewQuestDesc("swarming", string(payload), "{}", nil)
					So(qd.Normalize(), ShouldErrLike, "too large: 512002 > 262144")
				})

				Convey("json with null byte", func() {
					qd := dm.NewQuestDesc("swarming", "{\"key\": \"\x00\"}", "{}", nil)
					So(qd.Normalize(), ShouldErrLike, "invalid character")
				})

				Convey("not a dictionary", func() {
					qd := dm.NewQuestDesc("swarming", "[]", "{}", nil)
					So(qd.Normalize(), ShouldErrLike, "cannot unmarshal array")
				})
			})
		})

		Convey("ToProto", func() {
			qd := dm.NewQuestDesc("swarming", `{"key": ["value"]}`, "{}", nil)
			So(qd.Normalize(), ShouldBeNil)
			q := NewQuest(c, qd)
			p := q.ToProto()
			So(p, ShouldResemble, &dm.Quest{
				Id: dm.NewQuestID("1258phYs8GW6qM5AQopQ_L3A5cZhO7iaYQZyFkNusVw"),
				Data: &dm.Quest_Data{
					Created: google_pb.NewTimestamp(testclock.TestTimeUTC),
					Desc:    &q.Desc,
					BuiltBy: []*dm.Quest_TemplateSpec{},
				},
			})
			So(p.Data.Desc.Parameters, ShouldResemble, `{"key":["value"]}`)
		})

		Convey("QueryAttemptsForQuest", func() {
			qd := dm.NewQuestDesc("swarming", `{"key": ["value"]}`, "{}", nil)
			So(qd.Normalize(), ShouldBeNil)
			q := NewQuest(c, qd)
			So(ds.Put(c, q), ShouldBeNil)
			ds.GetTestable(c).CatchupIndexes()

			as := []*Attempt(nil)
			So(ds.GetAll(c, QueryAttemptsForQuest(c, q.ID), &as), ShouldBeNil)
			So(as, ShouldBeNil)

			a := &Attempt{ID: *dm.NewAttemptID(q.ID, 1)}
			So(ds.Put(c, a), ShouldBeNil)
			a.ID.Id = 2
			So(ds.Put(c, a), ShouldBeNil)
			a.ID.Quest = "eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7X" // one less
			a.ID.Id = 1
			So(ds.Put(c, a), ShouldBeNil)
			a.ID.Quest = "eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7Z" // one more
			So(ds.Put(c, a), ShouldBeNil)

			as = nil
			So(ds.GetAll(c, QueryAttemptsForQuest(c, q.ID), &as), ShouldBeNil)
			So(as, ShouldBeNil)

			ds.GetTestable(c).CatchupIndexes()
			as = nil
			So(ds.GetAll(c, QueryAttemptsForQuest(c, q.ID), &as), ShouldBeNil)
			So(as, ShouldResemble, []*Attempt{
				{ID: *dm.NewAttemptID("1258phYs8GW6qM5AQopQ_L3A5cZhO7iaYQZyFkNusVw", 2)},
				{ID: *dm.NewAttemptID("1258phYs8GW6qM5AQopQ_L3A5cZhO7iaYQZyFkNusVw", 1)},
			})

		})
	})
}
