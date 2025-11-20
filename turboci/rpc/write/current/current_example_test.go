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

package current_test

import (
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/attempt"
	"go.chromium.org/luci/turboci/rpc/write/current"
	"go.chromium.org/luci/turboci/rpc/write/stage"
)

func ExampleAttemptScheduled() {
	printCollected(
		write.Current(
			current.AttemptScheduled(),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "state": "STAGE_ATTEMPT_STATE_SCHEDULED"
	//   }
	// }
}

func ExampleAttemptRunning() {
	printCollected(
		write.Current(
			current.AttemptRunning("process-uid"),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "state": "STAGE_ATTEMPT_STATE_RUNNING",
	//     "processUid": "process-uid"
	//   }
	// }
}

func ExampleAttemptFinished() {
	printCollected(
		write.Current(
			current.AttemptFinished(true),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "state": "STAGE_ATTEMPT_STATE_COMPLETE"
	//   }
	// }
}

func ExampleAttemptDetails() {
	printCollected(
		write.Current(
			current.AttemptDetails(structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "currentStage": {
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

func ExampleAttemptProgress() {
	printCollected(
		write.Current(
			current.AttemptProgress("doing a thing", structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "progress": [
	//       {
	//         "msg": "doing a thing",
	//         "details": [
	//           {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         ]
	//       }
	//     ]
	//   }
	// }
}

func ExampleStageContinuationGroup() {
	printCollected(
		write.Current(
			current.StageContinuationGroup(
				stage.Edge("some-stage"),
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
	//               "id": "some-stage"
	//             }
	//           }
	//         }
	//       ]
	//     }
	//   }
	// }
}

func ExampleAttemptExecutionPolicy() {
	printCollected(
		write.Current(
			current.AttemptExecutionPolicy(
				attempt.HeartbeatRunning(10*time.Second),
				attempt.TimeoutRunning(5*time.Minute),
			),
		),
	)
	// Output:
	// {
	//   "currentStage": {
	//     "attemptExecutionPolicy": {
	//       "attemptHeartbeat": {
	//         "running": "10s"
	//       },
	//       "timeout": {
	//         "running": "300s"
	//       }
	//     }
	//   }
	// }
}
