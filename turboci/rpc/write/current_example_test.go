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
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/current"
	"go.chromium.org/luci/turboci/rpc/write/dep"
	"go.chromium.org/luci/turboci/rpc/write/stage"
)

func ExampleCurrent_stage() {
	printCollected(
		write.Current(
			current.StageContinuationGroup(
				stage.Edge("neeple"),
				stage.Edge("bleeple"),
				dep.Threshold(1),
			),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "continuationGroup": {
	//       "edges": [
	//         {
	//           "stage": {
	//             "identifier": {
	//               "isWorknode": false,
	//               "id": "neeple"
	//             }
	//           }
	//         },
	//         {
	//           "stage": {
	//             "identifier": {
	//               "isWorknode": false,
	//               "id": "bleeple"
	//             }
	//           }
	//         }
	//       ],
	//       "threshold": 1
	//     }
	//   }
	// }
}

func ExampleCurrent_attempt() {
	printCollected(
		write.Current(
			current.AttemptRunning("my-process-uid"),
			current.AttemptDetails(structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "state": "STAGE_ATTEMPT_STATE_RUNNING",
	//     "processUid": "my-process-uid",
	//     "details": [
	//       {
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": true
	//         }
	//       }
	//     ]
	//   }
	// }
}
