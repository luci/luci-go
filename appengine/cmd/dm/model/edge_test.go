// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBackdepEdge(t *testing.T) {
	t.Parallel()

	Convey("BackDep.Edge", t, func() {
		bd := &BackDep{
			*types.NewAttemptID("depender|fffffffa"),
			datastore.MakeKey("aid", "ns", "BackDepGroup", "quest|fffffffe"),
			true,
		}
		So(bd.Edge(), ShouldResemble, &FwdEdge{
			&types.AttemptID{QuestID: "depender", AttemptNum: 5},
			&types.AttemptID{QuestID: "quest", AttemptNum: 1},
		})
	})
}

func TestFwdDepEdge(t *testing.T) {
	t.Parallel()

	Convey("FwdDep.Edge", t, func() {
		bd := &FwdDep{
			Dependee: *types.NewAttemptID("quest|fffffffe"),
			Depender: datastore.MakeKey("aid", "ns", "Attempt", "depender|fffffffa"),
		}
		So(bd.Edge(), ShouldResemble, &FwdEdge{
			&types.AttemptID{QuestID: "depender", AttemptNum: 5},
			&types.AttemptID{QuestID: "quest", AttemptNum: 1},
		})
	})
}

func TestFwdEdge(t *testing.T) {
	t.Parallel()

	Convey("FwdEdge", t, func() {
		c := memory.Use(context.Background())
		e := &FwdEdge{
			types.NewAttemptID("from|fffffffe"),
			types.NewAttemptID("to|fffffffe"),
		}

		Convey("Fwd", func() {
			atmpt, fwddep := e.Fwd(c)
			So(atmpt.QuestID, ShouldEqual, "from")
			So(atmpt.AttemptNum, ShouldEqual, 1)
			So(fwddep.Dependee.QuestID, ShouldEqual, "to")
			So(fwddep.Dependee.AttemptNum, ShouldEqual, 1)
			So(fwddep.Depender.String(), ShouldEqual, `dev~app::/Attempt,"from|fffffffe"`)
		})

		Convey("Back", func() {
			bdg, bdep := e.Back(c)
			So(bdg.Dependee.QuestID, ShouldEqual, "to")
			So(bdg.Dependee.AttemptNum, ShouldEqual, 1)
			So(bdep.Depender.QuestID, ShouldEqual, "from")
			So(bdep.Depender.AttemptNum, ShouldEqual, 1)
			So(bdep.DependeeGroup.String(), ShouldEqual, `dev~app::/BackDepGroup,"to|fffffffe"`)
		})

	})
}

func TestAttemptFanout(t *testing.T) {
	t.Parallel()

	Convey("AttemptFanout.Fwds", t, func() {
		c := memory.Use(context.Background())
		fanout := &AttemptFanout{
			types.NewAttemptID("quest|fffffffe"),
			types.AttemptIDSlice{
				types.NewAttemptID("a|fffffffe"),
				types.NewAttemptID("b|fffffffe"),
				types.NewAttemptID("b|fffffffd"),
				types.NewAttemptID("c|fffffffe"),
			},
		}

		root := datastore.Get(c).MakeKey("Attempt", "quest|fffffffe")

		So(fanout.Fwds(c), ShouldResemble, []*FwdDep{
			{Depender: root, Dependee: *types.NewAttemptID("a|fffffffe")},
			{Depender: root, Dependee: *types.NewAttemptID("b|fffffffe")},
			{Depender: root, Dependee: *types.NewAttemptID("b|fffffffd")},
			{Depender: root, Dependee: *types.NewAttemptID("c|fffffffe")},
		})
	})

}
