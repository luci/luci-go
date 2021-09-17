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

package usertext

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFormatCLError(t *testing.T) {
	t.Parallel()

	Convey("CLError formatting works", t, func() {
		ctx := memory.Use(context.Background())
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

		Convey("Single CLError", func() {
			// reason.Kind and mode are set in tests below as needed.
			reason := &changelist.CLError{Kind: nil}
			var mode run.Mode
			mustFormat := func() string {
				s, err := SFormatCLError(ctx, reason, cl, mode)
				So(err, ShouldBeNil)
				So(s, ShouldNotContainSubstring, "<no value>")
				return s
			}

			Convey("Lacks owner email", func() {
				reason.Kind = &changelist.CLError_OwnerLacksEmail{
					OwnerLacksEmail: true,
				}
				So(mustFormat(), ShouldContainSubstring, "set preferred email at https://x-review.googlesource.com/settings/#EmailAddresses")
			})
			Convey("Not yet supported mode", func() {
				reason.Kind = &changelist.CLError_UnsupportedMode{
					UnsupportedMode: "CUSTOM_RUN",
				}
				So(mustFormat(), ShouldContainSubstring, `its mode "CUSTOM_RUN" is not supported`)
			})
			Convey("Depends on itself", func() {
				reason.Kind = &changelist.CLError_SelfCqDepend{SelfCqDepend: true}
				So(mustFormat(), ShouldContainSubstring, `because it depends on itself`)
			})
			Convey("CorruptGerrit", func() {
				reason.Kind = &changelist.CLError_CorruptGerritMetadata{CorruptGerritMetadata: "foo is not bar"}
				So(mustFormat(), ShouldContainSubstring, `foo is not bar`)
			})
			Convey("Watched by many config groups", func() {
				reason.Kind = &changelist.CLError_WatchedByManyConfigGroups_{
					WatchedByManyConfigGroups: &changelist.CLError_WatchedByManyConfigGroups{
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
			Convey("Watched by many LUCI projects", func() {
				reason.Kind = &changelist.CLError_WatchedByManyProjects_{
					WatchedByManyProjects: &changelist.CLError_WatchedByManyProjects{
						Projects: []string{"first", "second"},
					},
				}
				s := mustFormat()
				So(s, ShouldContainSubstring, text.Doc(`
				it is watched by more than 1 LUCI project:
				  * first
				  * second

				Please
			`))
			})
			Convey("Invalid deps", func() {
				// Save a CL snapshot for each dep.
				deps := make(map[int]*changelist.Dep, 3)
				for i := 101; i <= 102; i++ {
					depCL := changelist.MustGobID(gHost, int64(i)).MustCreateIfNotExists(ctx)
					depCL.Snapshot = &changelist.Snapshot{
						LuciProject:           "whatever",
						MinEquivalentPatchset: 1,
						Patchset:              2,
						ExternalUpdateTime:    timestamppb.New(testclock.TestRecentTimeUTC),
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: gf.CI(i),
							},
						},
					}
					So(datastore.Put(ctx, depCL), ShouldBeNil)
					deps[i] = &changelist.Dep{Clid: int64(depCL.ID)}
				}
				invalidDeps := &changelist.CLError_InvalidDeps{ /*set below*/ }
				reason.Kind = &changelist.CLError_InvalidDeps_{InvalidDeps: invalidDeps}

				Convey("Unwatched", func() {
					invalidDeps.Unwatched = []*changelist.Dep{deps[101]}
					s := mustFormat()
					So(s, ShouldContainSubstring, text.Doc(`
				are not watched by the same LUCI project:
				  * https://x-review.googlesource.com/c/101

				Please check Cq-Depend
			`))
				})
				Convey("WrongConfigGroup", func() {
					invalidDeps.WrongConfigGroup = []*changelist.Dep{deps[101], deps[102]}
					s := mustFormat()
					So(s, ShouldContainSubstring, text.Doc(`
				its deps do not belong to the same config group:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`))
				})
				Convey("Singular Full Run with open dependencies", func() {
					mode = run.FullRun
					invalidDeps.SingleFullDeps = []*changelist.Dep{deps[102], deps[101]}
					s := mustFormat()
					So(s, ShouldContainSubstring, text.Doc(`
				in "FULL_RUN" mode because it has not yet submitted dependencies:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`))
				})
				Convey("Combinable not triggered deps", func() {
					invalidDeps.CombinableUntriggered = []*changelist.Dep{deps[102], deps[101]}
					s := mustFormat()
					So(s, ShouldContainSubstring, text.Doc(`
				its dependencies weren't CQ-ed at all:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`))
				})
				Convey("Combinable mode mismatch", func() {
					mode = run.FullRun
					invalidDeps.CombinableMismatchedMode = []*changelist.Dep{deps[102], deps[101]}
					s := mustFormat()
					So(s, ShouldContainSubstring, text.Doc(`
				its mode "FULL_RUN" does not match mode on its dependencies:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`))
				})
				Convey("Too many", func() {
					invalidDeps.TooMany = &changelist.CLError_InvalidDeps_TooMany{Actual: 5, MaxAllowed: 4}
					s := mustFormat()
					So(s, ShouldContainSubstring, "has too many deps: 5 (max supported: 4)")
				})
			})
			Convey("Reuse of triggers", func() {
				reason.Kind = &changelist.CLError_ReusedTrigger_{
					ReusedTrigger: &changelist.CLError_ReusedTrigger{Run: "some/123-1-run"},
				}
				So(mustFormat(), ShouldContainSubstring, `previously completed a Run ("some/123-1-run") triggered by the same vote(s)`)
			})
		})

		Convey("Multiple", func() {
			reasons := []*changelist.CLError{
				{Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true}},
				{
					Kind: &changelist.CLError_WatchedByManyConfigGroups_{
						WatchedByManyConfigGroups: &changelist.CLError_WatchedByManyConfigGroups{
							ConfigGroups: []string{"first", "second"},
						},
					},
				},
			}
			s, err := SFormatCLErrors(ctx, reasons, cl, run.DryRun)
			So(err, ShouldBeNil)
			So(s, ShouldContainSubstring, "set preferred email")
			So(s, ShouldContainSubstring, "more than 1 config group")
		})
	})
}
