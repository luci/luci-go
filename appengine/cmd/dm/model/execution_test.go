// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/logging/memlogger"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestExecutions(t *testing.T) {
	t.Parallel()

	Convey("Execution", t, func() {
		c := memory.Use(context.Background())
		c = memlogger.Use(c)
		ds := datastore.Get(c)

		a := NewAttempt("q", 1)
		ak := ds.KeyForObj(a)

		e1 := &Execution{ID: 1, Attempt: ak, ExecutionKey: []byte("hello")}

		So(ds.Put(e1), ShouldBeNil)

		Convey("Done", func() {
			So(e1.Done(), ShouldBeFalse)
			e1.ExecutionKey = []byte{}
			So(e1.Done(), ShouldBeTrue)
			e1.ExecutionKey = nil
			So(e1.Done(), ShouldBeTrue)
		})

		Convey("Revoke", func() {
			e2 := *e1
			So(e2.Revoke(c), ShouldBeNil)

			So(e1.ExecutionKey, ShouldResembleV, []byte("hello"))
			So(ds.Get(e1), ShouldBeNil)
			So(e1.ExecutionKey, ShouldBeNil)
		})

		Convey("Verify", func() {
			_, _, err := VerifyExecution(c, types.NewAttemptID("q|fffffffe"), []byte("hello"))
			So(err, ShouldErrLike, "couldn't get attempt")

			So(ds.Put(a), ShouldBeNil)
			_, _, err = VerifyExecution(c, types.NewAttemptID("q|fffffffe"), []byte("hello"))
			So(err, ShouldErrLike, "couldn't get execution")

			a.CurExecution = 1
			So(ds.Put(a), ShouldBeNil)
			_, _, err = VerifyExecution(c, types.NewAttemptID("q|fffffffe"), []byte("hello"))
			So(err, ShouldErrLike, "Attempt is not executing yet")

			a.State = types.Executing
			So(ds.Put(a), ShouldBeNil)
			_, _, err = VerifyExecution(c, types.NewAttemptID("q|fffffffe"), []byte("wat"))
			So(err, ShouldErrLike, "Incorrect ExecutionKey")

			atmpt, exe, err := VerifyExecution(c, types.NewAttemptID("q|fffffffe"), []byte("hello"))
			So(err, ShouldBeNil)

			So(atmpt, ShouldResembleV, a)
			So(exe, ShouldResembleV, e1)
		})

		Convey("Invalidate", func() {
			a.CurExecution = 1
			a.State = types.Executing
			So(ds.Put(a), ShouldBeNil)

			_, _, err := InvalidateExecution(c, types.NewAttemptID("q|fffffffe"), []byte("wat"))
			So(err, ShouldErrLike, "Incorrect ExecutionKey")

			_, _, err = InvalidateExecution(c, types.NewAttemptID("q|fffffffe"), []byte("hello"))
			So(err, ShouldBeNil)

			So(ds.Get(e1), ShouldBeNil)
			So(e1.ExecutionKey, ShouldBeNil)
		})
	})

}
