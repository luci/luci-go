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

package bbfacade

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// AcceptedAdditionalPropKeys are additional properties keys that, if present
// in the requested properties of the build, indicate that LUCI CV should still
// consider the build as reusable.
//
// LUCI CV checks requested properties rather than input properties because
// LUCI CV only cares about whether the properties used by a build are
// different from the pre-defined properties in Project Config (assuming change
// in properties may result in change in build result). Requested properties
// are properties provided in ScheduleBuild, which currently is the only way to
// add/modify build properties. LUCI CV permits certain keys which are either
// added by LUCI CV itself, or known to not change build behavior.
var AcceptedAdditionalPropKeys = stringset.NewFromSlice(
	"$recipe_engine/cq", // TODO: crbug/333811087 - remove
	"$recipe_engine/cv", // future proof
	"requester",
)

var searchBuildsMask *bbpb.BuildMask

func init() {
	searchBuildsMask = proto.Clone(defaultMask).(*bbpb.BuildMask)
	if err := searchBuildsMask.Fields.Append((*bbpb.Build)(nil),
		"builder",
		"input.gerrit_changes",
		"infra.buildbucket.requested_properties",
	); err != nil {
		panic(err)
	}
}

// Search searches Buildbucket for builds that match all provided CLs and
// any of the provided definitions.
//
// Also filters out builds that specify extra properties. See:
// `AcceptedAdditionalPropKeys`.
//
// `cb` is invoked for each matching Tryjob converted from Buildbucket build
// until `cb` returns false or all matching Tryjobs are exhausted or error
// occurs. The Tryjob `cb` receives only populates following fields:
//   - ExternalID
//   - Definition
//   - Status
//   - Result
//   - CLPatchsets
//
// Also, the Tryjobs are guaranteed to have decreasing build ID (in other
// word, from newest to oldest) ONLY within the same host.
// For example, for following matching builds:
//   - host: A, build: 100, create time: now
//   - host: A, build: 101, create time: now - 2min
//   - host: B, build: 1000, create time: now - 1min
//   - host: B, build: 1001, create time: now - 3min
//
// It is possible that `cb` is called in following orders:
//   - host: B, build: 1000, create time: now - 1min
//   - host: A, build: 100, create time: now
//   - host: B, build: 1001, create time: now - 3min
//   - host: A, build: 101, create time: now - 2min
//
// TODO(crbug/1369200): Fix the edge case that may cause Search failing to
// return newer builds before older builds across different patchsets.
// TODO(yiwzhang): ensure `cb` get called from newest to oldest builds across
// all hosts.
//
// Uses the provided `luciProject` for authentication. If any of the given
// definitions defines builder from other LUCI Project, this other LUCI Project
// should grant bucket READ permission to the provided `luciProject`.
// Otherwise, the builds won't show up in the search result.
func (f *Facade) Search(ctx context.Context, cls []*run.RunCL, definitions []*tryjob.Definition, luciProject string, cb func(*tryjob.Tryjob) bool) error {
	shouldStop, stop := makeStopFunction()
	workers, err := f.makeSearchWorkers(ctx, cls, definitions, luciProject, shouldStop)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(workers))
	resultCh := make(chan searchResult)
	for _, worker := range workers {
		worker := worker
		go func() {
			defer wg.Done()
			worker.search(ctx, resultCh)
		}()
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for res := range resultCh {
		switch {
		case shouldStop(): // draining
			continue
		case res.err != nil:
			err = res.err
			stop()
		case !cb(res.tryjob):
			stop()
		}
	}
	return err
}

func makeStopFunction() (shouldStop func() bool, stop func()) {
	var stopIndicator int32
	shouldStop = func() bool {
		return atomic.LoadInt32(&stopIndicator) > 0
	}
	stop = func() {
		atomic.AddInt32(&stopIndicator, 1)
	}
	return shouldStop, stop
}

