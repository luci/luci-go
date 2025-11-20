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
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/proto/prototest"
	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/attempt"
	"go.chromium.org/luci/turboci/rpc/write/check"
	"go.chromium.org/luci/turboci/rpc/write/current"
	"go.chromium.org/luci/turboci/rpc/write/dep"
	"go.chromium.org/luci/turboci/rpc/write/stage"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

var data1 = structpb.NewBoolValue(true)
var data2 = structpb.NewStringValue("fleem")

func Example_full_request() {
	// cid would normally be obtained from the result of a query.
	cid := id.Check("existing check")
	cid2 := id.Check("another existing check")

	printCollected(
		write.Reason("stuff", data1),
		write.NewCheck(
			"fleeporp",
			check.KindAnalysis,
			check.Options(
				data1,
				data2,
			),
			check.Results(
				data1,
			),
			check.RealmResults(
				"very/secret",
				data2,
			),
		),
		write.UpdateCheck(
			cid,
			check.Deps(
				check.Edge("plorp", check.TargetCondition(check.StatePlanned)),
				dep.Group(
					check.Edge("external"),
					check.Edge("fleeporp"),
					dep.Threshold(1),
				),
			),
			check.FinalResults(),
		),
		write.UpdateCheck(
			cid2,
			check.Final(),
		),
		write.NewStage(
			"neeple",
			data1,
			stage.Deps(
				check.Edge("fleeporp",
					check.TargetCondition(check.StatePlanned))),
			stage.ShouldFinalize("fleeporp"),
			stage.MaxRetries(3),
			stage.AttemptExecutionPolicy(
				attempt.HeartbeatRunning(5*time.Minute),
				attempt.TimeoutPendingThrottled(time.Hour),
			),
		),
		write.NewStage("beeple", data1),
		write.Current(
			current.StageContinuationGroup(
				stage.Edge("neeple"),
				stage.Edge("bleeple"),
				dep.Threshold(1),
			),
			current.AttemptRunning("my process UID"),
			current.AttemptDetails(data1),
			current.AttemptProgress("a message"),
		),
		write.CancelStage("bad one", true),
	)
	// Output:
	// {
	//   "reasons": [
	//     {
	//       "reason": "stuff",
	//       "details": [
	//         {
	//           "value": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": true
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
	//               "value": true
	//             }
	//           }
	//         },
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": "fleem"
	//             }
	//           }
	//         }
	//       ],
	//       "results": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         },
	//         {
	//           "realm": "very/secret",
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": "fleem"
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
	//           "value": true
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
	//           "attemptHeartbeat": {
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
	//         "id": "beeple"
	//       },
	//       "args": {
	//         "value": {
	//           "@type": "type.googleapis.com/google.protobuf.Value",
	//           "value": true
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
	//           "value": true
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

func Example_full_request_builder() {
	// As an alternative, this shows the same as the other example using just the
	// generated proto _builder API.
	check2ID := id.Check("existing check")
	check3ID := id.Check("another existing check")

	reasonData, err := data.ValuesErr(data1)
	if err != nil {
		panic(err)
	}
	check1ID, err := id.CheckErr("fleeporp")
	if err != nil {
		panic(err)
	}
	check1Options, err := data.ValuesErr(data1, data2)
	if err != nil {
		panic(err)
	}
	check1Results, err := data.ValuesErr(data1, data2)
	if err != nil {
		panic(err)
	}
	check1Edge1, err := id.CheckErr("plorp")
	if err != nil {
		panic(err)
	}
	check1Edge2, err := id.CheckErr("external")
	if err != nil {
		panic(err)
	}
	stage1ID, err := id.StageErr(id.StageNotWorknode, "neeple")
	if err != nil {
		panic(err)
	}
	stage1Args, err := data.ValueErr(data1)
	if err != nil {
		panic(err)
	}
	stage2ID, err := id.StageErr(id.StageNotWorknode, "bleeple")
	if err != nil {
		panic(err)
	}
	stage2Args, err := data.ValueErr(data1)
	if err != nil {
		panic(err)
	}
	stage3ID, err := id.StageErr(id.StageIsWorknode, "bad one")
	if err != nil {
		panic(err)
	}
	attemptDetails, err := data.ValuesErr(data1)
	if err != nil {
		panic(err)
	}

	wrn := orchestratorpb.WriteNodesRequest_builder{
		Reasons: []*orchestratorpb.WriteNodesRequest_Reason{
			orchestratorpb.WriteNodesRequest_Reason_builder{
				Reason:  proto.String("stuff"),
				Details: reasonData,
			}.Build(),
		},
		Checks: []*orchestratorpb.WriteNodesRequest_CheckWrite{
			orchestratorpb.WriteNodesRequest_CheckWrite_builder{
				Identifier: check1ID,
				Kind:       orchestratorpb.CheckKind_CHECK_KIND_ANALYSIS.Enum(),
				Options: []*orchestratorpb.WriteNodesRequest_RealmValue{
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: check1Options[0],
					}.Build(),
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: check1Options[1],
					}.Build(),
				},
				Results: []*orchestratorpb.WriteNodesRequest_RealmValue{
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: check1Results[0],
					}.Build(),
					orchestratorpb.WriteNodesRequest_RealmValue_builder{
						Value: check1Results[1],
						Realm: proto.String("very/secret"),
					}.Build(),
				},
			}.Build(),
			orchestratorpb.WriteNodesRequest_CheckWrite_builder{
				Identifier: check2ID,
				Dependencies: orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
					Edges: []*orchestratorpb.Edge{
						orchestratorpb.Edge_builder{
							Check: orchestratorpb.Edge_Check_builder{
								Identifier: check1Edge1,
								Condition: orchestratorpb.Edge_Check_Condition_builder{
									OnState: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
								}.Build(),
							}.Build(),
						}.Build(),
					},
					Groups: []*orchestratorpb.WriteNodesRequest_DependencyGroup{
						orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
							Edges: []*orchestratorpb.Edge{
								orchestratorpb.Edge_builder{
									Check: orchestratorpb.Edge_Check_builder{
										Identifier: check1Edge2,
									}.Build(),
								}.Build(),
								orchestratorpb.Edge_builder{
									Check: orchestratorpb.Edge_Check_builder{
										Identifier: check1ID,
									}.Build(),
								}.Build(),
							},
							Threshold: proto.Int32(1),
						}.Build(),
					},
				}.Build(),
				FinalizeResults: proto.Bool(true),
			}.Build(),
			orchestratorpb.WriteNodesRequest_CheckWrite_builder{
				Identifier:      check3ID,
				FinalizeResults: proto.Bool(true),
				State:           orchestratorpb.CheckState_CHECK_STATE_FINAL.Enum(),
			}.Build(),
		},
		Stages: []*orchestratorpb.WriteNodesRequest_StageWrite{
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: stage1ID,
				Args:       stage1Args,
				Dependencies: orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
					Edges: []*orchestratorpb.Edge{
						orchestratorpb.Edge_builder{
							Check: orchestratorpb.Edge_Check_builder{
								Identifier: check1ID,
								Condition: orchestratorpb.Edge_Check_Condition_builder{
									OnState: orchestratorpb.CheckState_CHECK_STATE_PLANNED.Enum(),
								}.Build(),
							}.Build(),
						}.Build(),
					},
				}.Build(),
				Assignments: []*orchestratorpb.Stage_Assignment{
					orchestratorpb.Stage_Assignment_builder{
						Target:    check1ID,
						GoalState: orchestratorpb.CheckState_CHECK_STATE_FINAL.Enum(),
					}.Build(),
				},
				RequestedStageExecutionPolicy: orchestratorpb.StageExecutionPolicy_builder{
					Retry: orchestratorpb.StageExecutionPolicy_Retry_builder{
						MaxRetries: proto.Int32(3),
					}.Build(),
					AttemptExecutionPolicyTemplate: orchestratorpb.StageAttemptExecutionPolicy_builder{
						AttemptHeartbeat: orchestratorpb.StageAttemptExecutionPolicy_Heartbeat_builder{
							Running: durationpb.New(5 * time.Minute),
						}.Build(),
						Timeout: orchestratorpb.StageAttemptExecutionPolicy_Timeout_builder{
							PendingThrottled: durationpb.New(time.Hour),
						}.Build(),
					}.Build(),
				}.Build(),
			}.Build(),
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: stage2ID,
				Args:       stage2Args,
			}.Build(),
			orchestratorpb.WriteNodesRequest_StageWrite_builder{
				Identifier: stage3ID,
				Cancelled:  proto.Bool(true),
			}.Build(),
		},
		CurrentStage: orchestratorpb.WriteNodesRequest_CurrentStageWrite_builder{
			ContinuationGroup: orchestratorpb.WriteNodesRequest_DependencyGroup_builder{
				Edges: []*orchestratorpb.Edge{
					orchestratorpb.Edge_builder{
						Stage: orchestratorpb.Edge_Stage_builder{
							Identifier: stage1ID,
						}.Build(),
					}.Build(),
					orchestratorpb.Edge_builder{
						Stage: orchestratorpb.Edge_Stage_builder{
							Identifier: stage2ID,
						}.Build(),
					}.Build(),
				},
				Threshold: proto.Int32(1),
			}.Build(),
			State:      attempt.StateRunning.Enum(),
			ProcessUid: proto.String("my process UID"),
			Details:    attemptDetails,
			Progress: []*orchestratorpb.WriteNodesRequest_StageAttemptProgress{
				orchestratorpb.WriteNodesRequest_StageAttemptProgress_builder{
					Msg: proto.String("a message"),
				}.Build(),
			},
		}.Build(),
	}.Build()

	prototest.Print(wrn, nil)
	// Output:
	// {
	//   "reasons": [
	//     {
	//       "reason": "stuff",
	//       "details": [
	//         {
	//           "value": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": true
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
	//               "value": true
	//             }
	//           }
	//         },
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": "fleem"
	//             }
	//           }
	//         }
	//       ],
	//       "results": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         },
	//         {
	//           "realm": "very/secret",
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": "fleem"
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
	//           "value": true
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
	//           "attemptHeartbeat": {
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
	//           "value": true
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
	//           "value": true
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
