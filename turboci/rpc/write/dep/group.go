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

// Package dep has functions and types for composing
// WriteNodesRequest.DependencyGroup messages for the TurboCI WriteNodes RPC
// call.
package dep

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/proto/delta"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

type (
	// Diff is a unit which can update a WriteNodesRequest_DependencyGroup.
	Diff = delta.Diff[*orchestratorpb.WriteNodesRequest_DependencyGroup]

	// Builder is the protobuf Builder type to use with [Template].
	//
	// Unless you are implementing helpers inside of turboci/rpc/write, you can
	// ignore this.
	Builder = orchestratorpb.WriteNodesRequest_DependencyGroup_builder
)

// Template is an exported delta.Template to make *dep.Diff.
//
// Unless you are implementing helpers inside of turboci/rpc/write, you can
// ignore this.
//
// See also:
//   - [go.chromium.org/luci/turboci/rpc/write/check.Edge]
//   - [go.chromium.org/luci/turboci/rpc/write/stage.Edge]
var Template = delta.MakeTemplate[Builder](nil)

// Group returns a Diff for a WriteNodesRequest.DependencyGroup which adds
// a new Group to the DependencyGroup which contains all `diffs`.
//
// See also:
//   - [go.chromium.org/luci/turboci/rpc/write/check.Edge]
//   - [go.chromium.org/luci/turboci/rpc/write/stage.Edge]
//   - [Threshold]
func Group(diffs ...*Diff) *Diff {
	dg, err := delta.Collect(diffs...)
	return Template.New(Builder{
		Groups: []*orchestratorpb.WriteNodesRequest_DependencyGroup{
			dg,
		},
	}, err)
}

// Threshold returns a Diff for a WriteNodesRequest.DependencyGroup which sets
// the threshold.
//
// See [Group].
func Threshold(threshold int) *Diff {
	var err error
	if threshold < 0 {
		err = fmt.Errorf("dep.Threshold: must be positive")
	}
	ret := Template.New(Builder{
		Threshold: proto.Int32(int32(threshold)),
	}, err)
	return ret
}
