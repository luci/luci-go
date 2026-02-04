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

package data

import (
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestRedact(t *testing.T) {
	t.Parallel()

	val := Value(structpb.NewStringValue("old"))

	Redact(val)

	assert.That(t, val, should.Match(orchestratorpb.Value_builder{
		Value:   &anypb.Any{TypeUrl: URL[*structpb.Value]()},
		Omitted: orchestratorpb.Value_OMIT_REASON_NO_ACCESS.Enum(),
	}.Build()))
}

func TestFilterUnwanted(t *testing.T) {
	t.Parallel()

	ti, err := ParseTypeInfo(orchestratorpb.TypeInfo_builder{}.Build())
	assert.NoErr(t, err)

	val := Value(&emptypb.Empty{})
	assert.NoErr(t, Filter(val, ti, protojson.MarshalOptions{}))

	assert.That(t, val, should.Match(orchestratorpb.Value_builder{
		Omitted: orchestratorpb.Value_OMIT_REASON_UNWANTED.Enum(),
		Value:   &anypb.Any{TypeUrl: URL[*emptypb.Empty]()},
	}.Build()))
}

func TestFilterNoJSON(t *testing.T) {
	t.Parallel()

	ti, err := ParseTypeInfo(orchestratorpb.TypeInfo_builder{
		Wanted: TypeSetBuilder{}.WithMessages((*emptypb.Empty)(nil)).MustBuild(),
	}.Build())
	assert.NoErr(t, err)

	val := Value(&emptypb.Empty{})
	assert.NoErr(t, Filter(val, ti, protojson.MarshalOptions{}))

	assert.That(t, val, should.Match(Value(&emptypb.Empty{})))
}

func TestFilterJSONKnown(t *testing.T) {
	t.Parallel()

	ti, err := ParseTypeInfo(orchestratorpb.TypeInfo_builder{
		Wanted:        TypeSetBuilder{}.WithMessages((*emptypb.Empty)(nil)).MustBuild(),
		UnknownJsonpb: proto.Bool(true),
		Known:         TypeSetBuilder{}.WithMessages((*emptypb.Empty)(nil)).MustBuild(),
	}.Build())
	assert.NoErr(t, err)

	val := Value(&emptypb.Empty{})
	assert.NoErr(t, Filter(val, ti, protojson.MarshalOptions{}))

	assert.That(t, val, should.Match(Value(&emptypb.Empty{})))
}

func TestFilterJSONUnknown(t *testing.T) {
	t.Parallel()

	ti, err := ParseTypeInfo(orchestratorpb.TypeInfo_builder{
		Wanted:        TypeSetBuilder{}.WithMessages((*emptypb.Empty)(nil)).MustBuild(),
		UnknownJsonpb: proto.Bool(true),
	}.Build())
	assert.NoErr(t, err)

	val := Value(&emptypb.Empty{})
	assert.NoErr(t, Filter(val, ti, protojson.MarshalOptions{}))

	assert.That(t, val, should.Match(orchestratorpb.Value_builder{
		Value:     &anypb.Any{TypeUrl: URL[*emptypb.Empty]()},
		ValueJson: proto.String("{}"),
	}.Build()))
}

type nopResolver struct {
	mt protoreflect.MessageType
}

func (nopResolver) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	return nil, protoregistry.NotFound
}

func (nopResolver) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	return nil, protoregistry.NotFound
}

func (n nopResolver) FindMessageByName(message protoreflect.FullName) (protoreflect.MessageType, error) {
	return n.FindMessageByURL(TypePrefix + string(message))
}

func (n nopResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	if n.mt == nil {
		return nil, protoregistry.NotFound
	}
	return n.mt, nil
}

var _ interface {
	protoregistry.ExtensionTypeResolver
	protoregistry.MessageTypeResolver
} = nopResolver{}

func TestFilterJSONUnknownNoResolver(t *testing.T) {
	t.Parallel()

	ti, err := ParseTypeInfo(orchestratorpb.TypeInfo_builder{
		Wanted:        TypeSetBuilder{}.WithMessages((*emptypb.Empty)(nil)).MustBuild(),
		UnknownJsonpb: proto.Bool(true),
	}.Build())
	assert.NoErr(t, err)

	val := Value(&emptypb.Empty{})
	assert.ErrIsLike(t, Filter(val, ti, protojson.MarshalOptions{
		Resolver: nopResolver{},
	}), protoregistry.NotFound)
}

func TestFilterJSONUnknownWithUnknownFields(t *testing.T) {
	t.Parallel()

	ti, err := ParseTypeInfo(orchestratorpb.TypeInfo_builder{
		Wanted:        TypeSetBuilder{}.WithMessages((*emptypb.Empty)(nil)).MustBuild(),
		UnknownJsonpb: proto.Bool(true),
	}.Build())
	assert.NoErr(t, err)

	val := Value(&emptypb.Empty{})
	val.GetValue().Value = protowire.AppendTag(val.GetValue().Value, 1, protowire.BytesType)
	val.GetValue().Value = protowire.AppendBytes(val.GetValue().Value, []byte("what is up"))
	assert.NoErr(t, Filter(val, ti, protojson.MarshalOptions{
		Resolver: nopResolver{(*emptypb.Empty)(nil).ProtoReflect().Type()},
	}))

	assert.That(t, val, should.Match(orchestratorpb.Value_builder{
		Value:            &anypb.Any{TypeUrl: URL[*emptypb.Empty]()},
		ValueJson:        proto.String("{}"),
		HasUnknownFields: proto.Bool(true),
	}.Build()))
}

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
