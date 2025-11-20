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

	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/rpc/write/check"
)

func ExampleNewCheck() {
	printCollected(
		write.NewCheck(
			"fleeporp",
			check.KindAnalysis,
			check.Options(
				structpb.NewBoolValue(true),
			),
		),
	)
	// Output:
	// {
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
	//         }
	//       ]
	//     }
	//   ]
	// }
}

func ExampleUpdateCheck() {
	cid := id.Check("existing-check")
	printCollected(
		write.UpdateCheck(
			cid,
			check.Final(),
		),
	)
	// Output:
	// {
	//   "checks": [
	//     {
	//       "identifier": {
	//         "id": "existing-check"
	//       },
	//       "finalizeResults": true,
	//       "state": "CHECK_STATE_FINAL"
	//     }
	//   ]
	// }
}
