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

package write_test

import (
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/attempt"
	"go.chromium.org/luci/turboci/rpc/write/stage"
)

func ExampleNewStage() {
	printCollected(
		write.NewStage(
			"some-stage",
			structpb.NewBoolValue(true),
			stage.AttemptExecutionPolicy(
				attempt.HeartbeatRunning(10*time.Second),
			),
		),
	)
	// Output:
	// {
	//   "stages": [
	//     {
	//       "identifier": {
	//         "isWorknode": false,
	//         "id": "some-stage"
	//       },
	//       "args": {
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": true
	//         }
	//       },
	//       "requestedStageExecutionPolicy": {
	//         "attemptExecutionPolicyTemplate": {
	//           "attemptHeartbeat": {
	//             "running": "10s"
	//           }
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleCancelStage() {
	printCollected(
		write.CancelStage("stage-to-cancel", false),
	)
	// Output:
	// {
	//   "stages": [
	//     {
	//       "identifier": {
	//         "isWorknode": false,
	//         "id": "stage-to-cancel"
	//       },
	//       "cancelled": true
	//     }
	//   ]
	// }
}
