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

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/check"
	"go.chromium.org/luci/turboci/data"
)

// RealmsValues encapsulates a slice of RealmValue messages.
//
// In particular, this can easily set the realm for all messages in the slice
// with the [RealmsValues.SetRealm] method.
type RealmsValues []*orchestratorpb.WriteNodesRequest_RealmValue

// SetRealm modifies all contained RealmValue messages to have the given realm.
func (rv RealmsValues) SetRealm(realm string) {
	for _, val := range rv {
		val.SetRealm(realm)
	}
}

// CheckWrite wraps an orchestratorpb.WriteNodesRequest_CheckWrite.
//
// This notably provides the helpers to add options and results to the
// CheckWrite.
//
// See package documentation for the behavior of this wrapper type.
type CheckWrite struct {
	Msg *orchestratorpb.WriteNodesRequest_CheckWrite
}

// AddOptions appends and returns one or more option values.
//
// The realm for these can be set with [RealmsValues.SetRealm].
func (cw CheckWrite) AddOptions(opt ...proto.Message) (RealmsValues, error) {
	vals, err := data.ValuesErr(opt...)
	if err != nil {
		return nil, fmt.Errorf("write.CheckWrite.AddOptionsInRealm: %w", err)
	}
	toAdd := make([]*orchestratorpb.WriteNodesRequest_RealmValue, len(vals))
	for i, val := range vals {
		toAdd[i] = orchestratorpb.WriteNodesRequest_RealmValue_builder{
			Value: val,
		}.Build()
	}
	cw.Msg.SetOptions(append(cw.Msg.GetOptions(), toAdd...))
	return toAdd, nil
}

// AddResults appends and returns one or more result values.
//
// The realm for these can be set with [RealmsValues.SetRealm].
func (cw CheckWrite) AddResults(rslt ...proto.Message) (RealmsValues, error) {
	vals, err := data.ValuesErr(rslt...)
	if err != nil {
		return nil, fmt.Errorf("write.CheckWrite.AddResultsInRealm: %w", err)
	}
	toAdd := make([]*orchestratorpb.WriteNodesRequest_RealmValue, len(vals))
	for i, val := range vals {
		toAdd[i] = orchestratorpb.WriteNodesRequest_RealmValue_builder{
			Value: val,
		}.Build()
	}
	cw.Msg.SetResults(append(cw.Msg.GetResults(), toAdd...))
	return toAdd, nil
}

// AddNewCheck adds a new CheckWrite to the request for the creation of
// a new Check (i.e. the writer believes the check does not already exist in the
// graph).
func (req Request) AddNewCheck(id *idspb.Check, kind check.Kind) CheckWrite {
	ret := req.AddCheckUpdate(id)
	ret.Msg.SetKind(kind)
	return ret
}

// AddCheckUpdate adds a new CheckWrite to the request for the update of an
// existing Check (i.e. the writer believes the check already exists in the
// graph).
func (req Request) AddCheckUpdate(id *idspb.Check) CheckWrite {
	ret := orchestratorpb.WriteNodesRequest_CheckWrite_builder{
		Identifier: id,
	}.Build()
	req.Msg.SetChecks(append(req.Msg.GetChecks(), ret))
	return CheckWrite{ret}
}
