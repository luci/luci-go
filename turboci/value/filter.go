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
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// AccessCheck is a function provided to [FilterStage], [FilterStageAttempt]
// and [FilterCheck] to allow the caller to indicate when the caller does not
// have access to a particular ValueRef.
type AccessCheck func(realm string) (bool, error)

// Filter is the signature of the function from [ParseFilter].
//
// It expects a RefSlot and a ValueRef, and:
//   - [Omit]s the ref if the user does not have access or if the ref is
//     unwanted.
//   - Returns needsJSON = true if the user wants the data as JSON.
//
// The caller of the filter should consider the ref wanted if it was not
// omitted (i.e. ref.HasOmitReason() == false).
type Filter func(RefSlot, *orchestratorpb.ValueRef) (needsJSON bool, err error)

// filter is a parsed form of [orchestratorpb.ValueFilter].
type filterImpl struct {
	// A reduced version of the ValueMasks in ValueFilter; indicates which slots
	// need data.
	//
	// This will need to be extended to e.g. a bitmask when there are more
	// filterable things in Value (such as tags).
	//
	// Note that 'TYPE' is always wanted (just the TypeURL of the ValueRef).
	vf map[RefSlot]bool
	ti *TypeInfo

	// AccessCheck function; `nil` means "allow all".
	hasAccess AccessCheck
}

func (f *filterImpl) call(slot RefSlot, ref *orchestratorpb.ValueRef) (needsJSON bool, err error) {
	access := true
	if f.hasAccess != nil {
		if access, err = f.hasAccess(ref.GetRealm()); err != nil {
			return
		}
	}
	if !access {
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)
		return
	}
	if !f.vf[slot] {
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
		return
	}

	// At this point we *structurally* want, and can access, ref.
	// See how typeinfo deals with this.
	wanted, needsJSON := f.ti.Wants(ref.GetTypeUrl())
	if !wanted && !needsJSON {
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
	}
	return
}

// ParseFilter returns a usable form of the ValueFilter as a [Filter] function.
//
// If `hasAccess` is nil, access checks are disabled (meaning that no refs will
// be omitted with NO_ACCESS).
//
// See [RefsInStage], [RefsInStageAttempt] and [RefsInCheck] for iterators
// which easily compose with this.
func ParseFilter(vf *orchestratorpb.ValueFilter, hasAccess AccessCheck) (Filter, error) {
	ti, err := ParseTypeInfo(vf.GetTypeInfo())
	if err != nil {
		return nil, err
	}

	vfMap := map[RefSlot]bool{}
	// These two don't currently have a manual control in ValueMask.
	vfMap[StageEditReasonDetailsSlot] = true
	vfMap[CheckEditReasonDetailsSlot] = true

	setVF := func(slot RefSlot, vm orchestratorpb.ValueMask) {
		switch vm {
		case orchestratorpb.ValueMask_VALUE_MASK_VALUE_TYPE:
			vfMap[slot] = true
		}
	}
	setVF(StageArgsSlot, vf.GetStageArgs())
	setVF(StageAttemptDetailsSlot, vf.GetStageAttemptDetails())
	setVF(StageAttemptProgressDetailsSlot, vf.GetStageAttemptProgressDetails())
	setVF(StageEditAttemptDetailsSlot, vf.GetStageEditAttemptDetails())

	setVF(CheckOptionsSlot, vf.GetCheckOptions())
	setVF(CheckResultsDataSlot, vf.GetCheckResultData())
	setVF(CheckEditOptionsSlot, vf.GetCheckEditOptions())
	setVF(CheckEditResultsDataSlot, vf.GetCheckResultData())

	fi := filterImpl{vfMap, ti, hasAccess}

	return fi.call, nil
}
