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

package id

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
)

func checkToken(name, tok string) error {
	if len(tok) == 0 {
		return fmt.Errorf("%s: zero length", name)
	}
	if strings.Contains(tok, ":") {
		return fmt.Errorf(`%s: %q contains ":"`, name, tok)
	}
	return nil
}

func checkIdx(name string, idx int) (*int32, error) {
	if idx <= 0 || idx > math.MaxInt32 {
		return nil, fmt.Errorf("%s: %d must be in [1, max(int32)]", name, idx)
	}
	idx32 := int32(idx)
	return &idx32, nil
}

// SetWorkplanErr will modify `id` to set its WorkPlan id to `workPlan`.
//
// Returns an error if `workPlan` is malformed.
func SetWorkplanErr[Id Identifier](id Id, workPlanID string) (Id, error) {
	if err := checkToken("workPlanID", workPlanID); err != nil {
		return nil, fmt.Errorf("id.SetWorkplan: %w", err)
	}
	wp := idspb.WorkPlan_builder{Id: &workPlanID}.Build()
	_, check, stage := Root(id)
	if check != nil {
		check.SetWorkPlan(wp)
	} else if stage != nil {
		stage.SetWorkPlan(wp)
	}
	return id, nil
}

// SetWorkplan modifies `id`'s WorkPlan, regardless of the type of `id`.
//
// If `id` is an empty Identifier, returns `id` unchanged.
//
// Returns `id`.
// Panics if `workPlan` is malformed (if this is a possibility, use
// SetWorkplanErr instead).
func SetWorkplan[Id Identifier](id Id, workPlan string) Id {
	ret, err := SetWorkplanErr(id, workPlan)
	if err != nil {
		panic(err)
	}
	return ret
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

// CheckErr returns a Check identifier suitable for use with WriteNodes without
// a WorkPlan.
//
// Returns an error if `id` is malformed.
func CheckErr(id string) (*idspb.Check, error) {
	if err := checkToken("id", id); err != nil {
		return nil, fmt.Errorf("id.Check: %w", err)
	}
	return idspb.Check_builder{Id: &id}.Build(), nil
}

// CheckResultErr returns a CheckResult identifier suitable for use with
// WriteNodes without a WorkPlan.
//
// Returns an error if `checkID` is malformed or `resultIdx` is out of range.
func CheckResultErr(checkID string, resultIdx int) (*idspb.CheckResult, error) {
	cid, err := CheckErr(checkID)
	if err != nil {
		return nil, fmt.Errorf("id.CheckResult: %w", err)
	}
	idx, err := checkIdx("resultIdx", resultIdx)
	if err != nil {
		return nil, fmt.Errorf("id.CheckResult: %w", err)
	}
	return idspb.CheckResult_builder{
		Check: cid,
		Idx:   idx,
	}.Build(), nil
}

// CheckEditErr returns a CheckEdit identifier suitable for use with WriteNodes
// without a WorkPlan.
//
// Returns an error if `checkID` is malformed or `ts` is zero.
func CheckEditErr(checkID string, ts time.Time) (*idspb.CheckEdit, error) {
	cid, err := CheckErr(checkID)
	if err != nil {
		return nil, fmt.Errorf("id.CheckEdit: %w", err)
	}
	if ts.IsZero() {
		return nil, errors.New("id.CheckEdit: zero timestamp")
	}
	return idspb.CheckEdit_builder{
		Check:   cid,
		Version: timestamppb.New(ts),
	}.Build(), nil
}

// WorknodeMode is an enum to control the worknode-ness of a Stage.
//
// Used with [StageErr], [StageAttemptErr] and [StageEditErr].
type WorknodeMode int

const (
	StageIsUnknown WorknodeMode = iota
	StageIsWorknode
	StageNotWorknode
)

func (m WorknodeMode) apply(id *idspb.Stage) {
	if id == nil {
		return
	}
	switch m {
	case StageIsWorknode:
		id.SetIsWorknode(true)
	case StageNotWorknode:
		id.SetIsWorknode(false)
	default:
		id.ClearIsWorknode()
	}
}

// StageErr returns a Stage identifier suitable for use with WriteNodes without
// a WorkPlan.
//
// Returns an error if `id` is malformed.
func StageErr(mode WorknodeMode, stageID string) (*idspb.Stage, error) {
	if err := checkToken("stageID", stageID); err != nil {
		return nil, fmt.Errorf("id.Stage: %w", err)
	}
	ret := idspb.Stage_builder{Id: &stageID}.Build()
	mode.apply(ret)
	return ret, nil
}

// StageAttemptErr returns a StageAttempt identifier suitable for use with
// WriteNodes without a WorkPlan.
//
// Returns an error if `stageID` is malformed or `attemptIdx` is out of range.
func StageAttemptErr(mode WorknodeMode, stageID string, attemptIdx int) (*idspb.StageAttempt, error) {
	sid, err := StageErr(mode, stageID)
	if err != nil {
		return nil, fmt.Errorf("id.StageAttempt: %w", err)
	}
	idx, err := checkIdx("attemptIdx", attemptIdx)
	if err != nil {
		return nil, fmt.Errorf("id.StageAttempt: %w", err)
	}
	return idspb.StageAttempt_builder{
		Stage: sid,
		Idx:   idx,
	}.Build(), nil
}

// StageEditErr returns a StageEdit identifier suitable for use with WriteNodes
// without a WorkPlan.
//
// Returns an error if `stageID` is malformed or `ts` is zero.
func StageEditErr(mode WorknodeMode, stageID string, ts time.Time) (*idspb.StageEdit, error) {
	sid, err := StageErr(mode, stageID)
	if err != nil {
		return nil, fmt.Errorf("id.StageEdit: %w", err)
	}
	if ts.IsZero() {
		return nil, errors.New("id.StageEdit: zero timestamp")
	}
	return idspb.StageEdit_builder{
		Stage:   sid,
		Version: timestamppb.New(ts),
	}.Build(), nil
}

// WorkplanErr returns a WorkPlan identifier or an error if it is invalid.
func WorkplanErr(id string) (*idspb.WorkPlan, error) {
	if err := checkToken("workPlanID", id); err != nil {
		return nil, fmt.Errorf("id.Workplan: %w", err)
	}
	return idspb.WorkPlan_builder{Id: &id}.Build(), nil
}

// Workplan returns a WorkPlan identifier in a structured form.
func Workplan(id string) *idspb.WorkPlan {
	return must(WorkplanErr(id))
}

// Check returns a Check identifier suitable for use with WriteNodes (which will
// fill in the WorkPlan id with the WorkPlan id implied by the stage attempt
// token).
//
// Panics if `id` is malformed. If this is a possibility, use [CheckErr] instead.
func Check(id string) *idspb.Check {
	return must(CheckErr(string(id)))
}

// Stage returns a non-WorkNode Stage identifier suitable for use with
// WriteNodes (which will fill in the WorkPlan id with the WorkPlan id implied
// by the stage attempt token).
//
// Panics if `id` is malformed. If this is a possibility, use [StageErr] instead.
func Stage(id string) *idspb.Stage {
	return must(StageErr(StageNotWorknode, id))
}

// StageUnknown returns a Stage identifier suitable for use with WriteNodes
// (which will fill in the WorkPlan id with the WorkPlan id implied by the stage
// attempt token). This has `is_worknode` unset, and can be used with WriteNodes
// which will fill this in based on the args of the stage.
//
// Panics if `id` is malformed. If this is a possibility, use [StageErr] instead.
func StageUnknown(id string) *idspb.Stage {
	return must(StageErr(StageIsUnknown, id))
}

// StageWorkNode returns a WorkNode Stage identifier, suitable for use with
// WriteNodes (which will fill in the WorkPlan id with the WorkPlan id implied
// by the stage attempt token).
//
// Panics if `id` is malformed. If this is a possibility, use [StageErr] instead.
func StageWorkNode(id string) *idspb.Stage {
	return must(StageErr(StageIsWorknode, id))
}
