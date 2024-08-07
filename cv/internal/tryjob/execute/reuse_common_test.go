// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	recipe "go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCanReuse(t *testing.T) {
	t.Parallel()

	Convey("canReuseTryjob works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		Convey("reuse allowed", func() {
			Convey("empty mode allowlist", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
						Status:     tryjob.Result_SUCCEEDED,
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseAllowed)
			})

			Convey("explicitly allowed in mode allowlist", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
						Status:     tryjob.Result_SUCCEEDED,
						Output: &recipe.Output{
							Reusability: &recipe.Output_Reusability{
								ModeAllowlist: []string{string(run.DryRun), string(run.FullRun)},
							},
						},
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseAllowed)
			})
		})

		Convey("reuse maybe", func() {
			Convey("triggered fresh tryjob", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_TRIGGERED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseMaybe)
			})

			Convey("pending tryjob", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_PENDING,
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseMaybe)
			})
		})

		Convey("reuse denied", func() {
			Convey("triggered stale tryjob", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_TRIGGERED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseDenied)
			})

			Convey("successfully ended tryjob but stale", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
						Status:     tryjob.Result_SUCCEEDED,
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseDenied)
			})

			Convey("failed tryjob", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
						Status:     tryjob.Result_FAILED_PERMANENTLY,
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseDenied)
			})

			Convey("not in the mode allowlist", func() {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
						Status:     tryjob.Result_SUCCEEDED,
						Output: &recipe.Output{
							Reusability: &recipe.Output_Reusability{
								ModeAllowlist: []string{string(run.DryRun)},
							},
						},
					},
				}
				So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseDenied)
			})

			for _, st := range []tryjob.Status{tryjob.Status_CANCELLED, tryjob.Status_UNTRIGGERED} {
				Convey(fmt.Sprintf("status is %s", st), func() {
					tj := &tryjob.Tryjob{Status: st}
					So(canReuseTryjob(ctx, tj, run.FullRun), ShouldEqual, reuseDenied)
				})
			}
		})
	})
}

func TestComputeReuseKey(t *testing.T) {
	t.Parallel()

	Convey("computeReuseKey works", t, func() {
		cls := []*run.RunCL{
			{
				ID: 22222,
				Detail: &changelist.Snapshot{
					MinEquivalentPatchset: 22,
				},
			},
			{
				ID: 11111,
				Detail: &changelist.Snapshot{
					MinEquivalentPatchset: 11,
				},
			},
		}
		// Should yield the same result as
		// > python3 -c 'import base64;from hashlib import sha256;print(base64.b64encode(sha256(b"\0".join(sorted(b"%d/%d"%(x[0], x[1]) for x in [[22222,22],[11111,11]]))).digest()))'
		So(computeReuseKey(cls), ShouldEqual, "2Yh+hI8zJZFe8ac1TrrFjATWGjhiV9aXsKjNJIhzATk=")
	})
}
