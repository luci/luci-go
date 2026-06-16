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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/proto/prototest"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	"go.chromium.org/turboci/proto/go/utils/ids"
	"go.chromium.org/turboci/proto/go/utils/value"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/dep"
)

func ExampleNewRequest() {
	// cid and cid2 would normally be obtained from the result of a query.
	cid := ids.Check("existing check")
	cid2 := ids.Check("another existing check")

	req := write.NewRequest()

	req.SetReason("stuff", value.MustWrite(numData))

	chk := req.AddNewCheck(ids.Check("fleeporp"), check.KindAnalysis)
	chk.AddOptions(value.MustWrite(numData), value.MustWrite(boolData))
	chk.AddResultData(value.MustWrite(numData), value.MustWrite(boolData, "very/secret"))

	chk = req.AddCheckUpdate(cid)
	chk.Msg.SetDependencies(dep.MustGroup(
		dep.ConditionalCheck(ids.Check("plorp"), check.StatePlanned),
		dep.MustGroup(
			ids.Check("external"),
			ids.Check("fleeporp"),
			dep.Threshold(1),
		),
	))
	chk.Msg.SetFinalizeResults(true)

	chk = req.AddCheckUpdate(cid2)
	chk.Msg.SetState(check.StateFinal)
	chk.Msg.SetFinalizeResults(true)

	stg := req.AddNewStage(ids.Stage("neeple"), numData)
	stg.Msg.SetDependencies(dep.MustGroup(
		dep.ConditionalCheck(ids.Check("fleeporp"), check.StatePlanned),
	))
	sep := &orchestratorpb.StageExecutionPolicy{}
	stg.Msg.SetRequestedStageExecutionPolicy(sep)
	sep.SetRetry(orchestratorpb.StageExecutionPolicy_Retry_builder{
		MaxRetries: proto.Int32(3),
	}.Build())
	sep.SetAttemptExecutionPolicyTemplate(orchestratorpb.StageAttemptExecutionPolicy_builder{
		Heartbeat: orchestratorpb.StageAttemptExecutionPolicy_Heartbeat_builder{
			Running: durationpb.New(5 * time.Minute),
		}.Build(),
		Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
			PendingThrottled: durationpb.New(time.Hour),
		}.Build(),
	}.Build())
	stg.AddCheckAssignment(ids.Check("fleeporp"), check.StateFinal)

	req.AddNewStage(ids.Stage("bleeple"), numData)

	req.AddStageCancellation(ids.StageWorkNode("bad one"))

	curAttempt := req.GetCurrentAttempt()
	curAttempt.GetStateTransition().SetRunning("my process UID", nil)
	curAttempt.AddProgress("a message")
	curAttempt.AddDetails(value.MustWrite(numData))
	req.GetCurrentStage().Msg.SetContinuationGroup(
		dep.MustGroup(
			ids.Stage("neeple"),
			ids.Stage("bleeple"),
			dep.Threshold(1),
		),
	)

	prototest.Print(req.Msg, nil)
	// Output:
	// {
	//   "reason": {
	//     "message": "stuff",
	//     "details": [
	//       {
	//         "data": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         },
	//         "realm": "$from_container"
	//       }
	//     ]
	//   },
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "fleeporp"
	//       },
	//       "kind": "CHECK_KIND_ANALYSIS",
	//       "options": [
	//         {
	//           "data": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": 100
	//           },
	//           "realm": "$from_container"
	//         },
	//         {
	//           "data": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": true
	//           },
	//           "realm": "$from_container"
	//         }
	//       ],
	//       "resultData": [
	//         {
	//           "data": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": 100
	//           },
	//           "realm": "$from_container"
	//         },
	//         {
	//           "data": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": true
	//           },
	//           "realm": "very/secret"
	//         }
	//       ]
	//     },
	//     {
	//       "identifier": {
	//         "id": "existing check"
	//       },
	//       "dependencies": {
	//         "edges": [
	//           {
	//             "check": {
	//               "identifier": {
	//                 "id": "plorp"
	//               },
	//               "condition": {
	//                 "onState": "CHECK_STATE_PLANNED"
	//               }
	//             }
	//           }
	//         ],
	//         "groups": [
	//           {
	//             "edges": [
	//               {
	//                 "check": {
	//                   "identifier": {
	//                     "id": "external"
	//                   }
	//                 }
	//               },
	//               {
	//                 "check": {
	//                   "identifier": {
	//                     "id": "fleeporp"
	//                   }
	//                 }
	//               }
	//             ],
	//             "threshold": 1
	//           }
	//         ]
	//       },
	//       "finalizeResults": true
	//     },
	//     {
	//       "identifier": {
	//         "id": "another existing check"
	//       },
	//       "finalizeResults": true,
	//       "state": "CHECK_STATE_FINAL"
	//     }
	//   ],
	//   "stages": [
	//     {
	//       "identifier": {
	//         "isWorknode": false,
	//         "id": "neeple"
	//       },
	//       "args": {
	//         "data": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         },
	//         "realm": "$from_container"
	//       },
	//       "dependencies": {
	//         "edges": [
	//           {
	//             "check": {
	//               "identifier": {
	//                 "id": "fleeporp"
	//               },
	//               "condition": {
	//                 "onState": "CHECK_STATE_PLANNED"
	//               }
	//             }
	//           }
	//         ]
	//       },
	//       "requestedStageExecutionPolicy": {
	//         "retry": {
	//           "maxRetries": 3
	//         },
	//         "attemptExecutionPolicyTemplate": {
	//           "heartbeat": {
	//             "running": "300s"
	//           },
	//           "timeout": {
	//             "pendingThrottled": "3600s"
	//           }
	//         }
	//       },
	//       "assignments": [
	//         {
	//           "target": {
	//             "id": "fleeporp"
	//           },
	//           "goalState": "CHECK_STATE_FINAL"
	//         }
	//       ]
	//     },
	//     {
	//       "identifier": {
	//         "isWorknode": false,
	//         "id": "bleeple"
	//       },
	//       "args": {
	//         "data": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         },
	//         "realm": "$from_container"
	//       }
	//     },
	//     {
	//       "identifier": {
	//         "isWorknode": true,
	//         "id": "bad one"
	//       },
	//       "cancelled": true
	//     }
	//   ],
	//   "currentAttempt": {
	//     "details": [
	//       {
	//         "data": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         },
	//         "realm": "$from_container"
	//       }
	//     ],
	//     "progress": [
	//       {
	//         "message": "a message"
	//       }
	//     ],
	//     "stateTransition": {
	//       "running": {
	//         "processUid": "my process UID"
	//       }
	//     }
	//   },
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
