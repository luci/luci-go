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
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// Margin applied to a timeout duration from build to policy.
// To avoid Buildbucket and TurboCI colliding when they both try to handle
// a timeout event.
const buildToPolicyTimeoutMargin = 30 * time.Second

func buildToPolicyTimeout(d *durationpb.Duration) *durationpb.Duration {
	if d == nil {
		return nil
	}
	return durationpb.New(d.AsDuration() + buildToPolicyTimeoutMargin)
}

// policyToBuildTimeout calculates a timeout that should be given to a build
// based on the corresponding timeout in execution policy.
//
// If the policy timeout is smaller than buildToPolicyTimeoutMargin,
// return nil so that Buildbucket could apply configured/default timeout to
// the build instead.
func policyToBuildTimeout(d *durationpb.Duration) *durationpb.Duration {
	if d == nil {
		return nil
	}

	if d.AsDuration() < buildToPolicyTimeoutMargin {
		return nil
	}

	return durationpb.New(d.AsDuration() - buildToPolicyTimeoutMargin)
}

// fillScheduleBuildRequestWithPolicy updates req in place to fill its timeout
// fields with policy.
func fillScheduleBuildRequestWithPolicy(req *pb.ScheduleBuildRequest, policyTimeouts *orchestratorpb.StageAttemptExecutionPolicy_Timeout) {
	req.ExecutionTimeout = policyToBuildTimeout(policyTimeouts.GetRunning())
	req.SchedulingTimeout = policyToBuildTimeout(policyTimeouts.GetScheduled())
	req.GracePeriod = policyToBuildTimeout(policyTimeouts.GetTearingDown())
}

func scheduleBuildRequestToPolicyTimeout(req *pb.ScheduleBuildRequest) *orchestratorpb.StageAttemptExecutionPolicy_Timeout {
	b := orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
		Scheduled:   buildToPolicyTimeout(req.GetSchedulingTimeout()),
		Running:     buildToPolicyTimeout(req.GetExecutionTimeout()),
		TearingDown: buildToPolicyTimeout(req.GetGracePeriod()),
	}
	if b.Scheduled == nil && b.Running == nil && b.TearingDown == nil {
		return nil
	}
	return b.Build()
}

func buildToStageExecutionPolicy(bld *pb.Build, requested *orchestratorpb.StageExecutionPolicy) *orchestratorpb.StageExecutionPolicy {
	updated := orchestratorpb.StageExecutionPolicy_builder{
		Retry:                          requested.GetRetry(),
		AttemptExecutionPolicyTemplate: buildToStagetAttemptExecutionPolicy(bld),
	}.Build()

	if requested.HasStageTimeoutMode() {
		updated.SetStageTimeoutMode(updated.GetStageTimeoutMode())
	}

	perAttemptPolicy := updated.GetAttemptExecutionPolicyTemplate().GetTimeout()
	perAttemptTotal := perAttemptPolicy.GetScheduled().AsDuration() + perAttemptPolicy.GetRunning().AsDuration() + perAttemptPolicy.GetTearingDown().AsDuration()
	retry := updated.GetRetry().GetMaxRetries()
	if retry == 0 {
		retry = 1
	}
	stageDuration := perAttemptTotal * time.Duration(retry)
	updated.SetStageTimeout(durationpb.New(stageDuration))

	return updated
}

func buildToStagetAttemptExecutionPolicy(bld *pb.Build) *orchestratorpb.StageAttemptExecutionPolicy {
	return orchestratorpb.StageAttemptExecutionPolicy_builder{
		// Buildbucket will handle heartbeats by itself, it will not send
		// heartbeats to TurboCI.
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			Scheduled: buildToPolicyTimeout(bld.GetSchedulingTimeout()),
			Running:   buildToPolicyTimeout(bld.GetExecutionTimeout()),
			TearingDown: buildToPolicyTimeout(durationpb.New(
				bld.GetGracePeriod().AsDuration() + buildbucket.MinUpdateBuildInterval)),
		}.Build(),
	}.Build()
}
