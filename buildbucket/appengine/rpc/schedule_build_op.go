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
	"fmt"
	"iter"
	"maps"
	"slices"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// scheduleBuildOp is a batch of ScheduleBuildRequest processed in parallel.
//
// It carries the request state (being mutated during its processing) as well as
// a cache of entities used by different parts of the request processing.
//
// Construct it with newScheduleBuildOp.
type scheduleBuildOp struct {
	// Reqs are the requests being processed.
	Reqs []*pb.ScheduleBuildRequest
	// DryRun is true if this batch will not actually be submitted.
	DryRun bool

	// Errs accumulates per-request errors.
	Errs errors.MultiError
	// Builds are the created builds (may be incomplete if DryRun is true).
	Builds []*model.Build
	// Masks are masks to apply to the returned build protos.
	Masks []*model.BuildMask

	// Parents describes parent builds mentioned by the batch.
	Parents *parentsMap
	// Buckets are buckets containing builders requested by the batch.
	Buckets *bucketsMap
	// GlobalCfg is the global config used to handle this batch.
	GlobalCfg *pb.SettingsCfg
	// WellKnownExperiments are known 'global' experiments from the GlobalCfg.
	WellKnownExperiments stringset.Set

	// Builders are builder configs prefetched by PrefetchBuilders.
	Builders map[string]*pb.BuilderConfig

	// reqMap maps *pb.ScheduleBuildRequest to its index in Reqs.
	reqMap map[*pb.ScheduleBuildRequest]int
}

// newScheduleBuildOp initializes a scheduleBuildOp for a batch of requests.
//
// It prepares it to be used for detailed validation and eventual submission of
// the given requests by prefetching their parent builds, bucket configs and
// global service configs.
//
// It doesn't yet validate requests themselves.
//
// If parent is not nil, it will be used as parent of all requests in the batch
// regardless of parent_build_id or the build token.
//
// An error here means the entire batch has failed. It is always an appstatus
// error.
func newScheduleBuildOp(ctx context.Context, reqs []*pb.ScheduleBuildRequest, parent *model.Build) (*scheduleBuildOp, error) {
	if len(reqs) == 0 {
		return nil, appstatus.BadRequest(errors.New("at least one ScheduleBuildRequest is required"))
	}

	dryRun := reqs[0].DryRun
	for _, req := range reqs {
		if req.DryRun != dryRun {
			return nil, appstatus.BadRequest(errors.New("all requests must have the same dry_run value"))
		}
	}

	// Validate parents and fetch parent builds.
	pIDSet := make(map[int64]struct{})
	for _, req := range reqs {
		if req.ParentBuildId != 0 {
			pIDSet[req.ParentBuildId] = struct{}{}
		}
	}
	parents, err := validateParents(ctx, slices.Collect(maps.Keys(pIDSet)), parent)
	if err != nil {
		return nil, err
	}

	// Prefetch bucket configs to check shadowing settings.
	buckets, err := prefetchBuckets(ctx, reqs)
	if err != nil {
		return nil, err
	}

	// Need the config to lookup experiments to validate them in the request.
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "error fetching service config: %s", err)
	}

	op := &scheduleBuildOp{
		Reqs:                 reqs,
		DryRun:               dryRun,
		Errs:                 make(errors.MultiError, len(reqs)),
		Masks:                make([]*model.BuildMask, len(reqs)),
		Builds:               make([]*model.Build, len(reqs)),
		Parents:              parents,
		Buckets:              buckets,
		GlobalCfg:            globalCfg,
		WellKnownExperiments: protoutil.WellKnownExperiments(globalCfg),
	}
	op.UpdateReqMap()
	return op, nil
}

// reqIdx returns the index of *pb.ScheduleBuildRequest in Reqs or panic.
func (op *scheduleBuildOp) reqIdx(req *pb.ScheduleBuildRequest) int {
	idx, ok := op.reqMap[req]
	if !ok {
		panic("scheduleBuildOp is given an unknown *ScheduleBuildRequest")
	}
	return idx
}

// UpdateReqMap updates mapping from *pb.ScheduleBuildRequest to its index.
//
// Must be called in case Reqs is updated.
func (op *scheduleBuildOp) UpdateReqMap() {
	op.reqMap = make(map[*pb.ScheduleBuildRequest]int, len(op.Reqs))
	for idx, req := range op.Reqs {
		if req == nil {
			panic("scheduleBuildOp is given nil *ScheduleBuildRequest")
		}
		if _, exists := op.reqMap[req]; exists {
			panic(fmt.Sprintf("scheduleBuildOp is given identical *ScheduleBuildRequest: %p", req))
		}
		op.reqMap[req] = idx
	}
}

// Pending iterates over unresolved (not yet failed) requests.
func (op *scheduleBuildOp) Pending() iter.Seq2[int, *pb.ScheduleBuildRequest] {
	return func(yield func(int, *pb.ScheduleBuildRequest) bool) {
		idx := 0
		for i, req := range op.Reqs {
			if op.Errs[i] == nil {
				if !yield(idx, req) {
					return
				}
				idx++
			}
		}
	}
}

// Fail marks a single request as failed.
func (op *scheduleBuildOp) Fail(req *pb.ScheduleBuildRequest, err error) {
	idx := op.reqIdx(req)
	op.Errs[idx] = err
	op.Builds[idx] = nil
}

