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
				So(res.ExtraDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldBeEmpty)
			})
			Convey("Empty base", func() {
				def := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				res := Diff(
					&tryjob.Requirement{},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{def},
					})
				So(res.ExtraDefs, ShouldResembleProto, []*tryjob.Definition{def})
				So(res.RemovedDefs, ShouldBeEmpty)
			})
			Convey("Empty target", func() {
				def := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{def},
					},
					&tryjob.Requirement{})
				So(res.ExtraDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldResembleProto, []*tryjob.Definition{def})
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
				So(res.ExtraDefs, ShouldResembleProto, []*tryjob.Definition{extra})
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
				So(res.ExtraDefs, ShouldBeEmpty)
				So(res.RemovedDefs, ShouldResembleProto, []*tryjob.Definition{removed})
			})
			Convey("Target has one changed", func() {
				baseDef := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				targetDef := makeBBTryjobDefinition("a.example.com", "infra", "ci", "anotherBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{baseDef},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{targetDef},
					})
				So(res.ExtraDefs, ShouldResembleProto, []*tryjob.Definition{targetDef})
				So(res.RemovedDefs, ShouldResembleProto, []*tryjob.Definition{baseDef})
			})
			Convey("Multiple Definitions", func() {
				builder1 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder1")
				builder2 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder2")
				builder3 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder3")
				builder4 := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder4")
				builderDiffProj := makeBBTryjobDefinition("a.example.com", "chrome", "try", "builder")
				builderDiffHost1 := makeBBTryjobDefinition("b.example.com", "infra", "try", "builder1")
				builderDiffHost2 := makeBBTryjobDefinition("b.example.com", "infra", "try", "builder2")
				base := &tryjob.Requirement{
					Definitions: []*tryjob.Definition{builder1, builder2, builder3, builderDiffHost1},
				}
				target := &tryjob.Requirement{
					Definitions: []*tryjob.Definition{builder1, builder3, builder4, builderDiffProj, builderDiffHost2},
				}
				rand.Seed(testclock.TestRecentTimeUTC.Unix())
				rand.Shuffle(len(base.Definitions), func(i, j int) {
					base.Definitions[i], base.Definitions[j] = base.Definitions[j], base.Definitions[i]
				})
				rand.Shuffle(len(target.Definitions), func(i, j int) {
					target.Definitions[i], target.Definitions[j] = target.Definitions[j], target.Definitions[i]
				})
				res := Diff(base, target)
				So(res.ExtraDefs, ShouldResembleProto, []*tryjob.Definition{builderDiffProj, builder4, builderDiffHost2})
				So(res.RemovedDefs, ShouldResembleProto, []*tryjob.Definition{builder2, builderDiffHost1})
			})
			Convey("With equivalent Tryjob definition", func() {
				baseDef := makeBBTryjobDefinition("a.example.com", "infra", "try", "builder")
				targetDef := proto.Clone(baseDef).(*tryjob.Definition)
				targetDef.EquivalentTo = makeBBTryjobDefinition("a.example.com", "infra", "try", "equi_builder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{baseDef},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{targetDef},
					})
				So(res.ExtraDefs, ShouldResembleProto, []*tryjob.Definition{targetDef})
				So(res.RemovedDefs, ShouldResembleProto, []*tryjob.Definition{baseDef})
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
