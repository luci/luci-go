// Copyright 2020 The LUCI Authors.
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

package main

import (
	"context"
	"fmt"
	"time"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/luciexe/exe"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	exe.Run(func(ctx context.Context, build *pb.Build, userArgs []string, sendBuild exe.BuildSender) error {
		build.Status = pb.Status_STARTED

		for i := 0; i < 3; i++ {
			step := &pb.Step{
				Name:      fmt.Sprintf("Step %d", i),
				Status:    pb.Status_STARTED,
				StartTime: timestamppb.Now(),
			}
			build.Steps = append(build.Steps, step)
			<-time.After(3 * time.Second)
			step.EndTime = timestamppb.Now()
			step.Status = pb.Status_SUCCESS
			sendBuild()
		}

		build.Steps = append(build.Steps, &pb.Step{
			Name:      "SimulateUpdateBuildFailureNow",
			Status:    pb.Status_INFRA_FAILURE,
			StartTime: timestamppb.Now(),
			EndTime:   timestamppb.Now(),
		})
		sendBuild()
		<-time.After(3 * time.Second)

		for i := 3; i < 5; i++ {
			step := &pb.Step{
				Name:      fmt.Sprintf("Step %d", i),
				Status:    pb.Status_STARTED,
				StartTime: timestamppb.Now(),
			}
			build.Steps = append(build.Steps, step)
			<-time.After(3 * time.Second)
			step.EndTime = timestamppb.Now()
			step.Status = pb.Status_SUCCESS
			sendBuild()
		}

		build.Status = pb.Status_SUCCESS
		build.EndTime = timestamppb.Now()
		sendBuild()
		return nil
	})
}