// searchWorker is a worker search builds against a single Buildbucket host.
//
// The matching build will be pushed to `resultCh` one by one including any
// error occurred during the search. `resultCh` is closed when either a error
// is returned or all matching builds have been exhausted.
//
// Algorithm for searching:
//   - Pick the CL with smallest (patchset - min_equivalent_patchset) as the
//     search predicate
//   - Page the search response and accept a build if
//   - The Gerrit changes of the build matches the input CLs. Match means
//     host and change number are the same and the patchset is in between
//     cl.min_equivalent_patchset and cl.patchset
//   - The builder of the build should be either the main builder or the
//     equivalent builder of any of the input definitions
//   - The requested properties only have keys specified in
//     `AcceptedPropertyKeys`
type searchWorker struct {
	bbHost              string
	luciProject         string
	bbClient            buildbucket.Client
	clSearchTarget      *run.RunCL
	acceptedCLRanges    map[string]patchsetRange
	changeIDToCLID      map[string]common.CLID
	builderToDefinition map[string]*tryjob.Definition
	shouldStop          func() bool
}

type patchsetRange struct {
	minIncl, maxIncl int64
}

func (f *Facade) makeSearchWorkers(ctx context.Context, cls []*run.RunCL, definitions []*tryjob.Definition, luciProject string, shouldStop func() bool) ([]searchWorker, error) {
	var hostToWorker = make(map[string]searchWorker)
	for _, def := range definitions {
		if def.GetBuildbucket() == nil {
			panic(fmt.Errorf("call buildbucket backend for non-buildbucket definition: %s", def))
		}

		bbHost := def.GetBuildbucket().GetHost()
		worker, ok := hostToWorker[bbHost]
		if !ok {
			bbClient, err := f.ClientFactory.MakeClient(ctx, bbHost, luciProject)
			if err != nil {
				return nil, err
			}
			clRanges, clWithSmallestRange := computeCLRangesAndPickSmallest(cls)
			worker = searchWorker{
				bbHost:              bbHost,
				luciProject:         luciProject,
				bbClient:            bbClient,
				acceptedCLRanges:    clRanges,
				clSearchTarget:      clWithSmallestRange,
				changeIDToCLID:      make(map[string]common.CLID, len(cls)),
				builderToDefinition: make(map[string]*tryjob.Definition),
				shouldStop:          shouldStop,
			}
			for _, cl := range cls {
				worker.changeIDToCLID[formatChangeID(cl.Detail.GetGerrit().GetHost(), cl.Detail.GetGerrit().GetInfo().GetNumber())] = cl.ID
			}
			hostToWorker[bbHost] = worker
		}
		worker.builderToDefinition[bbutil.FormatBuilderID(def.GetBuildbucket().GetBuilder())] = def
		if def.GetEquivalentTo() != nil {
			worker.builderToDefinition[bbutil.FormatBuilderID(def.GetEquivalentTo().GetBuildbucket().GetBuilder())] = def
		}
	}
	ret := make([]searchWorker, 0, len(hostToWorker))
	for _, worker := range hostToWorker {
		ret = append(ret, worker)
	}
	return ret, nil
}

func computeCLRangesAndPickSmallest(cls []*run.RunCL) (map[string]patchsetRange, *run.RunCL) {
	clToRange := make(map[string]patchsetRange, len(cls))
	var clWithSmallestPatchsetRange *run.RunCL
	var smallestRange int64
	for _, cl := range cls {
		psRange := struct {
			minIncl int64
			maxIncl int64
		}{int64(cl.Detail.GetMinEquivalentPatchset()), int64(cl.Detail.GetPatchset())}
		clToRange[formatChangeID(cl.Detail.GetGerrit().GetHost(), cl.Detail.GetGerrit().GetInfo().GetNumber())] = psRange
		if r := psRange.maxIncl - psRange.minIncl + 1; smallestRange == 0 || r < smallestRange {
			clWithSmallestPatchsetRange = cl
			smallestRange = r
		}
	}
	return clToRange, clWithSmallestPatchsetRange
}

type searchResult struct {
	tryjob *tryjob.Tryjob
	err    error
}

