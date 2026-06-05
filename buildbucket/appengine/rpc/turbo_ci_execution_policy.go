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

const (
	// A difference between Turbo CI timeout and matching Buildbucket timeout.
	//
	// Turbo CI timeout will always be bigger by this margin. To avoid Buildbucket
	// and TurboCI colliding when they both try to handle a timeout event.
	timeoutMargin = 30 * time.Second

	// How long TurboCI will keep trying to call CancelStage RPC before failing
	// the stage. This is purely a property of the Buildbucket backend and it
	// doesn't depend on the build being cancelled.
	cancellingTimeout = 5 * time.Minute
)

// scheduleBuildRequestToPolicyTimeout constructs policy timeouts based on
// what's in `req`.
func scheduleBuildRequestToPolicyTimeout(req *pb.ScheduleBuildRequest) *orchestratorpb.StageAttemptExecutionPolicy_Timeout {
	if req.SchedulingTimeout == nil && req.ExecutionTimeout == nil && req.GracePeriod == nil {
		return nil
	}
	return orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
		Scheduled:   req.GetSchedulingTimeout(),
		Running:     req.GetExecutionTimeout(),
		TearingDown: req.GetGracePeriod(),
	}.Build()
}

// scheduleBuildRequestFromPolicyTimeout populates timeouts in `req` with
// what's in the policy.
//
// This is reverse of scheduleBuildRequestToPolicyTimeout(...).
func scheduleBuildRequestFromPolicyTimeout(req *pb.ScheduleBuildRequest, timeouts *orchestratorpb.StageAttemptExecutionPolicy_Timeout) {
	req.SchedulingTimeout = timeouts.GetScheduled()
	req.ExecutionTimeout = timeouts.GetRunning()
	req.GracePeriod = timeouts.GetTearingDown()
}

// buildToAttemptExecutionPolicy converts timeouts in the build into a policy.
//
// To avoid clashes between Turbo CI and Buildbucket timeout enforcing
// mechanisms, make all Turbo CI timeouts slightly larger (i.e. we'll
// primarily rely on Buildbucket to enforce timeouts).
//
// Heartbeat timeouts are unset, we'll rely on Buildbucket to track heartbeats.
func buildToAttemptExecutionPolicy(bld *pb.Build) *orchestratorpb.StageAttemptExecutionPolicy {
	return orchestratorpb.StageAttemptExecutionPolicy_builder{
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			Scheduled:   durationWithMargin(bld.GetSchedulingTimeout(), timeoutMargin),
			Running:     durationWithMargin(bld.GetExecutionTimeout(), timeoutMargin),
			TearingDown: durationWithMargin(bld.GetGracePeriod(), timeoutMargin+buildbucket.MinUpdateBuildInterval),
			Cancelling:  durationpb.New(cancellingTimeout),
		}.Build(),
	}.Build()
}

func durationWithMargin(d *durationpb.Duration, margin time.Duration) *durationpb.Duration {
	return durationpb.New(d.AsDuration() + margin)
}
