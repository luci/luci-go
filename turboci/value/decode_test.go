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
	"cmp"
	"slices"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	t.Run(`ok_inline_binary`, func(t *testing.T) {
		t.Parallel()

		vref := MustInline(structpb.NewStringValue("hi"), "proj:realm")

		sval, err := Decode[*structpb.Value](nil, vref)
		assertNoErr(t, err)

		assertMatch(t, structpb.NewStringValue("hi"), sval)
	})

	t.Run(`ok_source`, func(t *testing.T) {
		t.Parallel()

		vref := MustInline(structpb.NewStringValue("hi"), "proj:realm")

		dSrc := SimpleDataSource{}
		AbsorbInline(dSrc, vref)

		assertTrue(t, vref.HasDigest())

		sval, err := Decode[*structpb.Value](dSrc, vref)
		assertNoErr(t, err)

		assertMatch(t, structpb.NewStringValue("hi"), sval)
	})

	t.Run(`ok_source_json`, func(t *testing.T) {
		t.Parallel()

		vref := MustInline(structpb.NewStringValue("hi"), "proj:realm")

		dSrc := SimpleDataSource{}
		AbsorbAsJSON(dSrc, vref, protojson.MarshalOptions{})

		assertTrue(t, vref.HasDigest())
		assertTrue(t, dSrc.Retrieve(Digest(vref.GetDigest())).HasJson())

		sval, err := Decode[*structpb.Value](dSrc, vref)
		assertNoErr(t, err)

		assertMatch(t, structpb.NewStringValue("hi"), sval)
	})

	t.Run(`missing`, func(t *testing.T) {
		t.Parallel()

		vref := orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Digest:  proto.String("bogus"),
		}.Build()

		dSrc := SimpleDataSource{}

		sval, err := Decode[*structpb.Value](dSrc, vref)
		assertNoErr(t, err)
		assertNil(t, sval)
	})

	t.Run(`mismatch`, func(t *testing.T) {
		t.Parallel()

		vref := orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*emptypb.Empty]()),
			Digest:  proto.String("bogus"),
		}.Build()

		dSrc := SimpleDataSource{}

		_, err := Decode[*structpb.Value](dSrc, vref)
		assertErrLike(t, err, "mismatched types")
	})
}

func TestLookup(t *testing.T) {
	t.Parallel()

	var options []*orchestratorpb.ValueRef

	options, _ = SetByTypeIn(options, MustInline(&emptypb.Empty{}, "proj:realm"))
	options, _ = SetByTypeIn(options, MustInline(structpb.NewStringValue("hey"), "proj:realm"))
	options, _ = SetByTypeIn(options, MustInline(wrapperspb.UInt32(100), "proj:realm"))
	options, _ = SetByTypeIn(options, MustInline(wrapperspb.Bool(true), "proj:realm"))

	dSrc := SimpleDataSource{}
	AbsorbInline(dSrc, options[0]) // bool

	valGot, err := Lookup[*structpb.Value](dSrc, options)
	assertNoErr(t, err)
	assertMatch(t, structpb.NewStringValue("hey"), valGot)

	boolGot, err := Lookup[*wrapperspb.BoolValue](dSrc, options)
	assertNoErr(t, err)
	assertMatch(t, wrapperspb.Bool(true), boolGot)

	missing, err := Lookup[*wrapperspb.StringValue](dSrc, options)
	assertNoErr(t, err)
	assertNil(t, missing)
}

func TestFind(t *testing.T) {
	t.Parallel()

	options := []*orchestratorpb.ValueRef{
		MustInline(&emptypb.Empty{}, "proj:realm"),
		MustInline(structpb.NewStringValue("hey"), "proj:realm"),
		MustInline(wrapperspb.UInt32(100), "proj:realm"),
		MustInline(wrapperspb.Bool(true), "proj:realm"),

		MustInline(&emptypb.Empty{}, "proj:other_realm"),
		MustInline(wrapperspb.UInt32(100), "proj:other_realm"),
	}
	Omit(options[0], orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)
	Omit(options[len(options)-1], orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)

	slices.SortStableFunc(options, func(a, b *orchestratorpb.ValueRef) int {
		return cmp.Compare(a.GetTypeUrl(), b.GetTypeUrl())
	})

	for _, opt := range options {
		t.Log(opt)
	}

	dSrc := SimpleDataSource{}
	AbsorbInline(dSrc, options[0]) // bool

	valGot := Find(options, URL[*structpb.Value]())
	assertMatch(t, MustInline(structpb.NewStringValue("hey"), "proj:realm"), valGot)

	boolGot := Find(options, URL[*wrapperspb.BoolValue]())
	assertMatch(t, options[0], boolGot)

	missing := Find(options, URL[*wrapperspb.StringValue]())
	assertNil(t, missing)

	emptyGot := Find(options, URL[*emptypb.Empty]())
	assertMatch(t, MustInline(&emptypb.Empty{}, "proj:other_realm"), emptyGot)
}

func TestResults(t *testing.T) {
	t.Parallel()

	sortedData := func(refs ...*orchestratorpb.ValueRef) []*orchestratorpb.ValueRef {
		ret := make([]*orchestratorpb.ValueRef, 0, len(refs))
		for _, ref := range refs {
			ok := false
			ret, ok = AddByTypeIn(ret, ref)
			assertTrue(t, ok)
		}
		return ret
	}

	check := orchestratorpb.Check_builder{
		Results: []*orchestratorpb.Check_Result{
			orchestratorpb.Check_Result_builder{
				Data: sortedData(
					MustInline(&emptypb.Empty{}, ""),
					MustInline(wrapperspb.Bool(true), ""),
					MustInline(wrapperspb.String("hey"), ""),
				),
			}.Build(),
			orchestratorpb.Check_Result_builder{
				Data: sortedData(
					MustInline(&emptypb.Empty{}, ""),
					MustInline(wrapperspb.String("norp"), ""),
				),
			}.Build(),
			orchestratorpb.Check_Result_builder{}.Build(),
			orchestratorpb.Check_Result_builder{
				Data: sortedData(
					MustInline(&emptypb.Empty{}, ""),
					MustInline(wrapperspb.Bool(false), ""),
					MustInline(wrapperspb.String("dorp"), ""),
				),
			}.Build(),
		},
	}.Build()

	emptyRslts, err := Results[*emptypb.Empty](nil, check)
	assertNoErr(t, err)
	assertLen(t, emptyRslts, 3)

	boolRslts, err := Results[*wrapperspb.BoolValue](nil, check)
	assertNoErr(t, err)
	assertMatch(t, []*wrapperspb.BoolValue{
		wrapperspb.Bool(true),
		wrapperspb.Bool(false),
	}, boolRslts)

	strResults, err := Results[*wrapperspb.StringValue](nil, check)
	assertNoErr(t, err)
	assertMatch(t, []*wrapperspb.StringValue{
		wrapperspb.String("hey"),
		wrapperspb.String("norp"),
		wrapperspb.String("dorp"),
	}, strResults)

	intResults, err := Results[*wrapperspb.Int32Value](nil, check)
	assertNoErr(t, err)
	assertEmpty(t, intResults)
}
