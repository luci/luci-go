// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"fmt"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// Omit sets the OmitReason in this ref.
//
// If the ref has inline data, it's converted to a digest and then dropped.
//
// Otherwise, if the omit reason is NO_ACCESS, this clears the digest and
// inline data.
//
// Will panic on unknown reasons, including the UNKNOWN (0) value.
func Omit(ref *orchestratorpb.ValueRef, reason orchestratorpb.OmitReason) {
	ref.SetOmitReason(reason)

	switch reason {
	case orchestratorpb.OmitReason_OMIT_REASON_UNWANTED:
		if ref.HasInline() {
			ref.SetDigest(string(ComputeDigest(ref.GetInline())))
		}
	case orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS:
		ref.ClearData()

	default:
		panic(fmt.Sprintf("unknown OmitReason: %q", reason))
	}
}
