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
	"reflect"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

func protoFromStruct(unknownFields unknownFieldSetting) func(s *structpb.Struct, target any) error {
	return func(s *structpb.Struct, target any) error {
		jsonBlob, err := protojson.Marshal(s)
		if err != nil {
			return errors.Annotate(err, "protoFromStruct[%T]", target).Err()
		}
		opt := protojson.UnmarshalOptions{
			DiscardUnknown: unknownFields == ignoreUnknownFields,
		}
		return errors.Annotate(
			opt.Unmarshal(jsonBlob, target.(proto.Message)), "protoFromStruct[%T]", target).Err()
	}
}

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
