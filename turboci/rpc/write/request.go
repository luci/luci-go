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

// Package write contains helpers for manipulating a turboci orchestrator
// WriteNodesRequest proto message.
//
// Everything in this package is entirely optional, but is intended to produce
// more readable/maintainable code than just using the raw generated proto
// message stubs.
//
// Start with [NewRequest].
//
// # Wrapper Types
//
// All exported types in this package are wrappers over a protobuf message type.
//
// Each of these wrappers has a single field `Msg` which is the current state of
// the wrapped message, and each exported type is named after the type that it
// wraps.
//
// Wrappers returned from successful (error==nil) functions and methods in this
// package will never contain a nil `Msg` field.
//
// It is valid (and encouraged) to mix and match helper calls with directly
// manipulating the `Msg` message within the wrapper.
//
// There is no hidden state in any of these wrappers, so constructing one
// explicitly like `Wrapper{Msg: msg}` from an existing pointer is fine.
//
// # Methods
//
// In this package, methods are all named like <Verb><FieldName><Suffix?> where:
//
// <Verb> is one of:
//
//   - Add - Append (and return) a new message to the given field.
//   - Get - Get a helper (e.g. [Request.GetCurrentStage]).
//
// All methods can be called in any order - Getters will return a wrapper for
// the existing field value, if there already is one.
//
// # See Also
//
//   - [go.chromium.org/luci/turboci/check] - Short alias types for CheckState
//     and CheckKind.
//   - [go.chromium.org/luci/turboci/stage] - Short alias types for StageState
//     and StageAttemptState.
//   - [go.chromium.org/luci/turboci/rpc/write/dep] - Helpers for building
//     DependencyGroups and Edges.
package write

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
)

// Request wraps an orchestratorpb.WriteNodesRequest.
//
// This notably provides the helpers to add options and results to the
// WriteNodesRequest.
//
// See package documentation for the behavior of this wrapper type.
type Request struct {
	Msg *orchestratorpb.WriteNodesRequest
}

// NewRequest returns a Request populated with an empty WriteNodesRequest.
func NewRequest() Request {
	return Request{&orchestratorpb.WriteNodesRequest{}}
}

// AddReason adds a reason to the WriteNodesRequest.
func (req Request) AddReason(msg string, details ...proto.Message) (*orchestratorpb.WriteNodesRequest_Reason, error) {
	vals, err := data.ValuesErr(details...)
	if err != nil {
		return nil, fmt.Errorf("write.ReasonAddWithDetails: %w", err)
	}
	ret := orchestratorpb.WriteNodesRequest_Reason_builder{
		Reason:  &msg,
		Details: vals,
	}.Build()
	req.Msg.SetReasons(append(req.Msg.GetReasons(), ret))
	return ret, nil
}
