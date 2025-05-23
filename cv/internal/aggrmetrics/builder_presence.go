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

	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type builderPresenceAggregator struct {
	env *common.Env
}

// metrics implements aggregator interface.
func (t *builderPresenceAggregator) metrics() []types.Metric {
	return []types.Metric{metrics.Public.TryjobBuilderPresence}
}

// report implements aggregator interface.
func (t *builderPresenceAggregator) report(ctx context.Context, projects []string) error {
	err := parallel.WorkPool(min(8, len(projects)), func(work chan<- func() error) {
		for _, project := range projects {
			work <- func() error {
				meta, err := prjcfg.GetLatestMeta(ctx, project)
				switch {
				case err != nil:
					return err
				case meta.Status != prjcfg.StatusEnabled:
					// race condition: project gets disabled right before loading the
					// config
					return nil
				}
				cgs, err := meta.GetConfigGroups(ctx)
				if err != nil {
					return err
				}
				for _, cg := range cgs {
					if err := reportBuilders(ctx, t.env, project, cg); err != nil {
						return err
					}
				}
				return nil
			}
		}
	})
	return err
}

func reportBuilder(ctx context.Context, env *common.Env, cgName, project, bname string, cfg *cfgpb.Verifiers_Tryjob_Builder) error {
	bid, err := bbutil.ParseBuilderID(bname)
	if err != nil {
		return err
	}
	def := &tryjob.Definition{
		Backend: &tryjob.Definition_Buildbucket_{
			Buildbucket: &tryjob.Definition_Buildbucket{
				Builder: bid,
				Host:    chromeinfra.BuildbucketHost,
			},
		},
	}
	tryjob.RunWithBuilderMetricsTarget(ctx, env, def, func(ctx context.Context) {
		metrics.Public.TryjobBuilderPresence.Set(ctx, true,
			project,
			cgName,
			cfg.GetIncludableOnly(),
			len(cfg.GetLocationFilters()) > 0,
			cfg.GetExperimentPercentage() > 0,
		)
	})
	return nil
}

func reportBuilders(ctx context.Context, env *common.Env, project string, cg *prjcfg.ConfigGroup) error {
	cgName := cg.Content.GetName()
	for _, b := range cg.Content.GetVerifiers().GetTryjob().GetBuilders() {
		if err := reportBuilder(ctx, env, cgName, project, b.GetName(), b); err != nil {
			return err
		}
		// If the tryjob builder is configured with an equivalent builder,
		// report the equivalent builder with all the configs of
		// the tryjob builder, including incluable_only, config_group, etc.
		if eqb := b.GetEquivalentTo(); eqb != nil {
			if err := reportBuilder(ctx, env, cgName, project, eqb.GetName(), b); err != nil {
				return err
			}
		}
	}
	return nil
}
