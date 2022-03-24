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
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	recipe "go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReuse(t *testing.T) {
	t.Parallel()

	Convey("canReuseTryjob works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// makeTryjob is a helper function to make a sample tryjob with a given Result.
		makeTryjob := func(result *tryjob.Result) *tryjob.Tryjob {
			tj := tryjob.MustBuildbucketID("cr-buildbucket.example.com", 1234).MustCreateIfNotExists(ctx)
			tj.Result = result
			return tj
		}

		// Example Definition, illustrating that a Definition corresponds to a
		// builder. By default, DisableReuse is false.
		def := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host: "cr-buildbucket.example.com",
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "try",
						Builder: "foo",
					},
				},
			},
		}

		Convey("A simple recent tryjob with empty reuse option can be reused", func() {
			// canReuseTryjob only looks at a couple fields in tryjob, so not all
			// fields need to be populated in the example Tryjob.
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output:     &recipe.Output{},
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeTrue)
		})

		Convey("A tryjob is stale and can't be reused when it meets the age threshold", func() {
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge)),
				Output:     &recipe.Output{},
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeFalse)
		})

		Convey("An error is returned if the Tryjob has nil result.", func() {
			tj := makeTryjob(nil)
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			// And the error is not nil.
			So(err, ShouldNotBeNil)
			So(reuse, ShouldBeFalse)
		})

		Convey("A tryjob can be reused if it has nil output.", func() {
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output:     nil,
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeTrue)
		})

		Convey("A tryjob can be reused if it has no Reusability rules.", func() {
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output: &recipe.Output{
					Reusability: nil,
				},
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeTrue)
		})

		Convey("A tryjob can be reused if the reusability mode allowlist is empty.", func() {
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output: &recipe.Output{
					Reusability: &recipe.Output_Reusability{
						ModeAllowlist: []string{},
					},
				},
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeTrue)
		})

		Convey("A tryjob can be reused if its in the mode allowlist", func() {
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output: &recipe.Output{
					Reusability: &recipe.Output_Reusability{
						ModeAllowlist: []string{string(run.FullRun)},
					},
				},
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeTrue)
		})

		Convey("A tryjob cannot be reused if there's a mode allowlist that doesn't contain the target mode", func() {
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output: &recipe.Output{
					Reusability: &recipe.Output_Reusability{
						ModeAllowlist: []string{string(run.DryRun), string(run.QuickDryRun)},
					},
				},
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeFalse)
		})

		Convey("A tryjob cannot be reused if reuse is disabled for the builder", func() {
			def := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "cr-buildbucket.example.com",
						Builder: &buildbucketpb.BuilderID{
							Project: "chromium",
							Bucket:  "try",
							Builder: "presubmit",
						},
					},
				},
				DisableReuse: true,
			}
			tj := makeTryjob(&tryjob.Result{
				CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
				Output:     nil,
			})
			reuse, err := canReuseTryjob(ctx, tj, def, run.FullRun)
			So(err, ShouldBeNil)
			So(reuse, ShouldBeFalse)
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
