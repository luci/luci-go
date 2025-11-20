// Copyright 2025 The LUCI Authors.
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

package rpc

import (
	"context"
	"maps"
	"slices"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/resultdb"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// batchToLaunchKind specifies a strategy of how to launch batches of builds.
type batchToLaunchKind int

const (
	// batchKindNative is a batch of builds (perhaps under different parents) to
	// launch as native Buildbucket builds.
	batchKindNative batchToLaunchKind = 0

	// batchKindTurboCIRoot is a batch with exactly one parent-less build to
	// launch in a new Turbo CI workplan.
	batchKindTurboCIRoot batchToLaunchKind = 1

	// batchKindTurboCIChildren is a batch of child Turbo CI builds under the
	// same Turbo CI parent build to launch in the existing parent's workplan.
	batchKindTurboCIChildren batchToLaunchKind = 2
)

// batchToLaunch defines a batch of requests to launch and a strategy of how
// to launch them.
type batchToLaunch struct {
	// The kind of the batch.
	kind batchToLaunchKind
	// The requests in the batch (at least one).
	reqs []*pb.ScheduleBuildRequest
	// The matching Builds being launched. Always has the same length as `reqs`.
	builds []*model.Build
	// The parent build for batchKindTurboCIChildren, ignored otherwise.
	parent *model.Build
}

// launch launches all builds in the batch.
//
// Updates *model.Build in-place on success. Returns exactly len(b.reqs) number
// of errors.
func (b *batchToLaunch) launch(ctx context.Context, bldrsMCB stringset.Set, orch orchestratorgrpcpb.TurboCIOrchestratorClient) errors.MultiError {
	switch b.kind {
	case batchKindNative:
		return launchNative(ctx, b.reqs, b.builds, bldrsMCB)

	case batchKindTurboCIRoot:
		err := launchTurboCIRoot(ctx, b.reqs[0], b.builds[0], orch)
		return errors.MultiError{err}

	case batchKindTurboCIChildren:
		return launchTurboCIChildren(ctx, b.parent, b.reqs, b.builds, orch)

	default:
		panic("impossible")
	}
}

// splitByLaunchStrategy groups pending build requests by how to launch them.
//
// This is just a local calculation that doesn't fetch anything (everything it
// needs should already be prefetched). It thus can't fail and returns no
// errors.
func splitByLaunchStrategy(op *scheduleBuildOp) []*batchToLaunch {
	// TODO(b/454194663): Launching child Turbo CI builds is not actually
	// supported yet, so for now demote such builds to be launched natively
	// instead. This is a temporary filter.
	return demoteTurboCIChildrenToNative(splitByLaunchStrategyImpl(op))
}

// demoteTurboCIChildrenToNative moves builds from batchKindTurboCIChildren
// batches into batchKindNative batch.
func demoteTurboCIChildrenToNative(batches []*batchToLaunch) []*batchToLaunch {
	// Find the batch with native builds or instantiate it.
	var native *batchToLaunch
	for _, b := range batches {
		if b.kind == batchKindNative {
			native = b
			break
		}
	}
	if native == nil {
		native = &batchToLaunch{kind: batchKindNative}
	}

	// Move requests from Turbo CI child batches into the native batch.
	for _, b := range batches {
		if b.kind == batchKindTurboCIChildren {
			native.reqs = append(native.reqs, b.reqs...)
			native.builds = append(native.builds, b.builds...)
		}
	}

	// The filtered list of batches is just the native batch (if any) and all
	// batchKindTurboCIRoot batches.
	out := make([]*batchToLaunch, 0, len(batches))
	if len(native.reqs) != 0 {
		out = append(out, native)
	}
	for _, b := range batches {
		if b.kind == batchKindTurboCIRoot {
			out = append(out, b)
		}
	}
	return out
}

// splitByLaunchStrategyImpl is the actual implementation.
//
// This indirection exists temporary and only for testing. Will be removed once
// demoteTurboCIChildrenToNative is gone.
func splitByLaunchStrategyImpl(op *scheduleBuildOp) []*batchToLaunch {
	var batches []*batchToLaunch
	var native *batchToLaunch
	var perParent map[*model.Build]*batchToLaunch

	launchAsNative := func(req *pb.ScheduleBuildRequest, build *model.Build) {
		if native == nil {
			native = &batchToLaunch{kind: batchKindNative}
		}
		native.reqs = append(native.reqs, req)
		native.builds = append(native.builds, build)
	}

	launchAsTurboCIRoot := func(req *pb.ScheduleBuildRequest, build *model.Build) {
		batches = append(batches, &batchToLaunch{
			kind:   batchKindTurboCIRoot,
			reqs:   []*pb.ScheduleBuildRequest{req},
			builds: []*model.Build{build},
		})
	}

	launchAsTurboCIChild := func(parent *model.Build, req *pb.ScheduleBuildRequest, build *model.Build) {
		batch, _ := perParent[parent]
		if batch == nil {
			if perParent == nil {
				perParent = map[*model.Build]*batchToLaunch{}
			}
			batch = &batchToLaunch{
				kind:   batchKindTurboCIChildren,
				parent: parent,
			}
			perParent[parent] = batch
		}
		batch.reqs = append(batch.reqs, req)
		batch.builds = append(batch.builds, build)
	}

	for _, req := range op.Pending() {
		build := op.BuildForRequest(req)
		parent, _ := op.Parents.parentBuildForRequest(req)
		switch {
		case !hasTurboCIExperiment(build):
			// Builds without the experiment should always be launched natively
			// regardless if they have a parent or not.
			launchAsNative(req, build)
		case parent == nil:
			// Root Turbo CI builds each are launched in their own work plan. Each
			// root build thus needs to be in its own *batchToLaunch to be launched
			// independently of other builds.
			launchAsTurboCIRoot(req, build)
		case parent.StageAttemptID == "":
			// Never launch a child build via Turbo CI if its parent is not in
			// Turbo CI. We don't want multiple child builds of the same parent to be
			// in different workplans, this will just be confusing. Better no workplan
			// at all.
			launchAsNative(req, build)
		default:
			// Group child Turbo CI builds by their parent Turbo CI build. Such
			// child builds can all be launched via a single batch WriteNodes call
			// that adds them to parent's workplan.
			launchAsTurboCIChild(parent, req, build)
		}
	}

	// Assemble the final list of non-empty batches.
	batches = slices.AppendSeq(batches, maps.Values(perParent))
	if native != nil {
		batches = append(batches, native)
	}
	return batches
}

// hasTurboCIExperiment checks if the experiment is set in the build.
func hasTurboCIExperiment(b *model.Build) bool {
	return slices.Contains(b.Proto.GetInput().GetExperiments(), buildbucket.ExperimentRunInTurboCI)
}

// nativeBatchToLaunch unconditionally puts all builds into a native batch.
func nativeBatchToLaunch(op *scheduleBuildOp) *batchToLaunch {
	batch := &batchToLaunch{kind: batchKindNative}
	for _, req := range op.Pending() {
		batch.reqs = append(batch.reqs, req)
		batch.builds = append(batch.builds, op.BuildForRequest(req))
	}
	return batch
}

// launchNative launches a batch of builds via native Buildbucket queues.
//
// Updates `builds` in-place, returns exactly len(builds) appstatus errors.
func launchNative(ctx context.Context, reqs []*pb.ScheduleBuildRequest, builds []*model.Build, bldrsMCB stringset.Set) errors.MultiError {
	var buildsToCreate []*buildToCreate
	for idx, req := range reqs {
		buildsToCreate = append(buildsToCreate, &buildToCreate{
			build:     builds[idx],
			requestID: req.RequestId,
			resultDB: resultdb.CreateOptions{
				// Build is an export root in ResultDB if it has no parent, or if
				// explicitly requested.
				IsExportRoot: len(builds[idx].AncestorIds) == 0 || req.GetResultdb().GetIsExportRootOverride(),
			},
		})
	}
	return createBuilds(ctx, buildsToCreate, bldrsMCB)
}

// launchTurboCIRoot launches a root build via a new Turbo CI workplan.
//
// Updates `build` in-place. Returns appstatus errors.
func launchTurboCIRoot(ctx context.Context, req *pb.ScheduleBuildRequest, build *model.Build, orch orchestratorgrpcpb.TurboCIOrchestratorClient) error {
	logging.Infof(ctx, "TurboCI: launching new workplan with %s", protoutil.FormatBuilderID(req.Builder))
	return appstatus.Errorf(codes.Unimplemented, "Turbo CI builds are not implemented yet")
}

// launchTurboCIChildren launches child builds by adding them to an existing
// workplan.
//
// Updates `builds` in-place, returns exactly len(builds) appstatus errors.
func launchTurboCIChildren(ctx context.Context, parent *model.Build, reqs []*pb.ScheduleBuildRequest, builds []*model.Build, orch orchestratorgrpcpb.TurboCIOrchestratorClient) errors.MultiError {
	return slices.Repeat(errors.MultiError{
		appstatus.Errorf(codes.Unimplemented, "Turbo CI child builds are not implemented yet"),
	}, len(reqs))
}
