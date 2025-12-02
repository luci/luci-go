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

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/dep"
	"go.chromium.org/luci/turboci/stage"
)

func ExampleNewRequest() {
	// cid and cid2 would normally be obtained from the result of a query.
	cid := id.Check("existing check")
	cid2 := id.Check("another existing check")

	req := write.NewRequest()

	if _, err := req.AddReason("stuff", numData); err != nil {
		panic(err)
	}

	chk := req.AddNewCheck(id.Check("fleeporp"), check.KindAnalysis)
	if _, err := chk.AddOptions(numData, boolData); err != nil {
		panic(err)
	}
	if _, err := chk.AddResults(numData); err != nil {
		panic(err)
	}
	rslts, err := chk.AddResults(boolData)
	if err != nil {
		panic(err)
	}
	rslts.SetRealm("very/secret")

	chk = req.AddCheckUpdate(cid)
	chk.Msg.SetDependencies(dep.MustGroup(
		dep.ConditionalCheck(id.Check("plorp"), check.StatePlanned),
		dep.MustGroup(
			id.Check("external"),
			id.Check("fleeporp"),
			dep.Threshold(1),
		),
	))
	chk.Msg.SetFinalizeResults(true)

	chk = req.AddCheckUpdate(cid2)
	chk.Msg.SetState(check.StateFinal)
	chk.Msg.SetFinalizeResults(true)

	stg, err := req.AddNewStage(id.Stage("neeple"), numData)
	if err != nil {
		panic(err)
	}
	stg.Msg.SetDependencies(dep.MustGroup(
		dep.ConditionalCheck(id.Check("fleeporp"), check.StatePlanned),
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
	stg.AddCheckAssignment(id.Check("fleeporp"), check.StateFinal)

	if _, err = req.AddNewStage(id.Stage("bleeple"), numData); err != nil {
		panic(err)
	}

	req.AddStageCancellation(id.StageWorkNode("bad one"))

	cur := req.GetCurrentStage()
	cur.Msg.SetState(stage.AttemptStateRunning)
	cur.Msg.SetProcessUid("my process UID")
	cur.AddProgress("a message")
	if _, err := cur.AddDetails(numData); err != nil {
		panic(err)
	}
	cur.Msg.SetContinuationGroup(
		dep.MustGroup(
			id.Stage("neeple"),
			id.Stage("bleeple"),
			dep.Threshold(1),
		),
	)

	prototest.Print(req.Msg, nil)
	// Output:
	// {
	//   "reasons": [
	//     {
	//       "reason": "stuff",
	//       "details": [
	//         {
	//           "value": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": 100
	//           }
	//         }
	//       ]
	//     }
	//   ],
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "fleeporp"
	//       },
	//       "kind": "CHECK_KIND_ANALYSIS",
	//       "options": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": 100
	//             }
	//           }
	//         },
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ],
	//       "results": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": 100
	//             }
	//           }
	//         },
	//         {
	//           "realm": "very/secret",
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
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
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         }
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
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         }
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
	//   "currentStage": {
	//     "state": "STAGE_ATTEMPT_STATE_RUNNING",
	//     "processUid": "my process UID",
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
	//     },
	//     "details": [
	//       {
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": 100
	//         }
	//       }
	//     ],
	//     "progress": [
	//       {
	//         "msg": "a message"
	//       }
	//     ]
	//   }
	// }
}
