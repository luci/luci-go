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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	t.Run(`ok_inline_binary`, func(t *testing.T) {
		t.Parallel()

		vref := MustInline(structpb.NewStringValue("hi"), "proj:realm")

		sval, err := Decode[*structpb.Value](nil, vref)
		assert.NoErr(t, err)

		assert.That(t, sval, should.Match(structpb.NewStringValue("hi")))
	})

	t.Run(`ok_source`, func(t *testing.T) {
		t.Parallel()

		vref := MustInline(structpb.NewStringValue("hi"), "proj:realm")

		dSrc := SimpleDataSource{}
		AbsorbInline(dSrc, vref)

		assert.That(t, vref.HasDigest(), should.BeTrue)

		sval, err := Decode[*structpb.Value](dSrc, vref)
		assert.NoErr(t, err)

		assert.That(t, sval, should.Match(structpb.NewStringValue("hi")))
	})

	t.Run(`ok_source_json`, func(t *testing.T) {
		t.Parallel()

		vref := MustInline(structpb.NewStringValue("hi"), "proj:realm")

		dSrc := SimpleDataSource{}
		AbsorbAsJSON(dSrc, vref, protojson.MarshalOptions{})

		assert.That(t, vref.HasDigest(), should.BeTrue)
		assert.That(t, dSrc.Retrieve(Digest(vref.GetDigest())).HasJson(), should.BeTrue)

		sval, err := Decode[*structpb.Value](dSrc, vref)
		assert.NoErr(t, err)

		assert.That(t, sval, should.Match(structpb.NewStringValue("hi")))
	})

	t.Run(`missing`, func(t *testing.T) {
		t.Parallel()

		vref := orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Digest:  proto.String("bogus"),
		}.Build()

		dSrc := SimpleDataSource{}

		sval, err := Decode[*structpb.Value](dSrc, vref)
		assert.NoErr(t, err)
		assert.Loosely(t, sval, should.BeNil)
	})

	t.Run(`mismatch`, func(t *testing.T) {
		t.Parallel()

		vref := orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*emptypb.Empty]()),
			Digest:  proto.String("bogus"),
		}.Build()

		dSrc := SimpleDataSource{}

		_, err := Decode[*structpb.Value](dSrc, vref)
		assert.ErrIsLike(t, err, "mismatched types")
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
	assert.NoErr(t, err)
	assert.That(t, valGot, should.Match(structpb.NewStringValue("hey")))

	boolGot, err := Lookup[*wrapperspb.BoolValue](dSrc, options)
	assert.NoErr(t, err)
	assert.That(t, boolGot, should.Match(wrapperspb.Bool(true)))

	missing, err := Lookup[*wrapperspb.StringValue](dSrc, options)
	assert.NoErr(t, err)
	assert.Loosely(t, missing, should.BeNil)
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
	assert.That(t, valGot, should.Match(MustInline(structpb.NewStringValue("hey"), "proj:realm")))

	boolGot := Find(options, URL[*wrapperspb.BoolValue]())
	assert.That(t, boolGot, should.Match(options[0]))

	missing := Find(options, URL[*wrapperspb.StringValue]())
	assert.Loosely(t, missing, should.BeNil)

	emptyGot := Find(options, URL[*emptypb.Empty]())
	assert.That(t, emptyGot, should.Match(MustInline(&emptypb.Empty{}, "proj:other_realm")))

}
