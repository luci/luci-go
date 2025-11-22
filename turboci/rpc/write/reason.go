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

package write

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
)

// Reason returns a Diff which adds a new Reason to this write.
//
// Every WriteNodesRequest requires at least one Reason.
//
// This will inherit the realm of the Stage doing the write.
func Reason(reason string, details ...proto.Message) *Diff {
	return ReasonRealm("", reason, details...)
}

// ReasonRealm returns a Diff which adds a new Reason to this write within
// a specific realm.
//
// Every WriteNodesRequest requires at least one Reason.
//
// If `realm` is empty, this will inherit the realm of the Stage doing the
// write.
func ReasonRealm(realm, reason string, details ...proto.Message) *Diff {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		err = fmt.Errorf("write.ReasonRealm.details: %w", err)
	}
	reasonMsg := orchestratorpb.WriteNodesRequest_Reason_builder{
		Reason:  &reason,
		Details: vals,
	}.Build()
	if realm != "" {
		reasonMsg.SetRealm(realm)
	}
	return template.New(builder{
		Reasons: []*orchestratorpb.WriteNodesRequest_Reason{reasonMsg},
	}, err)
}
