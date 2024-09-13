// Copyright 2020 The LUCI Authors.
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

package build

import (
	"context"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
)

type inputPropertyReservations struct {
	locs         resLocations
	msgFactories map[string]func() proto.Message
}

func (i *inputPropertyReservations) reserve(ns string, mkMsg func() proto.Message, skip int) {
	i.locs.reserve(ns, "PropertyReader", skip+1)
	if i.msgFactories == nil {
		i.msgFactories = map[string]func() proto.Message{}
	}
	i.msgFactories[ns] = mkMsg
}

func (i *inputPropertyReservations) each(cb func(ns string, mkMsg func() proto.Message)) {
	i.locs.each(func(ns string) {
		cb(ns, i.msgFactories[ns])
	})
}

func (i *inputPropertyReservations) clear() {
	i.locs.clear(func() {
		i.msgFactories = nil
	})
}

var propReaderReservations = inputPropertyReservations{}

// MakePropertyReader allows your library/module to reserve a section of the
// input properties for itself.
//
// Attempting to reserve duplicate namespaces will panic. The namespace refers
// to the top-level property key. It is recommended that:
//   - The `ns` begins with '$'.
//   - The value after the '$' is the canonical Go package name for your
//     library.
//
// Using the generated function will parse the relevant input property namespace
// as JSONPB, returning the parsed message (and an error, if any).
//
//	var myPropertyReader func(context.Context) *MyPropertyMsg
//	func init() {
//	  MakePropertyReader("$some/namespace", &myPropertyReader)
//	}
//
// In Go2 this will be less weird:
//
//	MakePropertyReader[T proto.Message](ns string) func(context.Context) T
func MakePropertyReader(ns string, fnptr any) {
	fn, msgT := getReaderFnValue(fnptr)
	mkMsg := func() proto.Message {
		return msgT.New().Interface()
	}
	propReaderReservations.reserve(ns, mkMsg, 1)

	fn.Set(reflect.MakeFunc(fn.Type(), func(args []reflect.Value) []reflect.Value {
		st := getState(args[0].Interface().(context.Context))
		var msg proto.Message

		if st != nil && st.reservedInputProperties[ns] != nil {
			msg = proto.Clone(st.reservedInputProperties[ns])
		} else {
			msg = mkMsg().ProtoReflect().Type().Zero().Interface()
		}

		return []reflect.Value{reflect.ValueOf(msg)}
	}))
}

// getReaderFnValue returns the reflect.Value of the underlying function in the
// pointer-to-function `fntptr`, as well as a function to construct `fnptr`'s
// concrete proto.Message return type.
func getReaderFnValue(fnptr any) (reflect.Value, protoreflect.Message) {
	return derefFnPtr(
		errors.New("fnptr is not `func[T proto.Message](context.Context) T`"),
		fnptr,
		[]reflect.Type{ctxType},
		[]reflect.Type{protoMessageType},
	)
}

func parseReservedInputProperties(props *structpb.Struct, strict bool) (map[string]proto.Message, error) {
	ret := map[string]proto.Message{}
	merr := errors.MultiError{}

	propReaderReservations.each(func(ns string, mkMsg func() proto.Message) {
		propVal := props.GetFields()[ns]
		if propVal == nil {
			return
		}

		json, err := protojson.Marshal(propVal)
		if err != nil {
			panic(err) // this should be impossible
		}

		msg := mkMsg()
		unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: !strict}
		if err = unmarshaler.Unmarshal(json, msg); err != nil {
			merr = append(merr, errors.Annotate(err, "deserializing input property %q", ns).Err())
			return
		}

		ret[ns] = msg
	})

	var err error
	if len(merr) > 0 {
		err = merr
	}

	return ret, err
}

func parseTopLevelProperties(props *structpb.Struct, strict bool, dst proto.Message) error {
	dstR := dst.ProtoReflect()
	reservedLocs := propReaderReservations.locs.snap()

	// first check if `reserved` overlaps with the fields in `dst`
	fields := dstR.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		for _, conflict := range []string{field.TextName(), field.JSONName()} {
			if loc := reservedLocs[conflict]; loc != "" {
				return errors.Reason(
					"use of top-level property message %T conflicts with MakePropertyReader(ns=%q) reserved at: %s",
					dst, conflict, loc).Err()
			}
		}
	}

	// next, clone `props` and remove all fields which have been parsed
	props = proto.Clone(props).(*structpb.Struct)
	for ns := range reservedLocs {
		delete(props.GetFields(), ns)
	}

	json, err := protojson.Marshal(props)
	if err != nil {
		panic(err) // this should be impossible
	}

	return protojson.UnmarshalOptions{DiscardUnknown: !strict}.Unmarshal(json, dst)
}
