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

	"go.chromium.org/luci/common/proto/delta"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write/check"
)

// NewCheck returns a Diff which adds a new CheckWrite
// composed of the provided CheckWrite diffs.
//
// This should be used when the caller believes this check does not already
// exist in the graph. `kind` is required to write all new checks.
func NewCheck(checkID string, kind check.Kind, diffs ...*check.Diff) *Diff {
	cw := &orchestratorpb.WriteNodesRequest_CheckWrite{}
	var errs []error
	cid, err := id.CheckErr(checkID)
	if err != nil {
		errs = append(errs, fmt.Errorf("write.NewCheck.checkID: %w", err))
	}
	cw.SetIdentifier(cid)
	cw.SetKind(kind)
	if err := delta.CollectInto(cw, diffs...); err != nil {
		errs = append(errs, fmt.Errorf("write.NewCheck: %w", err))
	}
	return template.New(builder{
		Checks: []*orchestratorpb.WriteNodesRequest_CheckWrite{cw},
	}, errs...)
}

// UpdateCheck returns a Diff which adds a new CheckWrite composed of the
// provided CheckWrite diffs for an EXISTING Check.
//
// This should be used when the caller believes this check already exists in the
// graph - preferably, the caller should have obtained the check id via
// a previous QueryNodes call.
func UpdateCheck(cid *idspb.Check, diffs ...*check.Diff) *Diff {
	cw := &orchestratorpb.WriteNodesRequest_CheckWrite{}
	var errs []error
	cw.SetIdentifier(cid)
	if err := delta.CollectInto(cw, diffs...); err != nil {
		errs = append(errs, fmt.Errorf("write.UpdateCheck: %w", err))
	}
	return template.New(builder{
		Checks: []*orchestratorpb.WriteNodesRequest_CheckWrite{cw},
	}, errs...)
}
