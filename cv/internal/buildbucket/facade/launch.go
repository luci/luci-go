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
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// TODO: crbug/333811087 - remove after cq module is deleted
const legacyPropertyKey = "$recipe_engine/cq"
const propertyKey = "$recipe_engine/cv"

// Launch schedules requested Tryjobs in Buildbucket.
//
// The Tryjobs will include relevant info from the Run (e.g. Run mode) and
// involves all provided CLs.
//
// Return LaunchResults that has the same length as the provided Tryjobs.
// LaunchResult.Err is populated if the Buildbucket returns non-successful
// response or any error occurred even before sending the schedule request to
// buildbucket (e.g. invalid input).
//
// Uses Tryjob ID as the request key for deduplication. This ensures only one
// Buildbucket build will be scheduled for one Tryjob within the deduplication
// window (currently 1 min. in Buildbucket).
func (f *Facade) Launch(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) []*tryjob.LaunchResult {
	resultByTryjobID := make(map[common.TryjobID]*tryjob.LaunchResult, len(tryjobs))
	for _, tj := range tryjobs {
		resultByTryjobID[tj.ID] = &tryjob.LaunchResult{}
	}
	tryjobsByHost := splitTryjobsByHost(tryjobs)

	var wg sync.WaitGroup
	wg.Add(len(tryjobsByHost))
	for host, tryjobs := range tryjobsByHost {
		go func() {
			defer wg.Done()
			f.schedule(ctx, host, r, cls, tryjobs, resultByTryjobID)
		}()
	}
	wg.Wait()

	results := make([]*tryjob.LaunchResult, len(tryjobs))
	for i, tj := range tryjobs {
		results[i] = resultByTryjobID[tj.ID]
	}
	return results
}

func splitTryjobsByHost(tryjobs []*tryjob.Tryjob) map[string][]*tryjob.Tryjob {
	ret := make(map[string][]*tryjob.Tryjob, 2) // normally, at most 2 host.
	for _, tj := range tryjobs {
		bbDef := tj.Definition.GetBuildbucket()
		if bbDef == nil {
			panic(fmt.Errorf("launch non-Buildbucket Tryjob (%T) with Buildbucket backend", tj.Definition.GetBackend()))
		}
		ret[bbDef.GetHost()] = append(ret[bbDef.GetHost()], tj)
	}
	return ret
}

func (f *Facade) schedule(ctx context.Context, host string, r *run.Run, cls []*run.RunCL, tryjobs []*tryjob.Tryjob, results map[common.TryjobID]*tryjob.LaunchResult) {
	bbClient, err := f.ClientFactory.MakeClient(ctx, host, r.ID.LUCIProject())
	if err != nil {
		// assign the error to the launchResult.Err for each Tryjob
		for _, tj := range tryjobs {
			results[tj.ID].Err = fmt.Errorf("failed to create Buildbucket client: %w", err)
		}
		return
	}
	batches, err := prepareBatches(ctx, tryjobs, r, cls)
	if err != nil {
		// error could only occur when computing input parameters for all Tryjobs.
		// Hence, assign the err to the launchResult.Err for each Tryjob.
		for _, tj := range tryjobs {
			results[tj.ID].Err = fmt.Errorf("failed to create batch schedule build requests: %w", err)
		}
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(batches))
	for _, batch := range batches {
		go func() {
			defer wg.Done()
			makeBatchRequest(ctx, bbClient, batch, host, results)
		}()
	}
	wg.Wait()
}

func makeBatchRequest(ctx context.Context, bbClient buildbucket.Client, batch batch, host string, results map[common.TryjobID]*tryjob.LaunchResult) {
	logging.Debugf(ctx, "scheduling a batch of %d builds against buildbucket", len(batch.req.GetRequests()))
	res, err := bbClient.Batch(ctx, batch.req)
	if err != nil {
		for _, tj := range batch.tryjobs {
			results[tj.ID].Err = err
		}
		return
	}
	for i, res := range res.GetResponses() {
		launchResult := results[batch.tryjobs[i].ID]
		switch res.GetResponse().(type) {
		case *bbpb.BatchResponse_Response_ScheduleBuild:
			build := res.GetScheduleBuild()
			status, result, err := parseStatusAndResult(ctx, build)
			if err != nil {
				launchResult.Err = err
			} else {
				launchResult.ExternalID = tryjob.MustBuildbucketID(host, build.Id)
				launchResult.Status = status
				launchResult.Result = result
			}
		case *bbpb.BatchResponse_Response_Error:
			launchResult.Err = status.ErrorProto(res.GetError())
		default:
			launchResult.Err = fmt.Errorf("unexpected response type: %T", res.GetResponse())
		}
	}
}

// limit comes from buildbucket:
// https://pkg.go.dev/go.chromium.org/luci/buildbucket/proto#BatchRequest
const maxBatchSize = 200

// batch represents a batch request that contains smaller than $maxBatchSize
// of tryjobs
type batch struct {
	req     *bbpb.BatchRequest
	tryjobs []*tryjob.Tryjob // the tryjobs to launch in batch request
}

