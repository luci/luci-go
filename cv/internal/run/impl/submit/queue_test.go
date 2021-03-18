// Copyright 2021 The LUCI Authors.
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

package submit

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueue(t *testing.T) {
	t.Parallel()

	Convey("Queue", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "lProject"
		run1 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("deaddead"))
		run2 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("beefbeef"))
		run3 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("cafecafe"))
		submitOpts := &cfgpb.SubmitOptions{
			MaxBurst:   2,
			BurstDelay: durationpb.New(1 * time.Minute),
		}
		mustLoadQueue := func() *queue {
			q, err := loadQueue(ctx, lProject)
			So(err, ShouldBeNil)
			return q
		}

		Convey("Acquire", func() {
			Convey("When queue is empty", func() {
				waitlisted, err := TryAcquire(ctx, run1, submitOpts)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeFalse)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:      lProject,
					Current: run1,
					Opts:    submitOpts,
				})

				Convey("And acquire same Run again", func() {
					waitlisted, err := TryAcquire(ctx, run1, submitOpts)
					So(err, ShouldBeNil)
					So(waitlisted, ShouldBeFalse)
					So(mustLoadQueue(), shouldResembleQueue, &queue{
						ID:      lProject,
						Current: run1,
						Opts:    submitOpts,
					})
				})
			})

			Convey("Waitlisted", func() {
				So(datastore.Put(ctx, &queue{
					ID:      lProject,
					Current: run1,
					Opts:    submitOpts,
				}), ShouldBeNil)
				waitlisted, err := TryAcquire(ctx, run2, submitOpts)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeTrue)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2},
					Opts:     submitOpts,
				})
				waitlisted, err = TryAcquire(ctx, run3, submitOpts)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeTrue)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2, run3},
					Opts:     submitOpts,
				})

				Convey("And acquire same Run in waitlist again", func() {
					for _, r := range []common.RunID{run2, run3} {
						waitlisted, err := TryAcquire(ctx, r, submitOpts)
						So(err, ShouldBeNil)
						So(waitlisted, ShouldBeTrue)
						So(mustLoadQueue(), shouldResembleQueue, &queue{
							ID:       lProject,
							Current:  run1,
							Waitlist: common.RunIDs{run2, run3},
							Opts:     submitOpts,
						})
					}
				})
			})

			Convey("Promote first in the Waitlist if Current is empty", func() {
				So(datastore.Put(ctx, &queue{
					ID:       lProject,
					Waitlist: common.RunIDs{run2, run3},
					Opts:     submitOpts,
				}), ShouldBeNil)
				waitlisted, err := TryAcquire(ctx, run2, submitOpts)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeFalse)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:       lProject,
					Current:  run2,
					Waitlist: common.RunIDs{run3},
					Opts:     submitOpts,
				})
			})
		})

		Convey("Release", func() {
			So(datastore.Put(ctx, &queue{
				ID:       lProject,
				Current:  run1,
				Waitlist: common.RunIDs{run2, run3},
				Opts:     submitOpts,
			}), ShouldBeNil)
			Convey("Current slot", func() {
				err := Release(ctx, run1)
				So(err, ShouldBeNil)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:       lProject,
					Waitlist: common.RunIDs{run2, run3},
					Opts:     submitOpts,
				})
				runtest.AssertReceivedReadyForSubmission(ctx, run2, time.Time{})
				for _, r := range []common.RunID{run1, run3} {
					runtest.AssertEventboxEmpty(ctx, r)
				}
			})

			Convey("Run in waitlist", func() {
				err := Release(ctx, run2)
				So(err, ShouldBeNil)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run3},
					Opts:     submitOpts,
				})
				for _, r := range []common.RunID{run1, run2, run3} {
					runtest.AssertEventboxEmpty(ctx, r)
				}
			})

			Convey("Non-existing run", func() {
				nonExisting := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("badbadbad"))
				err := Release(ctx, nonExisting)
				So(err, ShouldBeNil)
				So(mustLoadQueue(), shouldResembleQueue, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2, run3},
					Opts:     submitOpts,
				})
				for _, r := range []common.RunID{run1, run2, run3} {
					runtest.AssertEventboxEmpty(ctx, r)
				}
			})
		})
	})
}

func shouldResembleQueue(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("expected 1 value, got %d", len(expected))
	}
	exp := expected[0] // this may be nil
	a, ok := actual.(*queue)
	if !ok {
		return fmt.Sprintf("Wrong actual type %T, must be %T", actual, a)
	}
	if err := ShouldHaveSameTypeAs(actual, exp); err != "" {
		return err
	}
	b := exp.(*queue)
	switch {
	case a == b:
		return ""
	case a == nil:
		return "actual is nil, but non-nil was expected"
	case b == nil:
		return "actual is not-nil, but nil was expected"
	}

	buf := strings.Builder{}
	for _, err := range []string{
		ShouldEqual(a.Current, b.Current),
		ShouldResemble(a.Waitlist, b.Waitlist),
		ShouldResembleProto(a.Opts, b.Opts),
		ShouldResemble(a.History, b.History),
	} {
		if err != "" {
			buf.WriteRune(' ')
			buf.WriteString(err)
		}
	}
	return strings.TrimSpace(buf.String())
}
