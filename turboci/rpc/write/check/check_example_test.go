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

import (
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/check"
)

func ExampleRealm() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.Realm("project/realm"),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "realm": "project/realm",
	//       "kind": "CHECK_KIND_TEST"
	//     }
	//   ]
	// }
}

func ExampleInWorkplan() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.InWorkplan("workplan-id"),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "workPlan": {
	//           "id": "workplan-id"
	//         },
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST"
	//     }
	//   ]
	// }
}

func ExampleOptions() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.Options(structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "options": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ]
	//     }
	//   ]
	// }
}

func ExampleRealmOptions() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.RealmOptions("project/realm", structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "options": [
	//         {
	//           "realm": "project/realm",
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ]
	//     }
	//   ]
	// }
}

func ExampleResults() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.Results(structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "results": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ]
	//     }
	//   ]
	// }
}

func ExampleRealmResults() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.RealmResults("project/realm", structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "results": [
	//         {
	//           "realm": "project/realm",
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ]
	//     }
	//   ]
	// }
}

func ExampleFinalResults() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.FinalResults(structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "results": [
	//         {
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ],
	//       "finalizeResults": true
	//     }
	//   ]
	// }
}

func ExampleFinalRealmResults() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.FinalRealmResults("project/realm", structpb.NewBoolValue(true)),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "results": [
	//         {
	//           "realm": "project/realm",
	//           "value": {
	//             "value": {
	//               "@type": "type.googleapis.com/google.protobuf.Value",
	//               "value": true
	//             }
	//           }
	//         }
	//       ],
	//       "finalizeResults": true
	//     }
	//   ]
	// }
}

func ExamplePlanned() {
	printCollected(
		write.NewCheck(
			"check-id",
			check.KindTest,
			check.Planned(),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "kind": "CHECK_KIND_TEST",
	//       "state": "CHECK_STATE_PLANNED"
	//     }
	//   ]
	// }
}

func ExampleFinal() {
	printCollected(
		write.UpdateCheck(
			id.Check("check-id"),
			check.Final(),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "check-id"
	//       },
	//       "finalizeResults": true,
	//       "state": "CHECK_STATE_FINAL"
	//     }
	//   ]
	// }
}
