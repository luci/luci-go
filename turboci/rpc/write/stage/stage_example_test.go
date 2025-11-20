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

package stage_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/attempt"
	"go.chromium.org/luci/turboci/rpc/write/stage"
)

func TestExample(t *testing.T) {
	t.Skip("examples are not run as tests")
}

func ExampleNewStage() {
	args, err := structpb.NewStruct(map[string]any{
		"hello": "world",
	})
	if err != nil {
		panic(err)
	}

	printCollected(
		write.NewStage(
			"stage-id",
			args,
			stage.Realm("project/realm"),
			stage.ShouldPlan("check-to-plan"),
			stage.Timeout(time.Hour, stage.TimeoutModeUnknown),
			stage.AttemptExecutionPolicy(
				attempt.HeartbeatRunning(10*time.Second),
				attempt.HeartbeatScheduled(60*time.Second),
			),
		),
	)
	// Output:
	// {
	//   "stages": [
	//     {
	//       "identifier": {
	//         "isWorknode": false,
	//         "id": "stage-id"
	//       },
	//       "args": {
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Struct",
	//           "value": {
	//             "hello": "world"
	//           }
	//         }
	//       },
	//       "realm": "project/realm",
	//       "requestedStageExecutionPolicy": {
	//         "stageTimeout": "3600s",
	//         "stageTimeoutMode": "STAGE_TIMEOUT_MODE_UNKNOWN",
	//         "attemptExecutionPolicyTemplate": {
	//           "attemptHeartbeat": {
	//             "scheduled": "60s",
	//             "running": "10s"
	//           }
	//         }
	//       },
	//       "assignments": [
	//         {
	//           "target": {
	//             "id": "check-to-plan"
	//           },
	//           "goalState": "CHECK_STATE_PLANNING"
	//         }
	//       ]
	//     }
	//   ]
	// }
}
