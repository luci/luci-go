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

package requirement

import (
	"math/rand"
	"testing"

	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	Convey("Diff works", t, func() {
		makeBBTryjobDefinition := func(host, project, bucket, builder string) *tryjob.Definition {
			return &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: host,
						Builder: &buildbucketpb.BuilderID{
							Project: project,
							Bucket:  bucket,
							Builder: builder,
						},
					},
				},
			}
		}

		Convey("Compare definitions", func() {
			Convey("Empty base and empty target", func() {
				res := Diff(&tryjob.Requirement{}, &tryjob.Requirement{})
				So(res.AddedDefs, ShouldBeEmpty)
				So(res.ChangedDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldBeEmpty)
			})
			Convey("Empty base", func() {
				def := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				res := Diff(
					&tryjob.Requirement{},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{def},
					})
				So(res.AddedDefs, ShouldHaveLength, 1)
				So(res.AddedDefs, ShouldContainKey, def)
				So(res.ChangedDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldBeEmpty)
			})
			Convey("Empty target", func() {
				def := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{def},
					},
					&tryjob.Requirement{})
				So(res.AddedDefs, ShouldBeEmpty)
				So(res.ChangedDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldHaveLength, 1)
				So(res.RemovedDefs, ShouldContainKey, def)
			})
			Convey("Target has one extra", func() {
				shared := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				extra := makeBBTryjobDefinition("a.example.com", "infra", "ci", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{shared},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{shared, extra},
					})
				So(res.AddedDefs, ShouldHaveLength, 1)
				So(res.AddedDefs, ShouldContainKey, extra)
				So(res.ChangedDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldBeEmpty)
			})
			Convey("Target has one removed", func() {
				shared := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				removed := makeBBTryjobDefinition("a.example.com", "infra", "ci", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{shared, removed},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{shared},
					})
				So(res.AddedDefs, ShouldBeEmpty)
				So(res.ChangedDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldHaveLength, 1)
				So(res.RemovedDefs, ShouldContainKey, removed)
			})
			Convey("Target has one changed", func() {
				baseDef := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				targetDef := proto.Clone(baseDef).(*tryjob.Definition)
				Convey("Change in equivalent", func() {
					targetDef.EquivalentTo = makeBBTryjobDefinition("a.example.com", "infra", "try", "equiBuilder")
				})
				Convey("Change in disable_reuse", func() {
					targetDef.DisableReuse = !baseDef.GetDisableReuse()
				})
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{baseDef},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{targetDef},
					})
				So(res.AddedDefs, ShouldBeEmpty)
				So(res.ChangedDefs, ShouldHaveLength, 1)
				So(res.ChangedDefs[baseDef], ShouldResembleProto, targetDef)
				So(res.RemovedDefs, ShouldBeEmpty)
			})
			Convey("Multiple Definitions", func() {
				builder1 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder1")
				builder2 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder2")
				builder3 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder3")
				builder3Changed := proto.Clone(builder3).(*tryjob.Definition)
				builder3Changed.DisableReuse = !builder3.GetDisableReuse()
				builder4 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder4")
				builderDiffProj := makeBBTryjobDefinition("a.example.com", "chrome", "try", "builder1")
				builderDiffProjChanged := proto.Clone(builderDiffProj).(*tryjob.Definition)
				builderDiffProjChanged.EquivalentTo = makeBBTryjobDefinition("a.example.com", "chrome", "try", "equi-builder1")

				base := &tryjob.Requirement{
					Definitions: []*tryjob.Definition{builder1, builder2, builder3, builderDiffProj},
				}
				target := &tryjob.Requirement{
					Definitions: []*tryjob.Definition{builder1, builder3Changed, builder4, builderDiffProjChanged},
				}
				rand.Seed(testclock.TestRecentTimeUTC.Unix())
				rand.Shuffle(len(base.Definitions), func(i, j int) {
					base.Definitions[i], base.Definitions[j] = base.Definitions[j], base.Definitions[i]
				})
				rand.Shuffle(len(target.Definitions), func(i, j int) {
					target.Definitions[i], target.Definitions[j] = target.Definitions[j], target.Definitions[i]
				})
				res := Diff(base, target)
				So(res.AddedDefs, ShouldHaveLength, 1)
				So(res.AddedDefs, ShouldContainKey, builder4)
				So(res.RemovedDefs, ShouldHaveLength, 1)
				So(res.RemovedDefs, ShouldContainKey, builder2)
				So(res.ChangedDefs, ShouldHaveLength, 2)
				So(res.ChangedDefs[builder3], ShouldResembleProto, builder3Changed)
				So(res.ChangedDefs[builderDiffProj], ShouldResembleProto, builderDiffProjChanged)
			})
		})

		Convey("Compare retry configuration", func() {
			Convey("Both empty", func() {
				res := Diff(&tryjob.Requirement{}, &tryjob.Requirement{})
				So(res.RetryConfigChanged, ShouldBeFalse)
			})
			Convey("Base nil, target non nil", func() {
				res := Diff(
					&tryjob.Requirement{},
					&tryjob.Requirement{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 1,
						},
					})
				So(res.RetryConfigChanged, ShouldBeTrue)
			})
			Convey("Base non nil, target nil", func() {
				res := Diff(
					&tryjob.Requirement{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 1,
						},
					},
					&tryjob.Requirement{})
				So(res.RetryConfigChanged, ShouldBeTrue)
			})
			Convey("Retry config changed", func() {
				res := Diff(
					&tryjob.Requirement{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 1,
						},
					},
					&tryjob.Requirement{
						RetryConfig: &config.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
						},
					})
				So(res.RetryConfigChanged, ShouldBeTrue)
			})
		})
	})
}
