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

package value

import orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

// Filter is a parsed form of [orchestratorpb.ValueFilter].
type Filter struct {
	vf *orchestratorpb.ValueFilter
	ti *TypeInfo
}

// ParseFilter returns a [Filter] ready to use with [FilterStage],
// [FilterStageAttempt] and [FilterCheck].
func ParseFilter(vf *orchestratorpb.ValueFilter) (*Filter, error) {
	ti, err := ParseTypeInfo(vf.GetTypeInfo())
	if err != nil {
		return nil, err
	}
	return &Filter{vf, ti}, nil
}

// AccessCheck is a function provided to [FilterStage], [FilterStageAttempt]
// and [FilterCheck] to allow the caller to indicate when the caller does not
// have access to a particular ValueRef.
type AccessCheck func(realm string) (bool, error)

type filterState struct {
	hasAccess AccessCheck
	ti        *TypeInfo

	res FilterResult
	err error
}

func (s *filterState) filterRef(ref *orchestratorpb.ValueRef, vm orchestratorpb.ValueMask) {
	if s.err != nil || ref == nil {
		return
	}

	access, err := s.hasAccess(ref.GetRealm())
	if err != nil {
		s.err = err
		return
	}
	if !access {
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)
		return
	}

	switch vm {
	case orchestratorpb.ValueMask_VALUE_MASK_UNKNOWN, orchestratorpb.ValueMask_VALUE_MASK_TYPE:
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
		return
	}

	// At this point we *structurally* want, and can access, ref.
	// See how typeinfo deals with this.
	wantBinary, wantJSON := s.ti.Wants(ref.GetTypeUrl())
	if !wantBinary && !wantJSON {
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)
		return
	}

	if ref.HasDigest() {
		s.res.WantedDigests = append(s.res.WantedDigests, ref.GetDigest())
	}
	if wantJSON {
		s.res.WantedJSON = append(s.res.WantedJSON, ref)
	}
}

func (s *filterState) result() (FilterResult, error) {
	if s.err != nil {
		return FilterResult{}, s.err
	}
	return s.res, nil
}

// FilterResult is the result type for [FilterStage], [FilterStageAttempt] and
// [FilterCheck].
//
// The caller should:
//   - Fetch all `WantedDigests` into some [DataSource].
//   - Call [AbsorbAsJSON] on all `WantedJSON` entries.
type FilterResult struct {
	// WantedDigests are the digests which need to be fetched into some [DataSource]
	// in order for the ValueRef's in the filtered object to be decodable.
	WantedDigests []string

	// WantedJSON are the set of ValueRef's which need to be [AbsorbAsJSON]'d.
	//
	// This should be done after populating the [DataSource] with everything in
	// `WantedDigests`, since these may refer to data by digest.
	WantedJSON []*orchestratorpb.ValueRef
}

// FilterStage walks the Stage and mutates all ValueRefs according to the
// following rules:
//   - If `hasAccess` returns false, the ref is [Omit]'d as NO_ACCESS.
//   - If `vf` does not want the ref, it is [Omit]'d as UNWANTED.
//
// Otherwise the ref is 'wanted', and:
//   - If the ValueRef has a digest, the digest is added to `WantedDigests`.
//   - If the ValueRef is wanted as JSON, it is added to `WantedJSON`.
//
// Note: BOTH of these rules may apply to the same ref.
//
// If `hasAccess` returns an error, it will be returned by `FilterStage`.
func FilterStage(stage *orchestratorpb.Stage, vf *Filter, hasAccess AccessCheck) (FilterResult, error) {
	if vf == nil {
		vf = &Filter{}
	}
	fs := filterState{
		hasAccess: hasAccess,
		ti:        vf.ti,
	}

	fs.filterRef(stage.GetArgs(), vf.vf.GetStageArgs())

	return fs.result()
}

// FilterCheck walks the Check and mutates all ValueRefs according to the
// following rules:
//   - If `hasAccess` returns false, the ref is [Omit]'d as NO_ACCESS.
//   - If `vf` does not want the ref, it is [Omit]'d as UNWANTED.
//
// Otherwise the ref is 'wanted', and:
//   - If the ValueRef has a digest, the digest is added to `WantedDigests`.
//   - If the ValueRef is wanted as JSON, it is added to `WantedJSON`.
//
// The caller should then:
//   - Fetch all `WantedDigests` into some [DataSource].
//   - Call [AbsorbAsJSON] on all `WantedJSON` entries.
//
// Note: BOTH of these rules may apply to the same ref.
//
// If `hasAccess` returns an error, it will be returned by `FilterCheck`.
func FilterCheck(check *orchestratorpb.Check, vf *Filter, hasAccess AccessCheck) (FilterResult, error) {
	if vf == nil {
		vf = &Filter{}
	}
	fs := filterState{
		hasAccess: hasAccess,
		ti:        vf.ti,
	}

	// Nothing yet.

	return fs.result()
}
