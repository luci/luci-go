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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/rpc/versioning"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func makeTryjobInvocations(ctx context.Context, r *run.Run) ([]*apiv0pb.TryjobInvocation, error) {
	if r.Tryjobs.GetState() == nil {
		return nil, nil // early return if there is no tryjob
	}
	// TODO(yiwzhang): store the config group ID in the
	// r.Tryjobs.State.Requirement. Using r.ConfigGroupID has a slight chance
	// causing unexpected behavior. For example:
	//  T1: config is at revision X and builder A is triggered
	//  T2: config gets a new revision X+1 and builder A is removed and the
	//      run is cancelled due to config change.
	//  T3: user loads the cancelled run. r.Tryjobs may contain information about
	//      triggering builder A. However, the config associated with the run
	//      no longer has builder A definition. Causing error in a later stage.
	// Saving the config group ID (i.e. pinned at the revision) that is used to
	// compute the requirement inside the requirement would solve the problem.
	cg, err := prjcfg.GetConfigGroup(ctx, r.ID.LUCIProject(), r.ConfigGroupID)
	if err != nil {
		return nil, appstatus.Attachf(err, codes.Internal, "failed to load config group for the run")
	}
	builderByName := computeBuilderByName(cg)
	ret := make([]*apiv0pb.TryjobInvocation, 0, len(r.Tryjobs.GetState().GetExecutions()))
	for i, execution := range r.Tryjobs.GetState().GetExecutions() {
		ti := &apiv0pb.TryjobInvocation{}
		definition := r.Tryjobs.GetState().GetRequirement().GetDefinitions()[i]
		switch definition.GetBackend().(type) {
		case *tryjob.Definition_Buildbucket_:
			builderName := bbutil.FormatBuilderID(definition.GetBuildbucket().GetBuilder())
			builderConfig, ok := builderByName[builderName]
			if !ok {
				// Error out once the TODO above is implemented
				logging.Warningf(ctx, "builder %q is triggered by LUCI CV but can NOT be found in the config. Skip including the builder in the response", builderName)
				continue
			}
			ti.BuilderConfig = builderConfig
		default:
			return nil, appstatus.Errorf(codes.Unimplemented, "unknown tryjob backend %T", definition.GetBackend())
		}
		ti.Critical = definition.GetCritical()
		if len(execution.GetAttempts()) == 0 {
			logging.Errorf(ctx, "NEEDS INVESTIGATION: tryjob execution has empty attempts. tryjob definition: %s", definition)
			continue
		}
		ti.Attempts = make([]*apiv0pb.TryjobInvocation_Attempt, len(execution.GetAttempts()))
		for i, attempt := range execution.GetAttempts() {
			// attempts in execution are sorted with earliest first and the response
			// need to be sorted with latest first.
			var err error
			ti.Attempts[len(execution.GetAttempts())-1-i], err = makeTryjobAttempt(attempt, definition)
			if err != nil {
				return nil, err
			}
		}
		ti.Status = ti.Attempts[0].GetStatus()
		ret = append(ret, ti)
	}
	return ret, nil
}

func computeBuilderByName(cg *prjcfg.ConfigGroup) map[string]*cfgpb.Verifiers_Tryjob_Builder {
	ret := make(map[string]*cfgpb.Verifiers_Tryjob_Builder, len(cg.Content.GetVerifiers().GetTryjob().GetBuilders()))
	for _, b := range cg.Content.GetVerifiers().GetTryjob().GetBuilders() {
		ret[b.GetName()] = b
		if equiName := b.GetEquivalentTo().GetName(); equiName != "" {
			ret[equiName] = b // map equivalent builder name to this builder too.
		}
	}
	return ret
}

func makeTryjobAttempt(attempt *tryjob.ExecutionState_Execution_Attempt, def *tryjob.Definition) (*apiv0pb.TryjobInvocation_Attempt, error) {
	ret := &apiv0pb.TryjobInvocation_Attempt{}
	if attempt.GetResult() != nil {
		switch attempt.GetResult().GetBackend().(type) {
		case *tryjob.Result_Buildbucket_:
			bbResult := attempt.GetResult().GetBuildbucket()
			ret.Result = &apiv0pb.TryjobResult{
				Backend: &apiv0pb.TryjobResult_Buildbucket_{
					Buildbucket: &apiv0pb.TryjobResult_Buildbucket{
						Host:    def.GetBuildbucket().GetHost(),
						Id:      bbResult.GetId(),
						Builder: bbResult.GetBuilder(),
					},
				},
			}
		default:
			return nil, appstatus.Errorf(codes.Unimplemented, "unknown tryjob backend %T", attempt.GetResult().GetBackend())
		}
	}
	ret.Reuse = attempt.GetReused()
	ret.Status = versioning.TryjobStatusV0(attempt.GetStatus(), attempt.Result.GetStatus())
	return ret, nil
}
