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
	"sync"

	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/hardcoded/chromeinfra"

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

type builderInfo struct {
	configGroup       string
	definition        *tryjob.Definition
	includableOnly    bool
	hasLocationFilter bool
	experimental      bool
}

// metrics implements aggregator interface.
func (t *builderPresenceAggregator) prepare(ctx context.Context, activeProjects stringset.Set) (reportFunc, error) {
	projects := activeProjects.ToSlice()
	infosByProject := make(map[string][]builderInfo, len(projects))
	var infosByProjectMu sync.Mutex

	err := parallel.WorkPool(min(8, len(projects)), func(work chan<- func() error) {
		for _, project := range projects {
			project := project
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
				builderInfos, err := computeBuilderInfos(cgs)
				if err != nil {
					return err
				}
				infosByProjectMu.Lock()
				infosByProject[project] = builderInfos
				infosByProjectMu.Unlock()
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for project, builderInfos := range infosByProject {
			for _, builderInfo := range builderInfos {
				metrics.RunWithBuilderTarget(ctx, t.env, builderInfo.definition, func(ctx context.Context) {
					metrics.Public.TryjobBuilderPresence.Set(ctx, true,
						project,
						builderInfo.configGroup,
						builderInfo.includableOnly,
						builderInfo.hasLocationFilter,
						builderInfo.experimental,
					)
				})
			}
		}
	}, nil
}

func computeBuilderInfos(cgs []*prjcfg.ConfigGroup) ([]builderInfo, error) {
	var ret []builderInfo
	for _, cg := range cgs {
		cgName := cg.Content.GetName()
		for _, b := range cg.Content.GetVerifiers().GetTryjob().GetBuilders() {
			builderID, err := bbutil.ParseBuilderID(b.GetName())
			if err != nil {
				return nil, err
			}
			ret = append(ret, builderInfo{
				configGroup: cgName,
				definition: &tryjob.Definition{
					Backend: &tryjob.Definition_Buildbucket_{
						Buildbucket: &tryjob.Definition_Buildbucket{
							Host:    chromeinfra.BuildbucketHost,
							Builder: builderID,
						},
					},
				},
				includableOnly:    b.GetIncludableOnly(),
				hasLocationFilter: len(b.GetLocationFilters()) > 0 || (len(b.GetLocationRegexp())+len(b.GetLocationRegexpExclude()) > 0),
				experimental:      b.GetExperimentPercentage() > 0,
			})
		}
	}
	return ret, nil
}
