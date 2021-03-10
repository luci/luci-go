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

package clpurger

import (
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/text"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPurgeCLFormatMessage(t *testing.T) {
	t.Parallel()

	Convey("PurgeCL formatMessage works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const gHost = "x-review.googlesource.com"
		ci := gf.CI(
			43, gf.PS(2), gf.Project("re/po"), gf.Ref("refs/heads/main"),
			gf.CQ(+2, testclock.TestRecentTimeUTC, gf.U("user-1")),
			gf.Updated(testclock.TestRecentTimeUTC),
		)
		cl := &changelist.CL{
			Snapshot: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					},
				},
			},
		}
		task := &prjpb.PurgeCLTask{
			LuciProject: "luci-prj",
			PurgingCl:   nil, // not relevant to this test.
			Trigger:     trigger.Find(ci),
			Reason: &prjpb.PurgeCLTask_Reason{
				Reason: nil, // Set below.
			},
		}

		mustFormat := func() string {
			s, err := formatMessage(ctx, task, cl)
			So(err, ShouldBeNil)
			So(s, ShouldNotContainSubstring, "<no value>")
			return s
		}

		Convey("Lacks owner email", func() {
			task.Reason.Reason = &prjpb.PurgeCLTask_Reason_OwnerLacksEmail{
				OwnerLacksEmail: true,
			}
			So(mustFormat(), ShouldContainSubstring, "set preferred email at https://x-review.googlesource.com/settings/#EmailAddresses")
		})

		Convey("Watched by many config groups", func() {
			task.Reason.Reason = &prjpb.PurgeCLTask_Reason_WatchedByManyConfigGroups_{
				WatchedByManyConfigGroups: &prjpb.PurgeCLTask_Reason_WatchedByManyConfigGroups{
					ConfigGroups: []string{"first", "second"},
				},
			}
			s := mustFormat()
			So(s, ShouldContainSubstring, text.Doc(`
				it is watched by more than 1 config group:
				  * first
				  * second

				Please
			`))
			So(s, ShouldContainSubstring, `current CL target ref is "refs/heads/main"`)
		})

		Convey("Invalid deps", func() {
			// Save a CL snapshot for each dep.
			deps := make(map[int]*changelist.Dep, 3)
			for i := 101; i <= 102; i++ {
				depCL, err := changelist.MustGobID(gHost, int64(i)).GetOrInsert(ctx, func(cl *changelist.CL) {
					cl.Snapshot = &changelist.Snapshot{
						LuciProject:           "whatever",
						MinEquivalentPatchset: 1,
						Patchset:              2,
						ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: gf.CI(i),
							},
						},
					}
				})
				So(err, ShouldBeNil)
				deps[i] = &changelist.Dep{Clid: int64(depCL.ID)}
			}
			invalidDeps := &prjpb.PurgeCLTask_Reason_InvalidDeps{ /*set below*/ }
			task.Reason.Reason = &prjpb.PurgeCLTask_Reason_InvalidDeps_{InvalidDeps: invalidDeps}

			Convey("Unwatched", func() {
				invalidDeps.Unwatched = []*changelist.Dep{deps[101]}
				s := mustFormat()
				So(s, ShouldContainSubstring, text.Doc(`
				are not watched by the same LUCI project:
				  * https://x-review.googlesource.com/101

				Please check Cq-Depend
			`))
			})
			Convey("WrongConfigGroup", func() {
				invalidDeps.WrongConfigGroup = []*changelist.Dep{deps[101], deps[102]}
				s := mustFormat()
				So(s, ShouldContainSubstring, text.Doc(`
				its deps do not belong to the same config group:
				  * https://x-review.googlesource.com/101
				  * https://x-review.googlesource.com/102
			`))
			})
			Convey("IncompatMode", func() {
				invalidDeps.IncompatMode = []*changelist.Dep{deps[102], deps[101]}
				s := mustFormat()
				So(s, ShouldContainSubstring, text.Doc(`
				its mode "FullRun" does not match mode on its dependencies:
				  * https://x-review.googlesource.com/101
				  * https://x-review.googlesource.com/102
			`))
			})
		})
	})
}
