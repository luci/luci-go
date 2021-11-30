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

package run

import (
	"testing"

	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSortTryjobs(t *testing.T) {
	t.Parallel()

	Convey("SortTryjobs works", t, func() {
		cv := func(cvid, bbid int64) *Tryjob {
			return &Tryjob{
				Id:         cvid,
				ExternalId: string(tryjob.MustBuildbucketID("bb-host", bbid)),
			}
		}
		sort := func(in ...*Tryjob) []*Tryjob {
			SortTryjobs(in)
			return in
		}
		t5 := cv(5, 11)
		t6 := cv(6, 12)
		t7 := cv(8, 12) // duplicate BB ID is possible
		t8 := cv(9, 10)

		So(sort(t7, t5, t8, t6), ShouldResembleProto, []*Tryjob{t5, t6, t7, t8})

		So(lessTryjob(t5, t6), ShouldBeTrue)
		So(lessTryjob(t6, t5), ShouldBeFalse)
		So(lessTryjob(t5, t5), ShouldBeFalse)

		Convey("CQDaemon compatibility", func() {
			// CQDaemon's tryjobs have no CV-id, only BB ID.
			cqd := func(bbid int64) *Tryjob {
				return &Tryjob{
					Id:         0,
					ExternalId: string(tryjob.MustBuildbucketID("bb-host", bbid)),
				}
			}

			q5 := cqd(11) // same as t5
			q6 := cqd(12) // same as t6
			q8 := cqd(10) // same as t8
			// Among CQD Tryjobs, order is by BB ID only.
			So(sort(q6, q8, q5), ShouldResembleProto, []*Tryjob{q8, q5, q6})

			So(lessTryjob(q5, q6), ShouldBeTrue)
			So(lessTryjob(q6, q5), ShouldBeFalse)
			So(lessTryjob(q5, q5), ShouldBeFalse)

			Convey("CQDaemon and CV mix", func() {
				// This shouldn't happen in practice, but just in case,
				// ensure sane ordering.
				So(sort(q6, t6, t7, q8, t8, q5, t5), ShouldResembleProto, []*Tryjob{
					q8, q5, q6, // CQD ones are first
					t5, t6, t7, t8, // CV ones are second
				})
			})
		})
	})
}

func TestDiffTryjobsForReport(t *testing.T) {
	t.Parallel()

	Convey("DiffTryjobsForReport works", t, func() {
		mk := func(cvid int64, ts tryjob.Status, rs ...tryjob.Result_Status) *Tryjob {
			t := &Tryjob{
				Id:     cvid,
				Status: ts,
			}
			if ts != tryjob.Status_PENDING {
				t.ExternalId = string(tryjob.MustBuildbucketID("bb-host", cvid))
			}
			if len(rs) != 0 {
				t.Result = &tryjob.Result{Status: rs[0]}
			}
			return t
		}

		t1pending := mk(1, tryjob.Status_PENDING)
		t1scheduled := mk(1, tryjob.Status_TRIGGERED)
		t1succeeded := mk(1, tryjob.Status_ENDED, tryjob.Result_SUCCEEDED)

		t2scheduled := mk(2, tryjob.Status_TRIGGERED)
		t2cancelled := mk(2, tryjob.Status_CANCELLED)

		t3running := mk(3, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
		t3failed := mk(3, tryjob.Status_ENDED, tryjob.Result_FAILED_PERMANENTLY)

		t4running := mk(4, tryjob.Status_TRIGGERED, tryjob.Result_UNKNOWN)
		t4transient := mk(4, tryjob.Status_ENDED, tryjob.Result_FAILED_TRANSIENTLY)

		Convey("same", func() {
			So(DiffTryjobsForReport(
				[]*Tryjob{t1pending, t2scheduled},
				[]*Tryjob{t1pending, t2scheduled},
			), ShouldBeEmpty)
		})
		Convey("removed tryjobs don't make it into diff", func() {
			So(DiffTryjobsForReport(
				[]*Tryjob{t1succeeded, t2cancelled, t3failed},
				[]*Tryjob{t1succeeded, t2cancelled},
			), ShouldBeEmpty)
		})
		Convey("new", func() {
			So(DiffTryjobsForReport(
				[]*Tryjob{t1scheduled, t4running},
				[]*Tryjob{t1scheduled, t3running, t4running},
			), ShouldResembleProto,
				[]*Tryjob{t3running},
			)
		})
		Convey("new and removed", func() {
			So(DiffTryjobsForReport(
				[]*Tryjob{t1scheduled, t3running, t4transient},
				[]*Tryjob{t1scheduled, t2scheduled, t4transient},
			), ShouldResembleProto,
				[]*Tryjob{t2scheduled},
			)
		})
		Convey("new and updated", func() {
			So(DiffTryjobsForReport(
				[]*Tryjob{t1pending, t2scheduled, t3running},
				[]*Tryjob{t1succeeded, t2scheduled, t3failed, t4running},
			), ShouldResembleProto,
				[]*Tryjob{t1succeeded, t3failed, t4running},
			)
		})
	})
}