// SetBuild associates a pending request with a build entity it produced.
func (op *scheduleBuildOp) SetBuild(req *pb.ScheduleBuildRequest, build *model.Build) {
	idx := op.reqIdx(req)
	if op.Errs[idx] != nil {
		panic(fmt.Sprintf("request #%d was already marked as failed", idx))
	}
	op.Builds[idx] = build
}

// BuildForRequest returns a build associated with a pending request.
func (op *scheduleBuildOp) BuildForRequest(req *pb.ScheduleBuildRequest) *model.Build {
	idx := op.reqIdx(req)
	if op.Errs[idx] != nil {
		panic(fmt.Sprintf("request #%d was already marked as failed", idx))
	}
	return op.Builds[idx]
}

// PrefetchBuilders fetches builder configs of all still pending requests.
//
// Fetched configs are placed into Builders map. Assumes bucket configs have
// been prefetched already.
//
// Errors are put into Errs, e.g. requests that point to non-existing builders
// are marked as failed with "no such builder" error. If the prefetch operation
// fails as a whole, then all pending requests are marked as failed with an
// internal appstatus error.
func (op *scheduleBuildOp) PrefetchBuilders(ctx context.Context) {
	type builderEntry struct {
		cfg *pb.BuilderConfig
		err error
	}
	builderByID := map[string]*builderEntry{}
	var buildersToFetch []*model.Builder

	for _, req := range op.Pending() {
		if err := protoutil.ValidateRequiredBuilderID(req.GetBuilder()); err != nil {
			op.Fail(req, appstatus.BadRequest(err))
			continue
		}

		// Check if we already processed this builder.
		builderID := protoutil.FormatBuilderID(req.Builder)
		if _, ok := builderByID[builderID]; ok {
			continue
		}

		// Check if the bucket exists.
		bucketID := protoutil.FormatBucketID(req.Builder.Project, req.Builder.Bucket)
		bucket := op.Buckets.Bucket(bucketID)
		if bucket == nil {
			builderByID[builderID] = &builderEntry{
				err: appstatus.Errorf(codes.NotFound, "bucket not found: %q", bucketID),
			}
			continue
		}

		// Check if this is a bucket with regular (not dynamic) builders. This is
		// indicated by presence of Swarming field. Regular builders must exist
		// in the datastore and we'll fetch them. Builders in dynamic buckets do not
		// physically exist in the datastore, but still are considered valid. We'll
		// just silently skip fetching them.
		builderByID[builderID] = &builderEntry{}
		if bucket.Swarming != nil {
			buildersToFetch = append(buildersToFetch, &model.Builder{
				Parent: model.BucketKey(ctx, req.Builder.Project, req.Builder.Bucket),
				ID:     req.Builder.Builder,
			})
		}
	}

	// Fetch all builders at once, fill in builderByID with the results.
	err := datastore.Get(ctx, buildersToFetch)
	builderErr := func(idx int) error {
		if merr, ok := err.(errors.MultiError); ok {
			return merr[idx]
		} else {
			return err
		}
	}
	for idx, builder := range buildersToFetch {
		entry := builderByID[protoutil.ToBuilderIDString(
			builder.Parent.Parent().StringID(), // project
			builder.Parent.StringID(),          // bucket
			builder.ID,                         // builder
		)]
		switch err := builderErr(idx); {
		case err == nil && builder.Config != nil:
			entry.cfg = builder.Config
		case err == nil && builder.Config == nil:
			entry.err = appstatus.Errorf(codes.Internal, "failed to fetch builder %q config: no proto config", builder.ID)
		case errors.Is(err, datastore.ErrNoSuchEntity):
			entry.err = appstatus.Errorf(codes.NotFound, "builder not found: %q", builder.ID)
		default:
			entry.err = appstatus.Errorf(codes.Internal, "failed to fetch builder %q config: %s", builder.ID, err)
		}
	}

	// Resolve requests that failed due to builder fetch errors.
	for _, req := range op.Pending() {
		builderID := protoutil.FormatBuilderID(req.Builder)
		if err := builderByID[builderID].err; err != nil {
			op.Fail(req, err)
		}
	}

	// Collect fetched non-dynamic builders into the final Builders map.
	builders := map[string]*pb.BuilderConfig{}
	for builderID, entry := range builderByID {
		if entry.cfg != nil {
			builders[builderID] = entry.cfg
		}
	}
	op.Builders = builders
}

// Finalize converts build entities to protos, applying the mask.
//
// May do datastore fetches to fetch build details if they are requested by
// the mask.
//
// Returns the final result of this batch operation.
func (op *scheduleBuildOp) Finalize(ctx context.Context) ([]*pb.Build, errors.MultiError) {
	out := make([]*pb.Build, len(op.Reqs))

	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(64)
	for i, build := range op.Builds {
		if op.Errs[i] != nil {
			continue
		}
		if build == nil {
			op.Errs[i] = appstatus.Errorf(codes.Internal, "build #%d/%d was left unprocessed, this should not be possible", i, len(op.Builds))
			continue
		}
		eg.Go(func() error {
			// Note: We don't redact the Build response here because we expect any
			// user with BuildsAdd permission should also have BuildsGet.
			//
			// TODO(crbug/1042991): Don't re-read freshly written entities
			// (see ToProto).
			if op.DryRun {
				out[i], op.Errs[i] = build.ToDryRunProto(ctx, op.Masks[i])
			} else {
				out[i], op.Errs[i] = build.ToProto(ctx, op.Masks[i], nil)
			}
			return nil
		})
	}
	_ = eg.Wait()

	return out, op.Errs
}
