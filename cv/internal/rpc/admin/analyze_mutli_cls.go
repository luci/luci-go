// Copyright 2023 The LUCI Authors.
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

package admin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/dsmapper"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

var multiCLAnalysisConfig = dsmapper.JobConfig{
	Mapper: "multiCLAnalysis",
	Query: dsmapper.Query{
		Kind: "Run",
	},
	PageSize:   32,
	ShardCount: 4,
}

var multiCLAnalysisMapperFactory = func(_ context.Context, j *dsmapper.Job, _ int) (dsmapper.Mapper, error) {
	analyzeMultiCLs := func(ctx context.Context, keys []*datastore.Key) error {
		runs := make([]*run.Run, len(keys))
		for i, k := range keys {
			runs[i] = &run.Run{
				ID: common.RunID(k.StringID()),
			}
		}

		// Check before a transaction if an update is even necessary.
		if err := datastore.Get(ctx, runs); err != nil {
			return errors.Annotate(err, "failed to fetch RunCLs").Tag(transient.Tag).Err()
		}
		// Only interested in all multi CL runs created between 2022-08-01 to
		// 2023-08-01
		multiCLRuns := runs[:0]
		for _, r := range runs {
			switch {
			case len(r.CLs) < 2:
			case r.CreateTime.Before(time.Date(2022, 8, 1, 0, 0, 0, 0, time.UTC)):
			case r.CreateTime.After(time.Date(2023, 8, 1, 0, 0, 0, 0, time.UTC)):
			default:
				multiCLRuns = append(multiCLRuns, r)
			}
		}
		if len(multiCLRuns) == 0 {
			return nil
		}

		transport, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
		if err != nil {
			return errors.Annotate(err, "failed to create Google Storage RPC transport").Err()
		}
		gsClient, err := gs.NewProdClient(ctx, transport)
		if err != nil {
			return errors.Annotate(err, "Failed to create GS client.").Err()
		}
		defer func() { _ = gsClient.Close() }()

		eg, ectx := errgroup.WithContext(ctx)
		eg.SetLimit(8)
		for _, r := range multiCLRuns {
			r := r
			eg.Go(func() error {
				cls, err := run.LoadRunCLs(ectx, r.ID, r.CLs)
				if err != nil {
					return errors.Annotate(err, "failed to load RunCLs").Tag(transient.Tag).Err()
				}
				clGraph := makeCLGraph(cls)
				if !clGraph.hasRootCL() {
					writer, err := gsClient.NewWriter(gs.MakePath("temp-multi-cl-graph-store", fmt.Sprintf("%s-%d", j.Config.Mapper, j.ID), fmt.Sprintf("%s.txt", r.ID)))
					if err != nil {
						return errors.Annotate(err, "failed to create new gs writer").Err()
					}
					defer func() { _ = writer.Close() }()
					if _, err := writer.Write([]byte(clGraph.computeDotGraph())); err != nil {
						return errors.Annotate(err, "failed to write to gs object").Err()
					}
				}
				return nil
			})
		}
		return eg.Wait()
	}

	return analyzeMultiCLs, nil
}

type clGraph struct {
	deps  map[common.CLID][]*changelist.Dep
	clMap map[common.CLID]*run.RunCL
}

func makeCLGraph(cls []*run.RunCL) clGraph {
	ret := clGraph{}
	ret.clMap = make(map[common.CLID]*run.RunCL, len(cls))
	externalIDToCL := make(map[changelist.ExternalID]*run.RunCL, len(cls))
	for _, cl := range cls {
		ret.clMap[cl.ID] = cl
		externalIDToCL[cl.ExternalID] = cl
	}
	ret.deps = make(map[common.CLID][]*changelist.Dep, len(cls))
	for _, cl := range cls {
		for _, hardDep := range cl.Detail.GetGerrit().GetGitDeps() {
			if !hardDep.GetImmediate() {
				continue
			}
			depExternalID := changelist.MustGobID(cl.Detail.GetGerrit().GetHost(), hardDep.GetChange())
			// A CL may depend on an already submitted CL that are not part of the
			// Run. Ignore them.
			if depCL, ok := externalIDToCL[depExternalID]; ok {
				ret.deps[cl.ID] = append(ret.deps[cl.ID], &changelist.Dep{
					Clid: int64(depCL.ID),
					Kind: changelist.DepKind_HARD,
				})
			}

		}

		for _, softDep := range cl.Detail.GetGerrit().GetSoftDeps() {
			depExternalID := changelist.MustGobID(softDep.GetHost(), softDep.GetChange())
			// A CL may depend on an already submitted CL that are not part of the
			// Run. Ignore them.
			if depCL, ok := externalIDToCL[depExternalID]; ok {
				ret.deps[cl.ID] = append(ret.deps[cl.ID], &changelist.Dep{
					Clid: int64(depCL.ID),
					Kind: changelist.DepKind_SOFT,
				})
			}
		}
	}
	return ret
}

// hasRootCL checks whether a multi CL Run has any root CLs.
//
// A root CL means if we start from this CL following its dependencies (i.e.
// hard dep == git parent-child relationship, soft dep == `cq-depend` footer),
// it can reach all other CLs involved in the Run.
func (g clGraph) hasRootCL() bool {
	for cl := range g.clMap {
		if g.canTraverseAllCLs(cl) {
			return true
		}
	}
	return false
}

func (g clGraph) canTraverseAllCLs(root common.CLID) bool {
	visited := common.CLIDsSet{}
	g.dfs(root, visited)
	return len(visited) == g.numCls()
}

func (g clGraph) dfs(cl common.CLID, visited common.CLIDsSet) {
	visited.Add(cl)
	for _, depCL := range g.depCLIDs(cl) {
		if !visited.Has(depCL) {
			g.dfs(common.CLID(depCL), visited)
		}
	}
}

func (g clGraph) numCls() int {
	return len(g.clMap)
}

func (g clGraph) depCLIDs(cl common.CLID) common.CLIDs {
	var ret common.CLIDs
	for _, dep := range g.deps[cl] {
		ret = append(ret, common.CLID(dep.Clid))
	}
	return ret
}

func (g clGraph) computeDotGraph() string {
	var nodeWriter strings.Builder
	var depsWriter strings.Builder

	for _, clid := range g.sortedCLs() {
		cl := g.clMap[clid]
		fmt.Fprintf(&nodeWriter, `  "%s" [href="%s", target="_parent"]`, cl.ExternalID, cl.ExternalID.MustURL())
		nodeWriter.WriteString("\n")

		for _, dep := range g.deps[clid] {
			depCL := g.clMap[common.CLID(dep.Clid)]
			fmt.Fprintf(&depsWriter, `  "%s" -> "%s"`, cl.ExternalID, depCL.ExternalID)
			if dep.Kind == changelist.DepKind_SOFT {
				depsWriter.WriteString(`[style="dotted"]`)
			}
			depsWriter.WriteString("\n")
		}
	}

	return fmt.Sprintf("digraph {\n%s\n%s}", nodeWriter.String(), depsWriter.String())
}

func (g clGraph) sortedCLs() common.CLIDs {
	ret := make(common.CLIDs, 0, len(g.clMap))
	for cl := range g.clMap {
		ret = append(ret, cl)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i] < ret[j]
	})
	return ret
}
