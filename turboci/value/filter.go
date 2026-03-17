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
	"errors"
	"iter"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// AccessCheck is a function provided to [FilterStage], [FilterStageAttempt]
// and [FilterCheck] to allow the caller to indicate when the caller does not
// have access to a particular ValueRef.
type AccessCheck func(realm string) (bool, error)

// Filter is a stateful, parsed, form of [orchestratorpb.ValueFilter].
type Filter struct {
	// A reduced version of the ValueMasks in ValueFilter; indicates which slots
	// need data.
	//
	// This will need to be extended to e.g. a bitmask when there are more
	// filterable things in Value (such as tags).
	//
	// Note that 'TYPE' is always wanted (just the TypeURL of the ValueRef).
	vf        map[RefSlot]bool
	ti        *TypeInfo
	hasAccess AccessCheck
}

// FilterResult is the return result of [Filter.Apply].
type FilterResult struct {
	// NeedFetch contains ValueRefs which are desired and accessible (via
	// `hasAccess`), but have non-inlined data which needs to be fetched.
	//
	// These are mapped by digest -> all ValueRefs which refer to that digest.
	NeedFetch map[string][]*orchestratorpb.ValueRef

	// NeedJSON are ValueRefs which need to be converted to JSON.
	//
	// This may overlap with `NeedFetch` in the case where a digest-based
	// ValueRef needs to be converted to JSON.
	NeedJSON []*orchestratorpb.ValueRef
}

// Apply applies the parsed ValueFilter w/ AccessCheck function to all refs
// yielded by the iterator.
//
// This will:
//   - Omit refs which the user does not have access to as NO_ACCESS.
//   - Omit refs which the user does not want (either via ValueFilter masks,
//     or via ValueFilter.TypeInfo) as UNWANTED.
//   - Return the set of wanted digest-based ValueRefs, collated by digest,
//     plus the list of wanted ValueRefs which need JSON conversion.
//
// See [RefsInStage], [RefsInStageAttempt] and [RefsInCheck] for iterators
// which easily compose with this.
func (f *Filter) Apply(i iter.Seq2[RefSlot, *orchestratorpb.ValueRef]) (*FilterResult, error) {
	ret := &FilterResult{NeedFetch: make(map[string][]*orchestratorpb.ValueRef)}

	var errs []error
	for slot, ref := range i {
		access, err := f.hasAccess(ref.GetRealm())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !access {
			Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)
			continue
		}
		if !f.vf[slot] {
			Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
			continue
		}

		// At this point we *structurally* want, and can access, ref.
		// See how typeinfo deals with this.
		wantBinary, wantJSON := f.ti.Wants(ref.GetTypeUrl())
		if !wantBinary && !wantJSON {
			Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
			continue
		}

		if dgst := ref.GetDigest(); dgst != "" {
			ret.NeedFetch[dgst] = append(ret.NeedFetch[dgst], ref)
		}
		if wantJSON {
			ret.NeedJSON = append(ret.NeedJSON, ref)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return ret, nil
}

// ParseFilter returns a [Filter] ready to use with [FilterStage],
// [FilterStageAttempt] and [FilterCheck].
//
// If `hasAccess` is nil, access checks are disabled (meaning that no refs will
// be omitted with NO_ACCESS).
func ParseFilter(vf *orchestratorpb.ValueFilter, hasAccess AccessCheck) (*Filter, error) {
	ti, err := ParseTypeInfo(vf.GetTypeInfo())
	if err != nil {
		return nil, err
	}
	if hasAccess == nil {
		hasAccess = func(realm string) (bool, error) { return true, nil }
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

	return &Filter{vfMap, ti, hasAccess}, nil
}
