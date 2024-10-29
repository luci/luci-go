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

package aggrmetrics

import (
	"context"
	"fmt"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestBuilderPresenceAggregator(t *testing.T) {
	t.Parallel()

	ftt.Run("builderPresenceAggregator works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject        = "test_proj"
			configGroupName = "test_config_group"
			bucketName      = "test_bucket"
			builderName     = "test_builder"
		)

		tryjobDef := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host: chromeinfra.BuildbucketHost,
					Builder: &bbpb.BuilderID{
						Project: lProject,
						Bucket:  bucketName,
						Builder: builderName,
					},
				},
			},
		}

		builder := &cfgpb.Verifiers_Tryjob_Builder{
			Name: bbutil.FormatBuilderID(tryjobDef.GetBuildbucket().GetBuilder()),
		}

		config := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: configGroupName,
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								builder,
							},
						},
					},
				},
			},
		}

		prjcfgtest.Create(ctx, lProject, config)

		mustReport := func(active ...string) {
			bpa := builderPresenceAggregator{env: ct.Env}
			err := bpa.report(ctx, active)
			assert.NoErr(t, err)
		}

		t.Run("Skip disabled project", func(t *ftt.Test) {
			prjcfgtest.Disable(ctx, lProject)
			mustReport(lProject)
			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.BeEmpty)
		})

		t.Run("Plain builder", func(t *ftt.Test) {
			mustReport(lProject)
			tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobBuilderPresence, lProject, configGroupName, false, false, false), should.BeTrue)
			})
		})

		t.Run("Includable only builder", func(t *ftt.Test) {
			builder.IncludableOnly = true
			prjcfgtest.Update(ctx, lProject, config)
			mustReport(lProject)
			tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobBuilderPresence, lProject, configGroupName, true, false, false), should.BeTrue)
			})
		})

		t.Run("With Location filter", func(t *ftt.Test) {
			builder.LocationFilters = append(builder.LocationFilters, &cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				PathRegexp: `.*\.md`,
			})
			prjcfgtest.Update(ctx, lProject, config)
			mustReport(lProject)
			tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobBuilderPresence, lProject, configGroupName, false, true, false), should.BeTrue)
			})
		})

		t.Run("Experimental", func(t *ftt.Test) {
			builder.ExperimentPercentage = 99.9
			prjcfgtest.Update(ctx, lProject, config)
			mustReport(lProject)
			tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobBuilderPresence, lProject, configGroupName, false, false, true), should.BeTrue)
			})
		})

		t.Run("Multiple projects, config groups, builders", func(t *ftt.Test) {
			var builderCount int
			genBuilders := func(count int) []*cfgpb.Verifiers_Tryjob_Builder {
				ret := make([]*cfgpb.Verifiers_Tryjob_Builder, count)
				for i := 0; i < count; i++ {
					ret[i] = &cfgpb.Verifiers_Tryjob_Builder{
						Name: fmt.Sprintf("%s/test-bucket/builder-%04d", lProject, builderCount+i),
					}
				}
				builderCount += count
				return ret
			}

			prjcfgtest.Create(ctx, "prj-0", &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "cg-0",
						Verifiers: &cfgpb.Verifiers{
							Tryjob: &cfgpb.Verifiers_Tryjob{
								Builders: genBuilders(3),
							},
						},
					},
					{
						Name: "cg-1",
						Verifiers: &cfgpb.Verifiers{
							Tryjob: &cfgpb.Verifiers_Tryjob{
								Builders: genBuilders(7),
							},
						},
					},
				},
			})
			prjcfgtest.Create(ctx, "prj-1", &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "cg-0",
						Verifiers: &cfgpb.Verifiers{
							Tryjob: &cfgpb.Verifiers_Tryjob{
								Builders: genBuilders(37),
							},
						},
					},
				},
			})
			prjcfgtest.Create(ctx, "prj-2", &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "cg-0",
						Verifiers: &cfgpb.Verifiers{
							Tryjob: &cfgpb.Verifiers_Tryjob{
								Builders: genBuilders(29),
							},
						},
					},
				},
			})
			mustReport("prj-0", "prj-1", "prj-2")
			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.HaveLength(builderCount))
		})

		t.Run("With EquivalentTo", func(t *ftt.Test) {
			const eqBuilderName = "eq_test_builder"
			eqb := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: chromeinfra.BuildbucketHost,
						Builder: &bbpb.BuilderID{
							Project: lProject,
							Bucket:  bucketName,
							Builder: eqBuilderName,
						},
					},
				},
			}
			builder.EquivalentTo = &cfgpb.Verifiers_Tryjob_EquivalentBuilder{
				Name: bbutil.FormatBuilderID(eqb.GetBuildbucket().GetBuilder()),
			}
			prjcfgtest.Update(ctx, lProject, config)
			mustReport(lProject)

			// both tryjobs should be reported.
			tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, tryjobDef, func(ctx context.Context) {
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobBuilderPresence, lProject, configGroupName, false, false, false), should.BeTrue)
			})
			tryjob.RunWithBuilderMetricsTarget(ctx, ct.Env, eqb, func(ctx context.Context) {
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.TryjobBuilderPresence, lProject, configGroupName, false, false, false), should.BeTrue)
			})
		})
	})
}
