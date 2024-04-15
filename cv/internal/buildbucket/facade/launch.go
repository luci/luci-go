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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/buildbucket"
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
// Updates the Tryjobs that are scheduled successfully in Buildbucket in place.
// The following fields will be updated:
//   - ExternalID
//   - Status
//   - Result
//
// Returns nil if all tryjobs have been successfully launched. Otherwise,
// returns `errors.MultiError` where each element is the launch error of
// the corresponding Tryjob.
//
// Uses Tryjob ID as the request key for deduplication. This ensures only one
// Buildbucket build will be scheduled for one Tryjob within the deduplication
// window (currently 1 min. in Buildbucket).
func (f *Facade) Launch(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) error {
	tryjobsByHost := splitTryjobsByHost(tryjobs)
	tryjobToIndex := make(map[*tryjob.Tryjob]int, len(tryjobs))
	for i, tj := range tryjobs {
		tryjobToIndex[tj] = i
	}
	launchErrs := errors.NewLazyMultiError(len(tryjobs))
	poolErr := parallel.WorkPool(min(len(tryjobsByHost), 8), func(work chan<- func() error) {
		for host, tryjobs := range tryjobsByHost {
			host, tryjobs := host, tryjobs
			work <- func() error {
				err := f.schedule(ctx, host, r, cls, tryjobs)
				var merrs errors.MultiError
				switch ok := errors.As(err, &merrs); {
				case err == nil:
				case !ok:
					// assign singular error to all tryjobs.
					for _, tj := range tryjobs {
						launchErrs.Assign(tryjobToIndex[tj], err)
					}
				default:
					for i, tj := range tryjobs {
						launchErrs.Assign(tryjobToIndex[tj], merrs[i])
					}
				}
				return nil
			}
		}
	})
	if poolErr != nil {
		panic(fmt.Errorf("impossible"))
	}
	return launchErrs.Get()
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

func (f *Facade) schedule(ctx context.Context, host string, r *run.Run, cls []*run.RunCL, tryjobs []*tryjob.Tryjob) error {
	bbClient, err := f.ClientFactory.MakeClient(ctx, host, r.ID.LUCIProject())
	if err != nil {
		return errors.Annotate(err, "failed to create Buildbucket client").Err()
	}
	batches, err := prepareBatches(ctx, tryjobs, r, cls)
	if err != nil {
		return errors.Annotate(err, "failed to create batch schedule build requests").Err()
	}

	scheduleErrs := errors.NewLazyMultiError(len(tryjobs))
	var wg sync.WaitGroup
	for _, batch := range batches {
		batch := batch
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := makeRequestAndUpdateTryjobs(ctx, bbClient, batch, host)
			var merrs errors.MultiError
			switch ok := errors.As(err, &merrs); {
			case err == nil:
			case !ok:
				// assign singular error to all tryjobs.
				for i := 0; i < len(batch.tryjobs); i++ {
					scheduleErrs.Assign(batch.offset+i, err)
				}
			default:
				for i, merr := range merrs {
					scheduleErrs.Assign(batch.offset+i, merr)
				}
			}
		}()
	}
	wg.Wait()
	return scheduleErrs.Get()
}

func makeRequestAndUpdateTryjobs(ctx context.Context, bbClient buildbucket.Client, batch batch, host string) error {
	logging.Debugf(ctx, "scheduling a batch of %d builds against buildbucket", len(batch.req.GetRequests()))
	res, err := bbClient.Batch(ctx, batch.req)
	if err != nil {
		return err
	}
	ret := errors.NewLazyMultiError(len(res.GetResponses()))
	for i, res := range res.GetResponses() {
		switch res.GetResponse().(type) {
		case *bbpb.BatchResponse_Response_ScheduleBuild:
			build := res.GetScheduleBuild()
			status, result, err := parseStatusAndResult(ctx, build)
			if err != nil {
				ret.Assign(i, err)
			}
			tj := batch.tryjobs[i]
			tj.ExternalID = tryjob.MustBuildbucketID(host, build.Id)
			tj.Status = status
			tj.Result = result
		case *bbpb.BatchResponse_Response_Error:
			ret.Assign(i, status.ErrorProto(res.GetError()))
		case nil:
			logging.Errorf(ctx, "FIXME(crbug/1359509): received nil response at index %d for batch request:\n%s", i, batch.req)
			ret.Assign(i, transient.Tag.Apply(errors.New("unexpected nil response from Buildbucket")))
		default:
			panic(fmt.Errorf("unexpected response type: %T", res.GetResponse()))
		}
	}
	return ret.Get()
}

// limit comes from buildbucket:
// https://pkg.go.dev/go.chromium.org/luci/buildbucket/proto#BatchRequest
const maxBatchSize = 200

// batch represents a batch request that contains smaller than $maxBatchSize
// of tryjobs
type batch struct {
	req     *bbpb.BatchRequest
	tryjobs []*tryjob.Tryjob // the tryjobs to launch in batch request
	offset  int              // offset of the tryjobs in the overall tryjob slice
}

func prepareBatches(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) ([]batch, error) {
	gcs := makeGerritChanges(cls)
	nonOptProp, optProp, err := makeProperties(ctx, r.Mode, r.Owner)
	if err != nil {
		return nil, errors.Annotate(err, "failed to make input properties").Err()
	}
	nonOptTags, optTags, err := makeTags(r, cls)
	if err != nil {
		return nil, errors.Annotate(err, "failed to make tags").Err()
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
			offset:  offset,
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
