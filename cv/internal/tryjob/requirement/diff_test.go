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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	ftt.Run("Diff works", t, func(t *ftt.Test) {
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

		t.Run("Compare definitions", func(t *ftt.Test) {
			t.Run("Empty base and empty target", func(t *ftt.Test) {
				res := Diff(&tryjob.Requirement{}, &tryjob.Requirement{})
				assert.Loosely(t, res.AddedDefs, should.BeEmpty)
				assert.Loosely(t, res.ChangedDefs, should.BeEmpty)
				assert.Loosely(t, res.RemovedDefs, should.BeEmpty)
			})
			t.Run("Empty base", func(t *ftt.Test) {
				def := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				res := Diff(
					&tryjob.Requirement{},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{def},
					})
				assert.Loosely(t, res.AddedDefs, should.HaveLength(1))
				assert.Loosely(t, res.AddedDefs, should.ContainKey(def))
				assert.Loosely(t, res.ChangedDefs, should.BeEmpty)
				assert.Loosely(t, res.RemovedDefs, should.BeEmpty)
			})
			t.Run("Empty target", func(t *ftt.Test) {
				def := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{def},
					},
					&tryjob.Requirement{})
				assert.Loosely(t, res.AddedDefs, should.BeEmpty)
				assert.Loosely(t, res.ChangedDefs, should.BeEmpty)
				assert.Loosely(t, res.RemovedDefs, should.HaveLength(1))
				assert.Loosely(t, res.RemovedDefs, should.ContainKey(def))
			})
			t.Run("Target has one extra", func(t *ftt.Test) {
				shared := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				extra := makeBBTryjobDefinition("a.example.com", "infra", "ci", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{shared},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{proto.Clone(shared).(*tryjob.Definition), extra},
					})
				assert.Loosely(t, res.AddedDefs, should.HaveLength(1))
				assert.Loosely(t, res.AddedDefs, should.ContainKey(extra))
				assert.Loosely(t, res.ChangedDefs, should.BeEmpty)
				assert.Loosely(t, res.UnchangedDefs, should.HaveLength(1))
				assert.Loosely(t, res.UnchangedDefs[shared], should.Resemble(shared))
				assert.Loosely(t, res.RemovedDefs, should.BeEmpty)
			})
			t.Run("Target has one removed", func(t *ftt.Test) {
				shared := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				removed := makeBBTryjobDefinition("a.example.com", "infra", "ci", "someBuilder")
				res := Diff(
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{shared, removed},
					},
					&tryjob.Requirement{
						Definitions: []*tryjob.Definition{proto.Clone(shared).(*tryjob.Definition)},
					})
				assert.Loosely(t, res.AddedDefs, should.BeEmpty)
				assert.Loosely(t, res.ChangedDefs, should.BeEmpty)
				assert.Loosely(t, res.UnchangedDefs, should.HaveLength(1))
				assert.Loosely(t, res.UnchangedDefs[shared], should.Resemble(shared))
				assert.Loosely(t, res.RemovedDefs, should.HaveLength(1))
				assert.Loosely(t, res.RemovedDefs, should.ContainKey(removed))
			})
			t.Run("Target has one changed", func(t *ftt.Test) {
				baseDef := makeBBTryjobDefinition("a.example.com", "infra", "try", "someBuilder")
				targetDef := proto.Clone(baseDef).(*tryjob.Definition)

				check := func(t testing.TB) {
					t.Helper()

					res := Diff(
						&tryjob.Requirement{
							Definitions: []*tryjob.Definition{baseDef},
						},
						&tryjob.Requirement{
							Definitions: []*tryjob.Definition{targetDef},
						})
					assert.Loosely(t, res.AddedDefs, should.BeEmpty, truth.LineContext())
					assert.Loosely(t, res.ChangedDefs, should.HaveLength(1), truth.LineContext())
					assert.Loosely(t, res.ChangedDefs[baseDef], should.Resemble(targetDef), truth.LineContext())
					assert.Loosely(t, res.ChangedDefsReverse, should.HaveLength(1), truth.LineContext())
					assert.Loosely(t, res.ChangedDefsReverse[targetDef], should.Resemble(baseDef), truth.LineContext())
					assert.Loosely(t, res.RemovedDefs, should.BeEmpty, truth.LineContext())
				}

				t.Run("Change in equivalent", func(t *ftt.Test) {
					targetDef.EquivalentTo = makeBBTryjobDefinition("a.example.com", "infra", "try", "equiBuilder")
					check(t)
				})

				t.Run("Change in disable_reuse", func(t *ftt.Test) {
					targetDef.DisableReuse = !baseDef.GetDisableReuse()
					check(t)
				})
			})
			t.Run("Multiple Definitions", func(t *ftt.Test) {
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
					Definitions: []*tryjob.Definition{proto.Clone(builder1).(*tryjob.Definition), builder3Changed, builder4, builderDiffProjChanged},
				}
				rand.Seed(testclock.TestRecentTimeUTC.Unix())
				rand.Shuffle(len(base.Definitions), func(i, j int) {
					base.Definitions[i], base.Definitions[j] = base.Definitions[j], base.Definitions[i]
				})
				rand.Shuffle(len(target.Definitions), func(i, j int) {
					target.Definitions[i], target.Definitions[j] = target.Definitions[j], target.Definitions[i]
				})
				res := Diff(base, target)
				assert.Loosely(t, res.AddedDefs, should.HaveLength(1))
				assert.Loosely(t, res.AddedDefs, should.ContainKey(builder4))
				assert.Loosely(t, res.RemovedDefs, should.HaveLength(1))
				assert.Loosely(t, res.RemovedDefs, should.ContainKey(builder2))
				assert.Loosely(t, res.ChangedDefs, should.HaveLength(2))
				assert.Loosely(t, res.ChangedDefs[builder3], should.Resemble(builder3Changed))
				assert.Loosely(t, res.ChangedDefs[builderDiffProj], should.Resemble(builderDiffProjChanged))
				assert.Loosely(t, res.ChangedDefsReverse, should.HaveLength(2))
				assert.Loosely(t, res.ChangedDefsReverse[builder3Changed], should.Resemble(builder3))
				assert.Loosely(t, res.ChangedDefsReverse[builderDiffProjChanged], should.Resemble(builderDiffProj))
				assert.Loosely(t, res.UnchangedDefs, should.HaveLength(1))
				assert.Loosely(t, res.UnchangedDefs[builder1], should.Resemble(builder1))
			})
		})

		t.Run("Compare retry configuration", func(t *ftt.Test) {
			t.Run("Both empty", func(t *ftt.Test) {
				res := Diff(&tryjob.Requirement{}, &tryjob.Requirement{})
				assert.Loosely(t, res.RetryConfigChanged, should.BeFalse)
			})
			t.Run("Base nil, target non nil", func(t *ftt.Test) {
				res := Diff(
					&tryjob.Requirement{},
					&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 1,
						},
					})
				assert.Loosely(t, res.RetryConfigChanged, should.BeTrue)
			})
			t.Run("Base non nil, target nil", func(t *ftt.Test) {
				res := Diff(
					&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 1,
						},
					},
					&tryjob.Requirement{})
				assert.Loosely(t, res.RetryConfigChanged, should.BeTrue)
			})
			t.Run("Retry config changed", func(t *ftt.Test) {
				res := Diff(
					&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 1,
						},
					},
					&tryjob.Requirement{
						RetryConfig: &cfgpb.Verifiers_Tryjob_RetryConfig{
							SingleQuota: 2,
						},
					})
				assert.Loosely(t, res.RetryConfigChanged, should.BeTrue)
			})
		})
	})
}
