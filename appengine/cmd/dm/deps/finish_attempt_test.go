// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	google_pb "github.com/luci/luci-go/common/proto/google"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestFinishAttempt(t *testing.T) {
	t.Parallel()

	Convey("FinishAttempt", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)
		s := &deps{}

		So(ds.Put(&model.Quest{ID: "quest"}), ShouldBeNil)
		a := &model.Attempt{
			ID:           *dm.NewAttemptID("quest", 1),
			State:        dm.Attempt_Executing,
			CurExecution: 1,
		}
		So(ds.Put(a), ShouldBeNil)
		So(ds.Put(&model.Execution{
			ID: 1, Attempt: ds.KeyForObj(a), Token: []byte("exKey"),
			State: dm.Execution_Running}), ShouldBeNil)

		req := &dm.FinishAttemptReq{
			Auth: &dm.Execution_Auth{
				Id:    dm.NewExecutionID(a.ID.Quest, a.ID.Id, 1),
				Token: []byte("exKey"),
			},
			JsonResult: `{"something": "valid"}`,
			Expiration: google_pb.NewTimestamp(testclock.TestTimeUTC),
		}

		Convey("bad", func() {
			Convey("bad Token", func() {
				req.Auth.Token = []byte("fake")
				_, err := s.FinishAttempt(c, req)
				So(err, ShouldBeRPCUnauthenticated, "execution Auth")
			})

			Convey("not real json", func() {
				req.JsonResult = `i am not valid json`
				_, err := s.FinishAttempt(c, req)
				So(err, ShouldErrLike, "invalid character 'i'")
			})
		})

		Convey("good", func() {
			_, err := s.FinishAttempt(c, req)
			So(err, ShouldBeNil)

			So(ds.Get(a), ShouldBeNil)
			So(a.State, ShouldEqual, dm.Attempt_Finished)
		})

	})
}
