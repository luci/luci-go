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
	anyID := any(id)
	if wrapped, ok := anyID.(*idspb.Identifier); ok {
		switch typ := wrapped.WhichType(); typ {
		case idspb.Identifier_WorkPlan_case:
			anyID = wrapped.GetWorkPlan()
		case idspb.Identifier_Check_case:
			anyID = wrapped.GetCheck()
		case idspb.Identifier_CheckOption_case:
			anyID = wrapped.GetCheckOption()
		case idspb.Identifier_CheckResult_case:
			anyID = wrapped.GetCheckResult()
		case idspb.Identifier_CheckResultDatum_case:
			anyID = wrapped.GetCheckResultDatum()
		case idspb.Identifier_CheckEdit_case:
			anyID = wrapped.GetCheckEdit()
		case idspb.Identifier_CheckEditOption_case:
			anyID = wrapped.GetCheckEditOption()
		case idspb.Identifier_Stage_case:
			anyID = wrapped.GetStage()
		case idspb.Identifier_StageAttempt_case:
			anyID = wrapped.GetStageAttempt()
		case idspb.Identifier_StageEdit_case:
			anyID = wrapped.GetStageEdit()

		case idspb.Identifier_Type_not_set_case:
			return id, nil

		default:
			panic(fmt.Sprintf("impossible type: %s", typ))
		}
	}

	wp := idspb.WorkPlan_builder{Id: &workPlanID}.Build()

	switch x := anyID.(type) {
	case *idspb.WorkPlan:
		x.SetId(workPlanID)
	case *idspb.Check:
		x.SetWorkPlan(wp)
	case *idspb.CheckOption:
		x.GetCheck().SetWorkPlan(wp)
	case *idspb.CheckResult:
		x.GetCheck().SetWorkPlan(wp)
	case *idspb.CheckResultDatum:
		x.GetResult().GetCheck().SetWorkPlan(wp)
	case *idspb.CheckEdit:
		x.GetCheck().SetWorkPlan(wp)
	case *idspb.CheckEditOption:
		x.GetCheckEdit().GetCheck().SetWorkPlan(wp)
	case *idspb.Stage:
		x.SetWorkPlan(wp)
	case *idspb.StageAttempt:
		x.GetStage().SetWorkPlan(wp)
	case *idspb.StageEdit:
		x.GetStage().SetWorkPlan(wp)
	default:
		panic(fmt.Sprintf("impossible type: %T", x))
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

// CheckOptionErr returns a CheckOption identifier suitable for use with
// WriteNodes without a WorkPlan.
//
// Returns an error if `checkID` is malformed or `optionIdx` is out of range.
func CheckOptionErr(checkID string, optionIdx int) (*idspb.CheckOption, error) {
	cid, err := CheckErr(checkID)
	if err != nil {
		return nil, fmt.Errorf("id.CheckOption: %w", err)
	}
	idx, err := checkIdx("optionIdx", optionIdx)
	if err != nil {
		return nil, fmt.Errorf("id.CheckOption: %w", err)
	}
	return idspb.CheckOption_builder{
		Check: cid,
		Idx:   idx,
	}.Build(), nil
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

// CheckResultDatumErr returns a CheckResultDatum identifier suitable for use
// with WriteNodes without a WorkPlan.
//
// Returns an error if `checkID` is malformed or `resultIdx`/`datumIdx` are out
// of range.
func CheckResultDatumErr(checkID string, resultIdx, datumIdx int) (*idspb.CheckResultDatum, error) {
	rid, err := CheckResultErr(checkID, resultIdx)
	if err != nil {
		return nil, fmt.Errorf("id.CheckResultDatum: %w", err)
	}
	idx, err := checkIdx("datumIdx", datumIdx)
	if err != nil {
		return nil, fmt.Errorf("id.CheckResultDatum: %w", err)
	}
	return idspb.CheckResultDatum_builder{
		Result: rid,
		Idx:    idx,
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

// CheckEditOptionErr returns a CheckEditOption identifier suitable for use with
// WriteNodes without a WorkPlan.
//
// Returns an error if `checkID` is malformed, `ts` is zero, or `optionIdx` is
// out of range.
func CheckEditOptionErr(checkID string, ts time.Time, optionIdx int) (*idspb.CheckEditOption, error) {
	ceid, err := CheckEditErr(checkID, ts)
	if err != nil {
		return nil, fmt.Errorf("id.CheckEditOption: %w", err)
	}
	idx, err := checkIdx("optionIdx", optionIdx)
	if err != nil {
		return nil, fmt.Errorf("id.CheckEditOption: %w", err)
	}
	return idspb.CheckEditOption_builder{
		CheckEdit: ceid,
		Idx:       idx,
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

// Wrap takes any Identifier sub-type and wraps it into a generic
// idspb.Identifier.
func Wrap[Id Identifier](id Id) *idspb.Identifier {
	return wrap(id)
}

func wrap(id any) *idspb.Identifier {
	switch x := id.(type) {
	case nil:
		return nil
	case *idspb.Identifier:
		return x
	case *idspb.WorkPlan:
		return idspb.Identifier_builder{WorkPlan: x}.Build()
	case *idspb.Check:
		return idspb.Identifier_builder{Check: x}.Build()
	case *idspb.CheckOption:
		return idspb.Identifier_builder{CheckOption: x}.Build()
	case *idspb.CheckResult:
		return idspb.Identifier_builder{CheckResult: x}.Build()
	case *idspb.CheckResultDatum:
		return idspb.Identifier_builder{CheckResultDatum: x}.Build()
	case *idspb.CheckEdit:
		return idspb.Identifier_builder{CheckEdit: x}.Build()
	case *idspb.CheckEditOption:
		return idspb.Identifier_builder{CheckEditOption: x}.Build()
	case *idspb.Stage:
		return idspb.Identifier_builder{Stage: x}.Build()
	case *idspb.StageAttempt:
		return idspb.Identifier_builder{StageAttempt: x}.Build()
	case *idspb.StageEdit:
		return idspb.Identifier_builder{StageEdit: x}.Build()
	default:
		panic(fmt.Sprintf("impossible type: %T", x))
	}
}
