// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestFinishAttempt(t *testing.T) {
	t.Parallel()

	Convey("FinishAttempt", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)
		s := getService()

		So(ds.Put(&model.Quest{ID: "quest"}), ShouldBeNil)
		a := &model.Attempt{
			AttemptID:    *types.NewAttemptID("quest|fffffffe"),
			State:        types.Executing,
			CurExecution: 1,
		}
		So(ds.Put(a), ShouldBeNil)
		So(ds.Put(&model.Execution{ID: 1, Attempt: ds.KeyForObj(a), ExecutionKey: []byte("exKey")}), ShouldBeNil)

		Convey("bad", func() {
			Convey("bad ExecutionKey", func() {
				err := s.FinishAttempt(c, &FinishAttemptReq{
					a.AttemptID,
					[]byte("fake"),

					[]byte(`{"something": "valid"}`),
					testclock.TestTimeUTC,
				})
				So(err, ShouldErrLike, "Incorrect ExecutionKey")
			})

			Convey("not real json", func() {
				err := s.FinishAttempt(c, &FinishAttemptReq{
					a.AttemptID,
					[]byte("exKey"),

					[]byte(`i am not valid json`),
					testclock.TestTimeUTC,
				})
				So(err, ShouldErrLike, "invalid character 'i'")
			})
		})

		Convey("good", func() {
			err := s.FinishAttempt(c, &FinishAttemptReq{
				a.AttemptID,
				[]byte("exKey"),

				[]byte(`{"data": "yes"}`),
				testclock.TestTimeUTC,
			})
			So(err, ShouldBeNil)

			So(ds.Get(a), ShouldBeNil)
			So(a.State, ShouldEqual, types.Finished)
		})

	})
}
