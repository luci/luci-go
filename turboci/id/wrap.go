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
	"fmt"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
)

// StageIdentifier is the union of all valid Identifier messages which are
// rooted via a Stage in the graph.
type StageIdentifier interface {
	*idspb.Identifier |
		*idspb.Stage |
		*idspb.StageAttempt |
		*idspb.StageEdit |
		*idspb.StageEditReason
}

// CheckIdentifier is the union of all valid Identifier messages which are
// rooted via a Check in the graph.
type CheckIdentifier interface {
	*idspb.Identifier |
		*idspb.Check |
		*idspb.CheckOption |
		*idspb.CheckResult |
		*idspb.CheckResultDatum |
		*idspb.CheckEdit |
		*idspb.CheckEditReason |
		*idspb.CheckEditOption
}

// Identifier is the union of all valid Identifier messages.
type Identifier interface {
	*idspb.Identifier |
		*idspb.WorkPlan |
		StageIdentifier |
		CheckIdentifier
}

// Wrap takes any Identifier sub-type and wraps it into a generic
// idspb.Identifier.
func Wrap[Id Identifier](id Id) *idspb.Identifier {
	return wrap(id)
}

// wrap takes any Identifier type and returns it as an idspb.Identifier.
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
	case *idspb.CheckEditReason:
		return idspb.Identifier_builder{CheckEditReason: x}.Build()
	case *idspb.CheckEditOption:
		return idspb.Identifier_builder{CheckEditOption: x}.Build()
	case *idspb.Stage:
		return idspb.Identifier_builder{Stage: x}.Build()
	case *idspb.StageAttempt:
		return idspb.Identifier_builder{StageAttempt: x}.Build()
	case *idspb.StageEdit:
		return idspb.Identifier_builder{StageEdit: x}.Build()
	case *idspb.StageEditReason:
		return idspb.Identifier_builder{StageEditReason: x}.Build()
	}

	panic(fmt.Sprintf("bug: id.wrap got unexpected type %T", id))
}

// unwraps any Identifier into it's *specific* identifier type.
//
// Allows generic functions to be written with a simpler type switch.
func unwrap(id any) any {
	if id == nil {
		return nil
	}

	switch x := id.(type) {
	case *idspb.Identifier:
		switch x.WhichType() {
		case idspb.Identifier_WorkPlan_case:
			return x.GetWorkPlan()
		case idspb.Identifier_Check_case:
			return x.GetCheck()
		case idspb.Identifier_CheckOption_case:
			return x.GetCheckOption()
		case idspb.Identifier_CheckResult_case:
			return x.GetCheckResult()
		case idspb.Identifier_CheckResultDatum_case:
			return x.GetCheckResultDatum()
		case idspb.Identifier_CheckEdit_case:
			return x.GetCheckEdit()
		case idspb.Identifier_CheckEditOption_case:
			return x.GetCheckEditOption()
		case idspb.Identifier_CheckEditReason_case:
			return x.GetCheckEditReason()
		case idspb.Identifier_Stage_case:
			return x.GetStage()
		case idspb.Identifier_StageAttempt_case:
			return x.GetStageAttempt()
		case idspb.Identifier_StageEdit_case:
			return x.GetStageEdit()
		case idspb.Identifier_StageEditReason_case:
			return x.GetStageEditReason()
		case idspb.Identifier_Type_not_set_case:
			return nil
		}

	case *idspb.WorkPlan,
		*idspb.Stage,
		*idspb.StageAttempt,
		*idspb.StageEdit,
		*idspb.StageEditReason,
		*idspb.Check,
		*idspb.CheckOption,
		*idspb.CheckResult,
		*idspb.CheckResultDatum,
		*idspb.CheckEdit,
		*idspb.CheckEditReason,
		*idspb.CheckEditOption:

		return x
	}

	panic(fmt.Sprintf("bug: id.unwrap got unexpected type %T", id))
}

// StageRoot returns the *Stage for any StageIdentifier.
//
// If the Stage portion of the identifier is `nil`, this returns `nil`.
func StageRoot[Id StageIdentifier](id Id) *idspb.Stage {
	_, _, stage := Root(id)
	return stage
}

// CheckRoot returns the *Check for any CheckIdentifier.
//
// If the Stage portion of the identifier is `nil`, this returns `nil`.
func CheckRoot[Id CheckIdentifier](id Id) *idspb.Check {
	_, check, _ := Root(id)
	return check
}

// Root returns either the Check or Stage which serves as the root for the
// specific id, plus the WorkPlan.
//
// For *idspb.WorkPlan (or a wrapped WorkPlan), this just returns `wp`.
//
// If the check/stage portion of `id` is nil, this returns nil.
// If the WorkPlan portion of `id` is nil, this returns nil.
func Root[Id Identifier](id Id) (wp *idspb.WorkPlan, check *idspb.Check, stage *idspb.Stage) {
	switch x := unwrap(id).(type) {
	case *idspb.WorkPlan:
		wp = x

	// Stage root
	case *idspb.Stage:
		stage = x
	case *idspb.StageAttempt:
		stage = x.GetStage()
	case *idspb.StageEdit:
		stage = x.GetStage()
	case *idspb.StageEditReason:
		stage = x.GetStageEdit().GetStage()

	// Check root
	case *idspb.Check:
		check = x
	case *idspb.CheckOption:
		check = x.GetCheck()
	case *idspb.CheckResult:
		check = x.GetCheck()
	case *idspb.CheckResultDatum:
		check = x.GetResult().GetCheck()
	case *idspb.CheckEdit:
		check = x.GetCheck()
	case *idspb.CheckEditReason:
		check = x.GetCheckEdit().GetCheck()
	case *idspb.CheckEditOption:
		check = x.GetCheckEdit().GetCheck()
	case nil:
		// nil identifiers have check, stage and workplan all-nil.

	default:
		panic(fmt.Sprintf("bug: id.Root got unexpected type %T", id))
	}

	// Fill in WorkPlan from the stage/check.
	if stage != nil {
		wp = stage.GetWorkPlan()
	} else if check != nil {
		wp = check.GetWorkPlan()
	}

	return
}

// KindOf describes any type of Identifier as an IdentifierKind enum.
func KindOf[Id Identifier](id Id) idspb.IdentifierKind {
	switch typ := unwrap(id); typ {
	case idspb.Identifier_WorkPlan_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_WORK_PLAN
	case idspb.Identifier_Check_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK
	case idspb.Identifier_CheckOption_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_OPTION
	case idspb.Identifier_CheckResult_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_RESULT
	case idspb.Identifier_CheckResultDatum_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_RESULT_DATUM
	case idspb.Identifier_CheckEdit_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_EDIT
	case idspb.Identifier_CheckEditReason_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_EDIT_REASON
	case idspb.Identifier_CheckEditOption_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_EDIT_OPTION
	case idspb.Identifier_Stage_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_STAGE
	case idspb.Identifier_StageAttempt_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_STAGE_ATTEMPT
	case idspb.Identifier_StageEdit_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_STAGE_EDIT
	case idspb.Identifier_StageEditReason_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_EDIT_REASON

	case idspb.Identifier_Type_not_set_case:
		return idspb.IdentifierKind_IDENTIFIER_KIND_UNKNOWN

	default:
		panic(fmt.Sprintf("impossible type: %s", typ))
	}
}
