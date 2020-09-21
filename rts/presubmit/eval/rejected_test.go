// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRejectedPatchSetSource(t *testing.T) {
	t.Parallel()
	Convey(`RejectedPatchSetSource`, t, func() {
		ctx := context.Background()

		src := rejectedPatchSetSource{
			evalRun: &evalRun{
				Eval:      Eval{CacheDir: t.TempDir()},
				startTime: testclock.TestRecentTimeUTC.Truncate(day),
				endTime:   testclock.TestRecentTimeUTC.Truncate(day).Add(7 * day),
			},
		}

		ps := func(change, patchSet int) GerritPatchset {
			return GerritPatchset{
				Change:   GerritChange{Host: "example.googlesource.com", Number: change},
				Patchset: patchSet,
			}
		}

		Convey(`E2E`, func() {
			var providerReq *RejectedPatchSetRequest
			var providerRes []*RejectedPatchSet
			src.RejectedPatchSetProvider = func(req RejectedPatchSetRequest) ([]*RejectedPatchSet, error) {
				providerReq = &req
				var ret []*RejectedPatchSet
				change := 1000
				for d := req.StartTime; !d.Equal(req.EndTime); d = d.Add(day) {
					ret = append(ret,
						&RejectedPatchSet{
							Patchset:  ps(change, 1),
							Timestamp: d,
						},
						&RejectedPatchSet{
							Patchset:  ps(change, 2),
							Timestamp: d,
						},
					)
					change++
				}
				providerRes = ret
				return ret, nil
			}

			res, err := src.Read(ctx)
			So(err, ShouldBeNil)
			So(res, ShouldHaveLength, 14)
			So(res[0].Timestamp, ShouldEqual, src.startTime)
			So(res[0].Patchset.Change.Number, ShouldEqual, 1000)
			So(providerReq.StartTime, ShouldEqual, src.startTime)
			So(providerReq.EndTime, ShouldEqual, src.endTime)

			// Now that we have cache, extend endTime by one day.
			oldEndTime := src.endTime
			src.endTime = src.endTime.Add(day)
			res, err = src.Read(ctx)
			So(err, ShouldBeNil)
			So(providerReq.StartTime, ShouldEqual, oldEndTime)
			So(providerReq.EndTime, ShouldEqual, src.endTime)
			So(providerRes, ShouldHaveLength, 2)
			So(res, ShouldHaveLength, 16)
			So(res[0].Timestamp, ShouldEqual, src.startTime)
			So(res[0].Patchset.Change.Number, ShouldEqual, 1000)
			So(res[15].Timestamp, ShouldEqual, src.endTime.Add(-day))
			So(res[15].Patchset.Change.Number, ShouldEqual, 1000)
		})
	})
}