func prepareBatches(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) ([]batch, error) {
	gcs := makeGerritChanges(cls)
	nonOptProp, optProp, err := makeProperties(ctx, r.Mode, r.Owner)
	if err != nil {
		return nil, errors.Fmt("failed to make input properties: %w", err)
	}
	nonOptTags, optTags, err := makeTags(r, cls)
	if err != nil {
		return nil, errors.Fmt("failed to make tags: %w", err)
	}
	requests := make([]*bbpb.BatchRequest_Request, len(tryjobs))
	for i, tj := range tryjobs {
		def := tj.Definition
		req := &bbpb.ScheduleBuildRequest{
			RequestId:     strconv.Itoa(int(tj.ID)),
			Builder:       def.GetBuildbucket().GetBuilder(),
			Properties:    nonOptProp,
			GerritChanges: gcs,
			Tags:          nonOptTags,
			Mask:          defaultMask,
		}
		if def.GetOptional() {
			req.Properties = optProp
			req.Tags = optTags
		}
		if experiments := tj.Definition.GetExperiments(); len(experiments) > 0 {
			req.Experiments = make(map[string]bool, len(experiments))
			for _, exp := range experiments {
				req.Experiments[exp] = true
			}
		}
		requests[i] = &bbpb.BatchRequest_Request{
			Request: &bbpb.BatchRequest_Request_ScheduleBuild{
				ScheduleBuild: req,
			},
		}
	}

	// creating $numBatch of batches with similar number of requests in each
	// batch.
	numBatch := (len(requests)-1)/maxBatchSize + 1
	batchSize := len(requests) / numBatch
	ret := make([]batch, 0, numBatch)
	for offset := 0; offset < len(requests); {
		req := &bbpb.BatchRequest{
			Requests: requests[offset:min(offset+batchSize, len(requests))],
		}
		ret = append(ret, batch{
			req:     req,
			tryjobs: tryjobs[offset:min(offset+batchSize, len(requests))],
		})
		offset += len(req.GetRequests())
	}
	return ret, nil
}

func makeProperties(ctx context.Context, mode run.Mode, owner identity.Identity) (nonOpt, opt *structpb.Struct, err error) {
	isGoogler, err := auth.GetState(ctx).DB().IsMember(ctx, owner, []string{"googlers"})
	if err != nil {
		return nil, nil, err
	}
	in := &recipe.Input{
		Active:         true,
		DryRun:         mode == run.DryRun,
		RunMode:        string(mode),
		TopLevel:       true,
		OwnerIsGoogler: isGoogler,
	}
	if nonOpt, err = makeCVProperties(in); err != nil {
		return nil, nil, err
	}
	in.Experimental = true
	if opt, err = makeCVProperties(in); err != nil {
		return nil, nil, err
	}
	return nonOpt, opt, nil
}

func makeCVProperties(in *recipe.Input) (*structpb.Struct, error) {
	b, err := protojson.Marshal(in)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	return structpb.NewStruct(map[string]any{
		propertyKey:       raw,
		legacyPropertyKey: raw,
	})
}

func makeGerritChanges(cls []*run.RunCL) []*bbpb.GerritChange {
	ret := make([]*bbpb.GerritChange, len(cls))
	for i, cl := range cls {
		g := cl.Detail.GetGerrit()
		if g == nil {
			panic(fmt.Errorf("change backend (%T) is not supported", cl.Detail.GetKind()))
		}
		ret[i] = &bbpb.GerritChange{
			Host:     g.GetHost(),
			Project:  g.GetInfo().GetProject(),
			Change:   g.GetInfo().GetNumber(),
			Patchset: int64(cl.Detail.GetPatchset()),
		}
	}
	return ret
}

func makeTags(r *run.Run, cls []*run.RunCL) (nonOpt, opt []*bbpb.StringPair, err error) {
	var commonTags []*bbpb.StringPair
	addTag := func(key string, values ...string) {
		for _, v := range values {
			commonTags = append(commonTags, &bbpb.StringPair{Key: key, Value: strings.TrimSpace(v)})
		}
	}
	addTag("user_agent", "cq")
	addTag("cq_attempt_key", r.ID.AttemptKey())
	addTag("cq_cl_group_key", run.ComputeCLGroupKey(cls, false))
	addTag("cq_equivalent_cl_group_key", run.ComputeCLGroupKey(cls, true))
	owners := stringset.New(1) // normally 1 owner 1 triggerer
	triggerers := stringset.New(1)
	for _, cl := range cls {
		ownerID, err := cl.Detail.OwnerIdentity()
		if err != nil {
			return nil, nil, err
		}
		owners.Add(ownerID.Email())
		triggerers.Add(cl.Trigger.GetEmail())
	}
	addTag("cq_cl_owner", owners.ToSlice()...)
	addTag("cq_triggerer", triggerers.ToSlice()...)
	addTag("cq_cl_tag", r.Options.GetCustomTryjobTags()...)
	nonOpt = append([]*bbpb.StringPair{{Key: "cq_experimental", Value: "false"}}, commonTags...)
	opt = append([]*bbpb.StringPair{{Key: "cq_experimental", Value: "true"}}, commonTags...)
	sortTags(nonOpt)
	sortTags(opt)
	return nonOpt, opt, nil
}

func sortTags(tags []*bbpb.StringPair) {
	slices.SortFunc(tags, func(a, b *bbpb.StringPair) int {
		if res := strings.Compare(a.GetKey(), b.GetKey()); res != 0 {
			return res
		}
		return strings.Compare(a.GetValue(), b.GetValue())
	})
}
