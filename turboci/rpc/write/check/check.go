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

package check

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/proto/delta"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
)

type (
	// Diff modifies a [WriteNodesRequest.CheckWrite].
	//
	// [WriteNodesRequest.CheckWrite]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/orchestrator/v1/write_nodes_request.proto#214
	Diff = delta.Diff[*orchestratorpb.WriteNodesRequest_CheckWrite]

	builder = orchestratorpb.WriteNodesRequest_CheckWrite_builder
)

var template = delta.MakeTemplate[builder](map[string]delta.ApplyMode{
	"identifier": delta.ModeMerge,
	"state":      delta.ModeMaxEnum,
})

// Realm is used to set the realm for a new Check.
//
// If this is used for an existing Check, it must exactly match the
// realm of the existing Check.
func Realm(realm string) *Diff {
	return template.New(builder{
		Realm: &realm,
	})
}

// InWorkplan sets the WorkPlan of this CheckWrite's identifier.
//
// NOTE: As of 2026Q1, cross-WorkPlan writes are not supported.
func InWorkplan(workplanID string) *Diff {
	return template.New(builder{
		Identifier: idspb.Check_builder{
			WorkPlan: idspb.WorkPlan_builder{
				Id: &workplanID,
			}.Build(),
		}.Build(),
	})
}

// Options returns a Diff to add one or more Option values to the CheckWrite.
//
// They will inherit their realm the target Check's realm.
func Options(opts ...proto.Message) *Diff {
	return RealmOptions("", opts...)
}

func realmValues(name, realm string, messages ...proto.Message) ([]*orchestratorpb.WriteNodesRequest_RealmValue, error) {
	vals, err := data.ValuesErr(messages...)
	if err != nil {
		err = fmt.Errorf("check.%s: %w", name, err)
	}

	realmVals := make([]*orchestratorpb.WriteNodesRequest_RealmValue, len(messages))
	for i, val := range vals {
		realmVal := orchestratorpb.WriteNodesRequest_RealmValue_builder{
			Value: val,
		}.Build()
		if realm != "" {
			realmVal.SetRealm(realm)
		}
		realmVals[i] = realmVal
	}
	return realmVals, err
}

// RealmOptions returns a Diff to add one or more Option values (with explicit
// realm) to the CheckWrite.
func RealmOptions(realm string, opts ...proto.Message) *Diff {
	vals, err := realmValues("RealmOptions", realm, opts...)
	return template.New(builder{
		Options: vals,
	}, err)
}

// Results returns a Diff to add one or more Result values to the CheckWrite.
//
// These will be set in the Check's Result which corresponds to the writer's
// Stage Attempt.
//
// The realm will be inherited from the target Check's realm.
func Results(results ...proto.Message) *Diff {
	return RealmResults("", results...)
}

// RealmResults returns a Diff to add one or more Results (with explicit realm)
// to the CheckWrite.
//
// These will be set in the Check's Result which corresponds to the writer's
// Stage Attempt.
func RealmResults(realm string, results ...proto.Message) *Diff {
	vals, err := realmValues("RealmResults", realm, results...)
	return template.New(builder{
		Results: vals,
	}, err)
}

// FinalResults returns a Diff to mark the Results belonging to the writer's
// Stage Attempt as final.
//
// As a convenience, you may also write zero or more result data.
//
// This is equivalent to `RealmResults` plus setting finalize_results to true.
//
// The realm will be inherited from the target Check's realm.
func FinalResults(data ...proto.Message) *Diff {
	return FinalRealmResults("", data...)
}

// FinalResults returns a Diff to mark the Results (with explicit realm) as
// final (from the current stage attempt).
//
// As a convenience, you may also write zero or more result data.
//
// This is equivalent to `RealmResults` plus setting finalize_results to true.
func FinalRealmResults(realm string, data ...proto.Message) *Diff {
	return delta.Combine(
		RealmResults(realm, data...),
		template.New(builder{
			FinalizeResults: proto.Bool(true),
		}))
}

// Planned returns a Diff which sets the state in this CheckWrite to
// CHECK_STATE_PLANNED.
//
// Once this is done, the check may no longer receive edits to its Options, and
// the orchestrator will start resolving dependencies of the Check. Once all the
// dependencies of the check are resolved, the orchestrator will move the Check
// to the WAITING state automatically.
//
// Moving a Check with no dependencies to PLANNED will show it transition
// immediately to WAITING.
func Planned() *Diff {
	return template.New(builder{
		State: StatePlanned.Enum(),
	})
}

// Final returns a Diff which sets the state in this CheckWrite to
// CHECK_STATE_FINAL.
//
// Once this is done, the check will not be modified/modifiable in any way.
func Final() *Diff {
	return template.New(builder{
		State:           StateFinal.Enum(),
		FinalizeResults: proto.Bool(true),
	})
}
