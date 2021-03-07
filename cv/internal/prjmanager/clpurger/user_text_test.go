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
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/text"

	"go.chromium.org/luci/cv/internal/changelist"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPurgeCLFormatMessage(t *testing.T) {
	t.Parallel()

	Convey("PurgeCL formatMessage works", t, func() {
		ctx := context.Background()
		ci := gf.CI(
			43, gf.PS(2), gf.Project("re/po"), gf.Ref("refs/heads/main"),
			gf.CQ(+2, testclock.TestRecentTimeUTC, gf.U("user-1")),
			gf.Updated(testclock.TestRecentTimeUTC),
		)
		cl := &changelist.CL{
			Snapshot: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: "x-review.googlesource.com",
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
	})
}
