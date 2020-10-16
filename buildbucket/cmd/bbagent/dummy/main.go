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
	"os"
	"os/signal"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/luciexe/exe"
)

func main() {
	exe.Run(func(ctx context.Context, build *pb.Build, userArgs []string, sendBuild exe.BuildSender) error {
		loc, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			return err
		}
		build.Status = pb.Status_STARTED
		build.Steps = append(build.Steps, &pb.Step{
			Name:      "fake step",
			Status:    pb.Status_SUCCESS,
			StartTime: timestamppb.Now(),
			EndTime:   timestamppb.Now(),
		})
		sendBuild()
		sigch := make(chan os.Signal, 10)
		signal.Notify(sigch, signals.Interrupts()...)
		fmt.Fprintln(os.Stderr, time.Now().In(loc), "Waiting for signal")
		<-sigch
		fmt.Fprintln(os.Stderr, time.Now().In(loc), "Receive Interruption")
		for range time.Tick(500 * time.Millisecond) {
			fmt.Fprintln(os.Stderr, time.Now().In(loc), "I'm still alive")
		}
		return nil
	})
}
