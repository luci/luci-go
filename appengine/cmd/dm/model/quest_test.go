// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestQuest(t *testing.T) {
	t.Parallel()

	Convey("Quest", t, func() {
		c := memory.Use(context.Background())
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

		Convey("QuestDescriptor", func() {
			Convey("good", func() {
				Convey("normal (normalized)", func() {
					qd := &QuestDescriptor{"swarming", []byte(`{  "key"  :  ["value"]}`)}
					q, err := qd.NewQuest(c)
					So(err, ShouldBeNil)
					So(q, ShouldResemble, &Quest{
						"h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbw",
						QuestDescriptor{"swarming", []byte(`{"key":["value"]}`)},
						testclock.TestTimeUTC,
					})
				})

				Convey("extra data", func() {
					qd := &QuestDescriptor{"swarming", []byte(`{"key":["value"]} foof`)}
					q, err := qd.NewQuest(c)
					So(err, ShouldBeNil)
					So(q, ShouldResemble, &Quest{
						"h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbw",
						QuestDescriptor{"swarming", []byte(`{"key":["value"]}`)},
						testclock.TestTimeUTC,
					})
				})
			})

			Convey("bad", func() {
				Convey("bad distributor string (bad format)", func() {
					qd := &QuestDescriptor{".swarming", []byte("{}")}
					_, err := qd.NewQuest(c)
					So(err, ShouldErrLike, "name is invalid")
				})

				Convey("bad distributor string (too long)", func() {
					qd := &QuestDescriptor{string(make([]byte, 100)), []byte("{}")}
					_, err := qd.NewQuest(c)
					So(err, ShouldErrLike, "too long: 100 > 64")
				})

				Convey("payload too large", func() {
					payload := make([]byte, 512*1000)
					qd := &QuestDescriptor{"swarming", payload}
					_, err := qd.NewQuest(c)
					So(err, ShouldErrLike, "too large: 512000 > 262144")
				})

				Convey("json with null byte", func() {
					qd := &QuestDescriptor{"swarming", []byte("{\"key\": \"\x00\"}")}
					_, err := qd.NewQuest(c)
					So(err, ShouldErrLike, "invalid character")
				})

				Convey("not a dictionary", func() {
					qd := &QuestDescriptor{"swarming", []byte("[]")}
					_, err := qd.NewQuest(c)
					So(err, ShouldErrLike, "cannot unmarshal array")
				})
			})
		})

		Convey("ToDisplay", func() {
			q, err := (&QuestDescriptor{"swarming", []byte(`{"key": ["value"]}`)}).NewQuest(c)
			So(err, ShouldBeNil)
			So(q.ToDisplay(), ShouldResemble, &display.Quest{
				ID:          "h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbw",
				Payload:     `{"key":["value"]}`,
				Distributor: "swarming",
				Created:     testclock.TestTimeUTC,
			})
		})

		Convey("GetAttempts", func() {
			q, err := (&QuestDescriptor{"swarming", []byte(`{"key": ["value"]}`)}).NewQuest(c)
			So(err, ShouldBeNil)
			ds := datastore.Get(c)
			So(ds.Put(q), ShouldBeNil)
			ds.Testable().CatchupIndexes()

			as, err := q.GetAttempts(c)
			So(err, ShouldBeNil)
			So(as, ShouldBeNil)

			a := &Attempt{}
			a.QuestID = q.ID
			a.AttemptNum = 1
			So(ds.Put(a), ShouldBeNil)
			a.AttemptNum = 2
			So(ds.Put(a), ShouldBeNil)
			a.QuestID = "h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbv" // one less
			a.AttemptNum = 1
			So(ds.Put(a), ShouldBeNil)
			a.QuestID = "h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbx" // one more
			So(ds.Put(a), ShouldBeNil)

			as, err = q.GetAttempts(c)
			So(err, ShouldBeNil)
			So(as, ShouldBeNil)

			ds.Testable().CatchupIndexes()
			as, err = q.GetAttempts(c)
			So(err, ShouldBeNil)
			So(as, ShouldResemble, []*Attempt{
				{AttemptID: *types.NewAttemptID("h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbw|fffffffd")},
				{AttemptID: *types.NewAttemptID("h0Bgaj7I6oxp08NArY3RWaaPS76mgTHwj_ePu35nMbw|fffffffe")},
			})

		})
	})
}
