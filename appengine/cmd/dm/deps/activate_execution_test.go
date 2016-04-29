// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"testing"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/tumble"
	dm "github.com/luci/luci-go/common/api/dm/service/v1"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestActivateExecution(t *testing.T) {
	t.Parallel()

	Convey("Test ActivateExecution", t, func() {
		ttest := &tumble.Testing{}
		c := ttest.Context()
		ds := datastore.Get(c)
		_ = ds
		s := newDecoratedDeps()

		qid := ensureQuest(c, "foo", 1)
		ttest.Drain(c)
		realAuth := execute(c, dm.NewAttemptID(qid, 1))

		eid := dm.NewExecutionID(qid, 1, 1)

		req := &dm.ActivateExecutionReq{
			Auth:           &dm.Execution_Auth{Id: eid},
			ExecutionToken: []byte("newtok"),
		}

		Convey("bad", func() {
			Convey("wrong token", func() {
				_, err := s.ActivateExecution(c, req)
				So(err, ShouldBeRPCUnauthenticated, "failed to activate")
			})

			Convey("wrong token (already activated)", func() {
				req.Auth.Token = realAuth.Token
				_, err := s.ActivateExecution(c, req)
				So(err, ShouldBeNil)

				req.Auth.Token = []byte("bad sekret")
				req.ExecutionToken = []byte("random other tok")
				_, err = s.ActivateExecution(c, req)
				So(err, ShouldBeRPCUnauthenticated, "failed to activate")
			})

			Convey("concurrent activation", func() {
				req.Auth.Token = realAuth.Token
				_, err := s.ActivateExecution(c, req)
				So(err, ShouldBeNil)

				req.ExecutionToken = []byte("other newtok")
				_, err = s.ActivateExecution(c, req)
				So(err, ShouldBeRPCUnauthenticated, "failed to activate")
			})
		})

		Convey("good", func() {
			req.Auth.Token = realAuth.Token

			Convey("normal activation", func() {
				_, err := s.ActivateExecution(c, req)
				So(err, ShouldBeNil)
			})

			Convey("repeated activation", func() {
				_, err := s.ActivateExecution(c, req)
				So(err, ShouldBeNil)
				_, err = s.ActivateExecution(c, req)
				So(err, ShouldBeNil)
			})
		})
	})
}
