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

package delta_test

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/proto/delta"
	"go.chromium.org/luci/common/proto/prototest"
	"go.chromium.org/luci/turboci/data"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// It's useful to define `builder` and `Diff` in your helper package as type
// aliases.
type (
	builder = orchestratorpb.WriteNodesRequest_builder

	// You will want to use the full typename in the Diff alias for godoc
	// purposes.
	Diff = delta.Diff[*orchestratorpb.WriteNodesRequest]
)

var template = delta.MakeTemplate[builder](nil)

// AddReason can be a private helper function, or exported from a package
// dedicated to assembling WriteNodesRequest messages.
//
// This returns a Diff (which is a partial WriteNodesRequest) which adds
// a reason built from `realm`, `msg` and `details`. Any errors encountered
// during this process are embedded in the returned Diff.
func AddReason(realm, msg string, details ...proto.Message) *Diff {
	vals, err := data.ValuesErr(details...)
	// holy boilerplate, batman!
	return template.New(builder{
		Reasons: []*orchestratorpb.WriteNodesRequest_Reason{
			orchestratorpb.WriteNodesRequest_Reason_builder{
				Reason:  &msg,
				Realm:   &realm,
				Details: vals,
			}.Build(),
		},
	}, err)
}

func Example() {
	// Now we can make a WriteNodesRequest in a much more comprehensible fashion.
	prototest.Print(delta.Collect(
		AddReason("some/realm", "hello there"),
		// We would use a real message type as a detail instead of a struct value.
		AddReason("some/secret-realm", "details reason", structpb.NewNumberValue(12345)),
	))
	// Output:
	// {
	//   "reasons": [
	//     {
	//       "realm": "some/realm",
	//       "reason": "hello there"
	//     },
	//     {
	//       "realm": "some/secret-realm",
	//       "reason": "details reason",
	//       "details": [
	//         {
	//           "value": {
	//             "@type": "type.googleapis.com/google.protobuf.Value",
	//             "value": 12345
	//           }
	//         }
	//       ]
	//     }
	//   ]
	// }
}
