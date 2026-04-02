// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"iter"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// RefSlot indicates which structural 'slot' a ValueRef belongs to for the
// RefsIn* functions.
type RefSlot byte

const (
	// StageArgsSlot indicates the ValueRef is from `Stage.args`.
	StageArgsSlot RefSlot = iota
	// StageLegacyWorkPlanSlot indicates the ValueRef is from
	// `Stage.legacy.worknode`.
	StageLegacyWorkPlanSlot

	// StageEditReasonDetailsSlot indicates the ValueRef is from
	// `Stage.edits.reason.details`.
	StageEditReasonDetailsSlot
	// StageEditAttemptDetailsSlot indicates the ValueRef is from
	// `Stage.edits.stage.attempts.details`.
	StageEditAttemptDetailsSlot

	// StageAttemptDetailsSlot indicates the ValueRef is from
	// `Stage.attempts.details`.
	StageAttemptDetailsSlot
	// StageAttemptProgressDetailsSlot indicates the ValueRef is from
	// `Stage.attempts.progress.details`.
	StageAttemptProgressDetailsSlot

	// CheckOptionsSlot indicates the ValueRef is from `Check.options`.
	CheckOptionsSlot
	// CheckResultsDataSlot indicates the ValueRef is from `Check.results.data`.
	CheckResultsDataSlot
	// CheckEditReasonDetailsSlot indicates the ValueRef is from
	// `Check.edits.reason.details`.
	CheckEditReasonDetailsSlot
	// CheckEditOptionsSlot indicates the ValueRef is from
	// `Check.edits.check.options`.
	CheckEditOptionsSlot
	// CheckEditResultsDataSlot indicates the ValueRef is from
	// `Check.edits.check.results.data`.
	CheckEditResultsDataSlot
)

// RefsInStage is an iterator which iterates through every non-nil *ValueRef
// in the stage.
func RefsInStage(stage *orchestratorpb.Stage) iter.Seq2[RefSlot, *orchestratorpb.ValueRef] {
	return func(yield func(RefSlot, *orchestratorpb.ValueRef) bool) {
		if args := stage.GetArgs(); args != nil && !yield(StageArgsSlot, args) {
			return
		}

		if worknode := stage.GetLegacy().GetWorknode(); worknode != nil && !yield(StageLegacyWorkPlanSlot, worknode) {
			return
		}

		for _, edit := range stage.GetEdits() {
			for _, detail := range edit.GetReason().GetDetails() {
				if detail != nil && !yield(StageEditReasonDetailsSlot, detail) {
					return
				}
			}
			for _, attempt := range edit.GetStage().GetAttempts() {
				for _, detail := range attempt.GetDetails() {
					if detail != nil && !yield(StageEditAttemptDetailsSlot, detail) {
						return
					}
				}
			}
		}

		for _, attempt := range stage.GetAttempts() {
			for slot, ref := range RefsInStageAttempt(attempt) {
				if ref != nil && !yield(slot, ref) {
					return
				}
			}
		}
	}
}

// RefsInStageAttempt is an iterator which iterates through every non-nil
// *ValueRef in the stage attempt.
func RefsInStageAttempt(attempt *orchestratorpb.Stage_Attempt) iter.Seq2[RefSlot, *orchestratorpb.ValueRef] {
	return func(yield func(RefSlot, *orchestratorpb.ValueRef) bool) {
		for _, detail := range attempt.GetDetails() {
			if detail != nil && !yield(StageAttemptDetailsSlot, detail) {
				return
			}
		}
		for _, progress := range attempt.GetProgress() {
			for _, detail := range progress.GetDetails() {
				if detail != nil && !yield(StageAttemptProgressDetailsSlot, detail) {
					return
				}
			}
		}
	}
}

// RefsInCheck is an iterator which iterates through every non-nil
// *ValueRef in the check.
func RefsInCheck(check *orchestratorpb.Check) iter.Seq2[RefSlot, *orchestratorpb.ValueRef] {
	return func(yield func(RefSlot, *orchestratorpb.ValueRef) bool) {
		for _, option := range check.GetOptions() {
			if option != nil && !yield(CheckOptionsSlot, option) {
				return
			}
		}
		for _, result := range check.GetResults() {
			for _, dat := range result.GetData() {
				if dat != nil && !yield(CheckResultsDataSlot, dat) {
					return
				}
			}
		}
		for _, edit := range check.GetEdits() {
			for _, detail := range edit.GetReason().GetDetails() {
				if detail != nil && !yield(CheckEditReasonDetailsSlot, detail) {
					return
				}
			}
			for _, option := range edit.GetCheck().GetOptions() {
				if option != nil && !yield(CheckEditOptionsSlot, option) {
					return
				}
			}
			for _, result := range edit.GetCheck().GetResults() {
				for _, dat := range result.GetData() {
					if dat != nil && !yield(CheckEditResultsDataSlot, dat) {
						return
					}
				}
			}
		}
	}
}

// RefsInWorkplan is an iterator which iterates through every non-nil
// *ValueRef in the WorkPlan.
func RefsInWorkplan(wp *orchestratorpb.WorkPlan) iter.Seq2[RefSlot, *orchestratorpb.ValueRef] {
	return func(yield func(RefSlot, *orchestratorpb.ValueRef) bool) {
		for _, stg := range wp.GetStages() {
			for slot, ref := range RefsInStage(stg) {
				if !yield(slot, ref) {
					return
				}
			}
		}
		for _, chk := range wp.GetChecks() {
			for slot, ref := range RefsInCheck(chk) {
				if !yield(slot, ref) {
					return
				}
			}
		}
	}
}
