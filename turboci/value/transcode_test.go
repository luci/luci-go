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
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestHasUnknownFields(t *testing.T) {
	t.Parallel()

	t.Run(`basic`, func(t *testing.T) {
		t.Parallel()

		buf := protowire.AppendTag(nil, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, 12345)

		e := &emptypb.Empty{}

		assert.NoErr(t, proto.Unmarshal(buf, e))

		assert.That(t, hasUnknownFields(e.ProtoReflect()), should.BeTrue)
	})

	t.Run(`list`, func(t *testing.T) {
		t.Parallel()

		// inner is a structpb.Value w/ number_value(1.234) plus 20:varint(100)
		inner := protowire.AppendTag(nil, 2, protowire.Fixed64Type)
		inner = protowire.AppendFixed64(inner, math.Float64bits(1.234))
		inner = protowire.AppendTag(inner, 20, protowire.VarintType)
		inner = protowire.AppendVarint(inner, 100)

		// outer is a structpb.List
		outer := protowire.AppendTag(nil, 1, protowire.BytesType)
		outer = protowire.AppendBytes(outer, inner)

		l := &structpb.ListValue{}
		assert.NoErr(t, proto.Unmarshal(outer, l))

		newList, err := structpb.NewList([]any{1.234})
		assert.NoErr(t, err)
		assert.That(t, l, should.Match(newList, protocmp.IgnoreUnknown()))

		assert.That(t, hasUnknownFields(l.ProtoReflect()), should.BeTrue)
	})

	t.Run(`map`, func(t *testing.T) {
		t.Parallel()

		// inner is a structpb.Value w/ number_value(1.234) plus 20:varint(100)
		inner := protowire.AppendTag(nil, 2, protowire.Fixed64Type)
		inner = protowire.AppendFixed64(inner, math.Float64bits(1.234))
		inner = protowire.AppendTag(inner, 20, protowire.VarintType)
		inner = protowire.AppendVarint(inner, 100)

		// mapVal is the implied map entry message
		mapVal := protowire.AppendTag(nil, 1, protowire.BytesType)
		mapVal = protowire.AppendBytes(mapVal, []byte("key"))
		mapVal = protowire.AppendTag(mapVal, 2, protowire.BytesType)
		mapVal = protowire.AppendBytes(mapVal, inner)

		// outer is a structpb.Struct
		outer := protowire.AppendTag(nil, 1, protowire.BytesType)
		outer = protowire.AppendBytes(outer, mapVal)

		s := &structpb.Struct{}
		assert.NoErr(t, proto.Unmarshal(outer, s))

		newStruct, err := structpb.NewStruct(map[string]any{"key": 1.234})
		assert.NoErr(t, err)
		assert.That(t, s, should.Match(newStruct, protocmp.IgnoreUnknown()))

		assert.That(t, hasUnknownFields(s.ProtoReflect()), should.BeTrue)
	})

	t.Run(`nested`, func(t *testing.T) {
		t.Parallel()

		// inner is a structpb.Value w/ number_value(1.234) plus 20:varint(100)
		inner := protowire.AppendTag(nil, 2, protowire.Fixed64Type)
		inner = protowire.AppendFixed64(inner, math.Float64bits(1.234))
		inner = protowire.AppendTag(inner, 20, protowire.VarintType)
		inner = protowire.AppendVarint(inner, 100)

		// mapVal is the implied map entry message
		mapVal := protowire.AppendTag(nil, 1, protowire.BytesType)
		mapVal = protowire.AppendBytes(mapVal, []byte("key"))
		mapVal = protowire.AppendTag(mapVal, 2, protowire.BytesType)
		mapVal = protowire.AppendBytes(mapVal, inner)

		// structVal is a structpb.Struct
		structVal := protowire.AppendTag(nil, 1, protowire.BytesType)
		structVal = protowire.AppendBytes(structVal, mapVal)

		// outer is a Value with the struct field set
		outer := protowire.AppendTag(nil, 5, protowire.BytesType)
		outer = protowire.AppendBytes(outer, structVal)

		v := &structpb.Value{}
		assert.NoErr(t, proto.Unmarshal(outer, v))

		newStruct, err := structpb.NewStruct(map[string]any{"key": 1.234})
		assert.NoErr(t, err)

		val := structpb.NewStructValue(newStruct)
		assert.That(t, v, should.Match(val, protocmp.IgnoreUnknown()))

		assert.That(t, hasUnknownFields(v.ProtoReflect()), should.BeTrue)
	})
}

func TestEnsureJSONInSource(t *testing.T) {
	t.Parallel()

	t.Run(`ok`, func(t *testing.T) {
		t.Parallel()

		dSrc := SimpleDataSource{}
		v := mustInline(structpb.NewStringValue("hi"), "proj:realm")

		assert.NoErr(t, AbsorbAsJSON(dSrc, v, protojson.MarshalOptions{}))

		assert.That(t, dSrc.Retrieve(v.GetDigest()).GetJson(), should.Match(orchestratorpb.ValueData_JsonAny_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Value:   proto.String(`"hi"`),
		}.Build()))
	})

	t.Run(`already_absorbed`, func(t *testing.T) {
		t.Parallel()

		dSrc := SimpleDataSource{}
		v := mustInline(structpb.NewStringValue("hi"), "proj:realm")

		AbsorbInline(dSrc, v)

		assert.NoErr(t, AbsorbAsJSON(dSrc, v, protojson.MarshalOptions{}))

		assert.That(t, dSrc.Retrieve(v.GetDigest()).GetJson(), should.Match(orchestratorpb.ValueData_JsonAny_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Value:   proto.String(`"hi"`),
		}.Build()))
	})

	t.Run(`already_json`, func(t *testing.T) {
		t.Parallel()

		dSrc := SimpleDataSource{}
		v := mustInline(structpb.NewStringValue("hi"), "proj:realm")

		assert.NoErr(t, AbsorbAsJSON(dSrc, v, protojson.MarshalOptions{}))

		// should be a no-op
		assert.NoErr(t, AbsorbAsJSON(dSrc, v, protojson.MarshalOptions{}))

		assert.That(t, dSrc.Retrieve(v.GetDigest()).GetJson(), should.Match(orchestratorpb.ValueData_JsonAny_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Value:   proto.String(`"hi"`),
		}.Build()))
	})

	t.Run(`not_in_registry`, func(t *testing.T) {
		t.Parallel()

		dSrc := SimpleDataSource{}
		v := orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*emptypb.Empty]()),
			Digest:  proto.String("superfake"),
		}.Build()

		assert.ErrIsLike(t, AbsorbAsJSON(dSrc, v, protojson.MarshalOptions{}), "missing data")
	})

	t.Run(`unknown_fields`, func(t *testing.T) {
		t.Parallel()

		dSrc := SimpleDataSource{}
		v := mustInline(&emptypb.Empty{}, "proj:realm")

		raw, err := proto.Marshal(structpb.NewStringValue("hi"))
		assert.NoErr(t, err)
		v.GetInline().Value = raw

		assert.NoErr(t, AbsorbAsJSON(dSrc, v, protojson.MarshalOptions{}))

		assert.That(t, dSrc.Retrieve(v.GetDigest()).GetJson(), should.Match(orchestratorpb.ValueData_JsonAny_builder{
			TypeUrl:          proto.String(URL[*emptypb.Empty]()),
			Value:            proto.String("{}"),
			HasUnknownFields: proto.Bool(true),
		}.Build()))
	})
}
