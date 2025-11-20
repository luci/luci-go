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

package check_test

import "go.chromium.org/luci/turboci/rpc/write/check"

func ExampleEdge() {
	printCollected(
		check.Edge("something"),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "check": {
	//         "identifier": {
	//           "id": "something"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetInWorkplan() {
	printCollected(
		check.Edge("something", check.TargetInWorkplan("bob")),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "check": {
	//         "identifier": {
	//           "workPlan": {
	//             "id": "bob"
	//           },
	//           "id": "something"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetCondition() {
	printCollected(
		check.Edge("something", check.TargetCondition(check.StateWaiting)),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "check": {
	//         "identifier": {
	//           "id": "something"
	//         },
	//         "condition": {
	//           "onState": "CHECK_STATE_WAITING"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetCondition_expression() {
	printCollected(
		check.Edge("something", check.TargetCondition(
			check.StateFinal, "true")),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "check": {
	//         "identifier": {
	//           "id": "something"
	//         },
	//         "condition": {
	//           "onState": "CHECK_STATE_FINAL",
	//           "expression": "true"
	//         }
	//       }
	//     }
	//   ]
	// }
}

func ExampleTargetCondition_multi_expression() {
	printCollected(
		check.Edge("something", check.TargetCondition(
			check.StateFinal, "true", "othervar==1")),
	)
	// Output:
	// {
	//   "edges": [
	//     {
	//       "check": {
	//         "identifier": {
	//           "id": "something"
	//         },
	//         "condition": {
	//           "onState": "CHECK_STATE_FINAL",
	//           "expression": "(true)&&(othervar==1)"
	//         }
	//       }
	//     }
	//   ]
	// }
}
