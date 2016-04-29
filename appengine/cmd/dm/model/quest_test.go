// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	google_pb "github.com/luci/luci-go/common/proto/google"
	. "github.com/luci/luci-go/common/testing/assertions"

	"github.com/luci/luci-go/common/api/dm/service/v1"
)

func TestQuest(t *testing.T) {
	t.Parallel()

	desc := func(cfg, jsonData string) *dm.Quest_Desc {
		return &dm.Quest_Desc{
			DistributorConfigName: cfg,
			JsonPayload:           jsonData,
		}
	}

	Convey("Quest", t, func() {
		c := memory.Use(context.Background())
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

		Convey("QuestDescriptor", func() {
			Convey("good", func() {
				Convey("normal (normalized)", func() {
					qd := desc("swarming", `{  "key"  :  ["value"]}`)
					q, err := NewQuest(c, qd)
					So(err, ShouldBeNil)
					So(q, ShouldResemble, &Quest{
						"eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7Y",
						*desc("swarming", `{"key":["value"]}`),
						nil,
						testclock.TestTimeUTC,
					})
				})

				Convey("extra data", func() {
					qd := desc("swarming", `{"key":["value"]} foof`)
					_, err := NewQuest(c, qd)
					So(err, ShouldErrLike, "extra junk")
				})

				Convey("data ordering", func() {
					qd := desc("swarming", `{"key":["value"], "abc": true}`)
					q, err := NewQuest(c, qd)
					So(err, ShouldBeNil)
					So(q, ShouldResemble, &Quest{
						"KO5hRgXIFouei7Xg5Oai0K5hJeuuiO70jRaDyqQO0MM",
						*desc("swarming", `{"abc":true,"key":["value"]}`),
						nil,
						testclock.TestTimeUTC,
					})
				})

			})

			Convey("bad", func() {
				Convey("payload too large", func() {
					payload := make([]byte, 512*1000)
					qd := desc("swarming", string(payload))
					_, err := NewQuest(c, qd)
					So(err, ShouldErrLike, "too large: 512000 > 262144")
				})

				Convey("json with null byte", func() {
					qd := desc("swarming", "{\"key\": \"\x00\"}")
					_, err := NewQuest(c, qd)
					So(err, ShouldErrLike, "invalid character")
				})

				Convey("not a dictionary", func() {
					qd := desc("swarming", "[]")
					_, err := NewQuest(c, qd)
					So(err, ShouldErrLike, "cannot unmarshal array")
				})
			})
		})

		Convey("ToProto", func() {
			q, err := NewQuest(c, desc("swarming", `{"key": ["value"]}`))
			So(err, ShouldBeNil)
			p := q.ToProto()
			So(p, ShouldResemble, &dm.Quest{
				Id: dm.NewQuestID("eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7Y"),
				Data: &dm.Quest_Data{
					Created: google_pb.NewTimestamp(testclock.TestTimeUTC),
					Desc:    &q.Desc,
					BuiltBy: []*dm.Quest_TemplateSpec{},
				},
			})
			So(p.Data.Desc.JsonPayload, ShouldResemble, `{"key":["value"]}`)
		})

		Convey("QueryAttemptsForQuest", func() {
			q, err := NewQuest(c, desc("swarming", `{"key": ["value"]}`))
			So(err, ShouldBeNil)
			ds := datastore.Get(c)
			So(ds.Put(q), ShouldBeNil)
			ds.Testable().CatchupIndexes()

			as := []*Attempt(nil)
			So(ds.GetAll(QueryAttemptsForQuest(c, q.ID), &as), ShouldBeNil)
			So(as, ShouldBeNil)

			a := &Attempt{ID: *dm.NewAttemptID(q.ID, 1)}
			So(ds.Put(a), ShouldBeNil)
			a.ID.Id = 2
			So(ds.Put(a), ShouldBeNil)
			a.ID.Quest = "eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7X" // one less
			a.ID.Id = 1
			So(ds.Put(a), ShouldBeNil)
			a.ID.Quest = "eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7Z" // one more
			So(ds.Put(a), ShouldBeNil)

			as = nil
			So(ds.GetAll(QueryAttemptsForQuest(c, q.ID), &as), ShouldBeNil)
			So(as, ShouldBeNil)

			ds.Testable().CatchupIndexes()
			as = nil
			So(ds.GetAll(QueryAttemptsForQuest(c, q.ID), &as), ShouldBeNil)
			So(as, ShouldResemble, []*Attempt{
				{ID: *dm.NewAttemptID("eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7Y", 2)},
				{ID: *dm.NewAttemptID("eMpqiyje5ItTX8IistN7IlAMVxyCsJcez4DAHKvhm7Y", 1)},
			})

		})
	})
}