func (sw *searchWorker) search(ctx context.Context, resultCh chan<- searchResult) {
	changeDetail := sw.clSearchTarget.Detail
	gc := &bbpb.GerritChange{
		Host:    changeDetail.GetGerrit().GetHost(),
		Project: changeDetail.GetGerrit().GetInfo().GetProject(),
		Change:  changeDetail.GetGerrit().GetInfo().GetNumber(),
	}
	req := &bbpb.SearchBuildsRequest{
		Predicate: &bbpb.BuildPredicate{
			GerritChanges:       []*bbpb.GerritChange{gc},
			IncludeExperimental: true,
		},
		Mask: searchBuildsMask,
	}
	for ps := changeDetail.GetPatchset(); ps >= changeDetail.GetMinEquivalentPatchset(); ps-- {
		gc.Patchset = int64(ps)
		req.PageToken = ""
		for {
			if sw.shouldStop() {
				return
			}
			res, err := sw.bbClient.SearchBuilds(ctx, req)
			if err != nil {
				resultCh <- searchResult{err: errors.Annotate(err, "failed to call buildbucket.SearchBuilds").Tag(transient.Tag).Err()}
				return
			}
			for _, build := range res.GetBuilds() {
				if def, ok := sw.canUseBuild(build); ok {
					tj, err := sw.toTryjob(ctx, build, def)
					if err != nil {
						resultCh <- searchResult{err: err}
						return
					}
					resultCh <- searchResult{tryjob: tj}
				}
			}
			if res.NextPageToken == "" {
				break
			}
			req.PageToken = res.NextPageToken
		}
	}
}

func (sw searchWorker) canUseBuild(build *bbpb.Build) (*tryjob.Definition, bool) {
	switch def, matchBuilder := sw.builderToDefinition[bbutil.FormatBuilderID(build.GetBuilder())]; {
	case !matchBuilder:
	case !sw.matchCLs(build):
	case hasAdditionalProperties(build):
	default:
		return def, true
	}
	return nil, false
}

func (sw searchWorker) matchCLs(build *bbpb.Build) bool {
	gcs := build.GetInput().GetGerritChanges()
	changeToPatchset := make(map[string]int64, len(gcs))
	for _, gc := range gcs {
		changeToPatchset[formatChangeID(gc.GetHost(), gc.GetChange())] = gc.GetPatchset()
	}
	if len(changeToPatchset) != len(sw.acceptedCLRanges) {
		return false
	}
	for changeID, ps := range changeToPatchset {
		switch psRange, ok := sw.acceptedCLRanges[changeID]; {
		case !ok:
			return false
		case ps < psRange.minIncl:
			return false
		case ps > psRange.maxIncl:
			return false
		}
	}
	return true
}

func hasAdditionalProperties(build *bbpb.Build) bool {
	props := build.GetInfra().GetBuildbucket().GetRequestedProperties().GetFields()
	for key := range props {
		if !AcceptedAdditionalPropKeys.Has(key) {
			return true
		}
	}
	return false
}

func formatChangeID(host string, changeNum int64) string {
	return fmt.Sprintf("%s/%d", host, changeNum)
}

func (sw *searchWorker) toTryjob(ctx context.Context, build *bbpb.Build, def *tryjob.Definition) (*tryjob.Tryjob, error) {
	status, result, err := parseStatusAndResult(ctx, build)
	if err != nil {
		return nil, err
	}
	tj := &tryjob.Tryjob{
		ExternalID:  tryjob.MustBuildbucketID(sw.bbHost, build.Id),
		Definition:  def,
		Status:      status,
		Result:      result,
		CLPatchsets: make(tryjob.CLPatchsets, len(build.GetInput().GetGerritChanges())),
	}
	for i, gc := range build.GetInput().GetGerritChanges() {
		tj.CLPatchsets[i] = tryjob.MakeCLPatchset(sw.changeIDToCLID[formatChangeID(gc.GetHost(), gc.GetChange())], int32(gc.GetPatchset()))
	}
	sort.Sort(tj.CLPatchsets)
	return tj, nil
}
