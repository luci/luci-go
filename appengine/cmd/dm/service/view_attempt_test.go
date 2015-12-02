// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"testing"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestViewAttempt(t *testing.T) {
	t.Parallel()

	Convey("ViewAttempt", t, func() {
		ttest := tumble.Testing{}
		c := ttest.Context()

		ds := datastore.Get(c)
		s := getService()
		log := logging.Get(c).(*memlogger.MemLogger)

		_ = log // keep this around for convenience

		Convey("bad", func() {
			Convey("no attempt", func() {
				ret, err := s.ViewAttempt(c, &ViewAttemptReq{AttemptID: *mkAid("quest", 1)})
				So(err, ShouldErrLike, datastore.ErrNoSuchEntity)
				So(ret, ShouldBeNil)
			})
		})

		Convey("good", func() {
			ds.Testable().Consistent(true)

			So(ds.Put(
				&model.Distributor{Name: "foof", URL: "https://foof.example.com"}),
				ShouldBeNil)

			w := mkQuest(c, "w")

			So(s.EnsureAttempt(c, &EnsureAttemptReq{*mkAid(w, 1)}), ShouldBeNil)

			req := &ViewAttemptReq{
				AttemptID: *mkAid(w, 1),
				Options: &AttemptOpts{
					BackDeps: true,
					FwdDeps:  true,
					Result:   true,
					MaxDepth: 2,
				},
			}

			view := func() *display.Data {
				ret, err := s.ViewAttempt(c, req)
				So(err, ShouldBeNil)
				return ret
			}

			Convey("nil options (no dependencies)", func() {
				req.Options = nil
				So(view(), ShouldResembleV, &display.Data{
					Attempts: display.AttemptSlice{
						{ID: *mkAid(w, 1), State: types.NeedsExecution},
					},
				})
			})

			Convey("no dependencies", func() {
				So(view(), ShouldResembleV, &display.Data{
					Attempts: display.AttemptSlice{
						{ID: *mkAid(w, 1), State: types.NeedsExecution},
					},
				})
			})

			Convey("finished", func() {
				execute(c, mkAid(w, 1))

				So(s.FinishAttempt(c, &FinishAttemptReq{
					*mkAid(w, 1),
					[]byte("sekret"),

					[]byte(`{"data": ["very", "yes"]}`),
					clock.Now(c).Add(time.Hour * 24 * 4),
				}), ShouldBeNil)

				So(view(), ShouldResembleV, &display.Data{
					Attempts: display.AttemptSlice{
						{
							ID:            *mkAid(w, 1),
							State:         types.Finished,
							NumExecutions: 1,
							Expiration:    clock.Now(c).Add(time.Hour * 24 * 4).Round(time.Millisecond),
						},
					},
					AttemptResults: display.AttemptResultSlice{
						{
							ID:   *mkAid(w, 1),
							Data: `{"data":["very","yes"]}`,
						},
					},
				})
			})

			Convey("deps (no dest attempts)", func() {
				execute(c, mkAid(w, 1))
				x := mkQuest(c, "x")
				depOn(c, mkAid(w, 1), mkAid(x, 1), mkAid(x, 2))
				// don't run tumble, so that x|1 and x|2 don't get created.
				So(view(), ShouldResembleV, &display.Data{
					Attempts: display.AttemptSlice{
						{ID: *mkAid(w, 1), State: types.AddingDeps,
							NumExecutions: 1, NumWaitingDeps: 2},
					},
					FwdDeps: display.DepsFromAttemptSlice{
						mkDisplayDeps(mkAid(w, 1), mkAid(x, 1), mkAid(x, 2)),
					},
				})

				Convey("deps (with dest attempts)", func() {
					ttest.Drain(c)

					So(view(), ShouldResembleV, &display.Data{
						Attempts: display.AttemptSlice{
							{ID: *mkAid(w, 1), State: types.Blocked,
								NumExecutions: 1, NumWaitingDeps: 2},
							{ID: *mkAid(x, 1), State: types.NeedsExecution},
							{ID: *mkAid(x, 2), State: types.NeedsExecution},
						},
						FwdDeps: display.DepsFromAttemptSlice{
							mkDisplayDeps(mkAid(w, 1), mkAid(x, 1), mkAid(x, 2)),
						},
						BackDeps: display.DepsFromAttemptSlice{
							mkDisplayDeps(mkAid(x, 1), mkAid(w, 1)),
							mkDisplayDeps(mkAid(x, 2), mkAid(w, 1)),
						},
					})
				})

				Convey("diamond", func() {
					ttest.Drain(c)
					z := mkQuest(c, "z")
					execute(c, mkAid(x, 1))
					execute(c, mkAid(x, 2))
					depOn(c, mkAid(x, 1), mkAid(z, 1))
					depOn(c, mkAid(x, 2), mkAid(z, 1))
					ttest.Drain(c)

					So(view(), ShouldResembleV, &display.Data{
						Attempts: display.AttemptSlice{
							{ID: *mkAid(w, 1), State: types.Blocked, NumExecutions: 1, NumWaitingDeps: 2},
							{ID: *mkAid(z, 1), State: types.NeedsExecution},
							{ID: *mkAid(x, 1), State: types.Blocked, NumExecutions: 1, NumWaitingDeps: 1},
							{ID: *mkAid(x, 2), State: types.Blocked, NumExecutions: 1, NumWaitingDeps: 1},
						},
						FwdDeps: display.DepsFromAttemptSlice{
							mkDisplayDeps(mkAid(w, 1), mkAid(x, 1), mkAid(x, 2)),
							mkDisplayDeps(mkAid(x, 1), mkAid(z, 1)),
							mkDisplayDeps(mkAid(x, 2), mkAid(z, 1)),
						},
						BackDeps: display.DepsFromAttemptSlice{
							mkDisplayDeps(mkAid(z, 1), mkAid(x, 1), mkAid(x, 2)),
							mkDisplayDeps(mkAid(x, 1), mkAid(w, 1)),
							mkDisplayDeps(mkAid(x, 2), mkAid(w, 1)),
						},
					})
				})

				Convey("diamond (dfs)", func() {
					ttest.Drain(c)
					z := mkQuest(c, "z")
					execute(c, mkAid(x, 1))
					execute(c, mkAid(x, 2))
					depOn(c, mkAid(x, 1), mkAid(z, 1))
					depOn(c, mkAid(x, 2), mkAid(z, 1))
					ttest.Drain(c)

					req.Options.DFS = true
					So(view(), ShouldResembleV, &display.Data{
						Attempts: display.AttemptSlice{
							{ID: *mkAid(w, 1), State: types.Blocked, NumExecutions: 1, NumWaitingDeps: 2},
							{ID: *mkAid(z, 1), State: types.NeedsExecution},
							{ID: *mkAid(x, 1), State: types.Blocked, NumExecutions: 1, NumWaitingDeps: 1},
							{ID: *mkAid(x, 2), State: types.Blocked, NumExecutions: 1, NumWaitingDeps: 1},
						},
						FwdDeps: display.DepsFromAttemptSlice{
							mkDisplayDeps(mkAid(w, 1), mkAid(x, 1), mkAid(x, 2)),
							mkDisplayDeps(mkAid(x, 1), mkAid(z, 1)),
							mkDisplayDeps(mkAid(x, 2), mkAid(z, 1)),
						},
						BackDeps: display.DepsFromAttemptSlice{
							mkDisplayDeps(mkAid(z, 1), mkAid(x, 1), mkAid(x, 2)),
							mkDisplayDeps(mkAid(x, 1), mkAid(w, 1)),
							mkDisplayDeps(mkAid(x, 2), mkAid(w, 1)),
						},
					})
				})

				Convey("early stop (simulated)", func() {
					req.Options.testSimulateTimeout = true
					So(view(), ShouldResembleV, &display.Data{Timeout: true})
				})

			})
		})

	})
}
