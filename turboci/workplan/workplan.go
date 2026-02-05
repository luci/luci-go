// Copyright 2026 The LUCI Authors.
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

package workplan

import (
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/id"
)

// flatStageID is a version of idspb.Stage which can be used as a map key.
type flatStageID struct {
	workplan   string
	isWorknode bool
	stageId    string
}

// makeFlatStageID produces a flatStageID from an idspb.Stage.
func makeFlatStageID(sid *idspb.Stage) flatStageID {
	return flatStageID{
		workplan:   sid.GetWorkPlan().GetId(),
		isWorknode: sid.GetIsWorknode(),
		stageId:    sid.GetId(),
	}
}

// flatCheckID is a version of idspb.Check which can be used as a map key.
type flatCheckID struct {
	workplan string
	checkID  string
}

// makeFlatCheckID produces a flatCheckID from an idspb.Check.
func makeFlatCheckID(cid *idspb.Check) flatCheckID {
	return flatCheckID{
		workplan: cid.GetWorkPlan().GetId(),
		checkID:  cid.GetId(),
	}
}

// NodeBag is an alternate form of []*WorkPlan which indexes checks and stages
// directly by their ID.
type NodeBag struct {
	checks map[flatCheckID]*orchestratorpb.Check
	stages map[flatStageID]*orchestratorpb.Stage
}

// ToNodeBag composes a NodeBag from zero or more WorkPlans.
//
// Providing no workplans (or empty workplans) produces an empty, but
// functional, NodeBag.
func ToNodeBag(workplans ...*orchestratorpb.WorkPlan) *NodeBag {
	checkCount := 0
	stageCount := 0
	for _, wp := range workplans {
		checkCount += len(wp.GetChecks())
		stageCount += len(wp.GetStages())
	}
	// even if we have no nodes, we return a functional NodeBag.
	ret := &NodeBag{
		checks: make(map[flatCheckID]*orchestratorpb.Check, checkCount),
		stages: make(map[flatStageID]*orchestratorpb.Stage, stageCount),
	}
	// TODO: When Update exists, use that to ensure that workplans are correctly
	// merged together.
	for _, wp := range workplans {
		for _, check := range wp.GetChecks() {
			ret.checks[makeFlatCheckID(check.GetIdentifier())] = check
		}
		for _, stage := range wp.GetStages() {
			ret.stages[makeFlatStageID(stage.GetIdentifier())] = stage
		}
	}
	return ret
}

// TODO: Implement Update which ingests another WorkPlan of some version, and
// updates this NodeBag with new data within.

// RootFor returns the root check or stage which equals (or contains) `id`.
//
// Returns (nil, nil) if the identifier's Root is not in this NodeBag.
func (n *NodeBag) RootFor(ident *idspb.Identifier) (*orchestratorpb.Check, *orchestratorpb.Stage) {
	_, checkRoot, stageRoot := id.Root(ident)
	if checkRoot != nil {
		return n.Check(checkRoot), nil
	}
	if stageRoot != nil {
		return nil, n.Stage(stageRoot)
	}
	return nil, nil
}

// Stage returns the Stage identified by `sid`, or `nil` if it does not
// exist in this NodeBag.
func (n *NodeBag) Stage(sid *idspb.Stage) *orchestratorpb.Stage {
	return n.stages[makeFlatStageID(sid)]
}

// StageAttempt returns the Stage Attempt identified by `aid`, or `nil` if it
// does not exist in this NodeBag.
func (n *NodeBag) StageAttempt(aid *idspb.StageAttempt) *orchestratorpb.Stage_Attempt {
	attempts := n.Stage(aid.GetStage()).GetAttempts()
	idx := int(aid.GetIdx()) - 1
	if idx >= len(attempts) {
		return nil
	}
	return attempts[idx]
}

// Check returns the Check identified by `cid`, or `nil` if it does not exist.
func (n *NodeBag) Check(cid *idspb.Check) *orchestratorpb.Check {
	return n.checks[makeFlatCheckID(cid)]
}
