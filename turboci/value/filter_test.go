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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// Tests [filterState.filterRef] by virtue of Stage.Args.
func TestFilterRef(t *testing.T) {
	t.Parallel()

	makeRef := func(t testing.TB, src DataSource, msg proto.Message) *orchestratorpb.ValueRef {
		t.Helper()

		ret := MustInline(msg, "proj:realm")

		if src != nil {
			AbsorbInline(src, ret)
		}

		return ret
	}

	vf := orchestratorpb.ValueFilter_builder{
		TypeInfo: orchestratorpb.TypeInfo_builder{
			// want google.protobuf.*
			Wanted:        TypeSetBuilder{}.WithPackagesOf((*structpb.Value)(nil)).MustBuild(),
			UnknownJsonpb: proto.Bool(true),
			// know google.protobuf.Value
			Known: TypeSetBuilder{}.WithMessages((*structpb.Value)(nil)).MustBuild(),
		}.Build(),
		StageArgs: orchestratorpb.ValueMask_VALUE_MASK_VALUE_TYPE.Enum(),
	}.Build()

	filter, err := ParseFilter(vf)
	assertNoErr(t, err)

	t.Run(`want_binary_inline`, func(t *testing.T) {
		ref := makeRef(t, nil, structpb.NewBoolValue(true))
		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason(0), ref.GetOmitReason())
		assertFalse(t, wantJSON)
	})

	t.Run(`want_binary_remote`, func(t *testing.T) {
		mSrc := SimpleDataSource{}
		ref := makeRef(t, mSrc, structpb.NewBoolValue(true))
		dgst := "nP03LSTuMLuLfYp94hWnwHOj2kT2Pg_DikrWVQk2tJ4vAQ"

		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, dgst, ref.GetDigest())
		assertFalse(t, wantJSON)
	})

	t.Run(`want_json_inline`, func(t *testing.T) {
		lst, err := structpb.NewList([]any{true})
		assertNoErr(t, err)
		ref := makeRef(t, nil, lst)

		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason(0), ref.GetOmitReason())
		assertTrue(t, wantJSON)
	})

	t.Run(`want_json_remote`, func(t *testing.T) {
		mSrc := SimpleDataSource{}
		lst, err := structpb.NewList([]any{true})
		assertNoErr(t, err)

		ref := makeRef(t, mSrc, lst)
		dgst := "TiL2hG12z5bCnO-q4sXjaMqObIM7ZeZNAYcHd56bTRE1AQ"

		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason(0), ref.GetOmitReason())
		assertTrue(t, wantJSON)

		assertEqual(t, dgst, ref.GetDigest())
	})

	t.Run(`want_no_access`, func(t *testing.T) {
		ref := makeRef(t, nil, structpb.NewBoolValue(true))

		filter, err := ParseFilter(vf)
		assertNoErr(t, err)

		wantJSON, err := filter.Apply(StageArgsSlot, ref, func(realm string) (bool, error) {
			return false, nil
		})
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS, ref.GetOmitReason())
		assertFalse(t, wantJSON)

		assertFalse(t, ref.HasDigest())
		assertFalse(t, ref.HasInline())
	})

	t.Run(`unwant_structural_inline`, func(t *testing.T) {
		vf := proto.CloneOf(vf)
		vf.ClearStageArgs()

		filter, err := ParseFilter(vf)
		assertNoErr(t, err)

		ref := makeRef(t, nil, structpb.NewBoolValue(true))

		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED, ref.GetOmitReason())
		assertFalse(t, wantJSON)

		assertEqual(t, "nP03LSTuMLuLfYp94hWnwHOj2kT2Pg_DikrWVQk2tJ4vAQ", ref.GetDigest())
		assertFalse(t, ref.HasInline())
	})

	t.Run(`unwant_structural_remote`, func(t *testing.T) {
		vf := proto.CloneOf(vf)
		vf.ClearStageArgs()

		filter, err := ParseFilter(vf)
		assertNoErr(t, err)

		mSrc := SimpleDataSource{}

		ref := makeRef(t, mSrc, structpb.NewBoolValue(true))

		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED, ref.GetOmitReason())
		assertFalse(t, wantJSON)

		assertEqual(t, "nP03LSTuMLuLfYp94hWnwHOj2kT2Pg_DikrWVQk2tJ4vAQ", ref.GetDigest())
		assertFalse(t, ref.HasInline())
	})

	t.Run(`unwant_type_inline`, func(t *testing.T) {
		ref := makeRef(t, nil, structpb.NewBoolValue(true))
		ref.SetTypeUrl(TypePrefix + "bogus.namespace.Message")
		ref.GetInline().TypeUrl = TypePrefix + "bogus.namespace.Message"

		wantJSON, err := filter.Apply(StageArgsSlot, ref, nil)
		assertNoErr(t, err)
		assertEqual(t, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED, ref.GetOmitReason())
		assertFalse(t, wantJSON)

		assertEqual(t, "hvSVT6KdvPHO0-h55_J5by3wAe3u5ymMnl0ColX35QkxAQ", ref.GetDigest())
		assertFalse(t, ref.HasInline())
	})

	t.Run(`auth_error`, func(t *testing.T) {
		ref := makeRef(t, nil, structpb.NewBoolValue(true))

		filter, err := ParseFilter(vf)
		assertNoErr(t, err)

		_, err = filter.Apply(StageArgsSlot, ref, func(realm string) (bool, error) {
			return false, errors.New("oh no auth exploded")
		})
		assertErrLike(t, err, "oh no auth exploded")
	})
}
