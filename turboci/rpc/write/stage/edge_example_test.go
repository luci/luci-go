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

import "go.chromium.org/luci/turboci/rpc/write/stage"

func ExampleEdge() {
	printCollected(
		stage.Edge("something"),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "stage": {
	//         "identifier": {
	//           "isWorknode": false,
	//           "id": "something"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetInWorkplan() {
	printCollected(
		stage.Edge("something", stage.TargetInWorkplan("bob")),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "stage": {
	//         "identifier": {
	//           "workPlan": {
	//             "id": "bob"
	//           },
	//           "isWorknode": false,
	//           "id": "something"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetCondition() {
	printCollected(
		stage.Edge("something", stage.TargetCondition(stage.StateAwaitingGroup)),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "stage": {
	//         "identifier": {
	//           "isWorknode": false,
	//           "id": "something"
	//         },
	//         "condition": {
	//           "onState": "STAGE_STATE_AWAITING_GROUP"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetCondition_expression() {
	printCollected(
		stage.Edge("something", stage.TargetCondition(
			stage.StateFinal, "true")),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "stage": {
	//         "identifier": {
	//           "isWorknode": false,
	//           "id": "something"
	//         },
	//         "condition": {
	//           "onState": "STAGE_STATE_FINAL",
	//           "expression": "true"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetCondition_multi_expression() {
	printCollected(
		stage.Edge("something", stage.TargetCondition(
			stage.StateFinal, "true", "othervar==1")),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "stage": {
	//         "identifier": {
	//           "isWorknode": false,
	//           "id": "something"
	//         },
	//         "condition": {
	//           "onState": "STAGE_STATE_FINAL",
	//           "expression": "(true)&&(othervar==1)"
	//         }
	//       }
	//     }
	//   ]
	// }
}
