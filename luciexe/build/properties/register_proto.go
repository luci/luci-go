// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"context"
	"reflect"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

func protoFromStruct(ctx context.Context, ns string, unknown unknownFieldSetting, s *structpb.Struct, target any) (badExtras bool, err error) {
	jsonBlob, err := protojson.Marshal(s)
	if err != nil {
		return false, errors.Fmt("impossible - could not marshal proto to JSONPB: %w", err)
	}
	opt := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err = opt.Unmarshal(jsonBlob, target.(proto.Message)); err != nil {
		return false, errors.Fmt("protoFromStruct[%T]: %w", target, err)
	}

	// serialize target out twice, once with proto names, and once with json
	// names, and subtract BOTH from `s`. This is because when we use
	// `opt.Unmarshal` it will accept BOTH names for a given field.
	//
	// TODO: It would be REALLY NICE if UnmarshalOptions had a way to return the
	// unrecognized data.
	//
	// TODO: Implement a custom subtraction option for protos?
	mo := protojson.MarshalOptions{}
	toSubtract := make([]*structpb.Struct, 2)
	for i, useProtoNames := range []bool{true, false} {
		mo.UseProtoNames = useProtoNames
		// we re-use jsonBlob to help cut down on allocations.
		raw, err := mo.MarshalAppend(jsonBlob[:0], target.(proto.Message))
		if err != nil {
			return false, errors.Fmt("impossible - could not marshal proto to JSONPB: %w", err)
		}
		toSubtract[i] = &structpb.Struct{}
		if err := protojson.Unmarshal(raw, toSubtract[i]); err != nil {
			return false, errors.Fmt("impossible - could not unmarshal JSON to Struct: %w", err)
		}
	}
	return handleInputLogging(ctx, ns, jsonBlob, unknown, s, toSubtract)
}

var _ inputParser = protoFromStruct

func protoToJSON(useJSONNames bool) func(any) []byte {
	return func(data any) []byte {
		ret, err := protojson.MarshalOptions{
			UseProtoNames: !useJSONNames,
		}.Marshal(data.(proto.Message))
		if err != nil {
			panic(err)
		}
		return ret
	}
}

var protoToVisibleFields = map[reflect.Type]stringset.Set{}
var protoToVisibleFieldsMu sync.Mutex

func protoVisibleFieldsOf(typ reflect.Type) stringset.Set {
	protoToVisibleFieldsMu.Lock()
	defer protoToVisibleFieldsMu.Unlock()

	ret, ok := protoToVisibleFields[typ]
	if ok {
		return ret
	}

	desc := reflect.New(typ.Elem()).Interface().(proto.Message).ProtoReflect().Descriptor()

	fields := desc.Fields()

	ret = stringset.New(fields.Len())
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		ret.Add(f.TextName())
		ret.Add(f.JSONName())
	}

	protoToVisibleFields[typ] = ret

	return ret
}
