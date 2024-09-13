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
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

type outputPropertyReservations struct {
	locs resLocations
}

func (o *outputPropertyReservations) reserve(ns string, skip int) {
	o.locs.reserve(ns, "PropertyModifier", skip+1)
}

func (o *outputPropertyReservations) clear() {
	o.locs.clear(nil)
}

var propModifierReservations = outputPropertyReservations{}

// MakePropertyModifier allows your library/module to reserve a section of the
// output properties for itself.
//
// You can use this to obtain a write function (replace contents at namespace)
// and/or a merge function (do proto.Merge on the current contents of that
// namespace). If one of the function pointers is nil, it will be skipped (at
// least one must be non-nil). If both function pointers are provided, their
// types must exactly agree.
//
// Attempting to reserve duplicate namespaces will panic. The namespace refers
// to the top-level property key. It is recommended that:
//   - The `ns` begins with '$'.
//   - The value after the '$' is the canonical Go package name for your
//     library.
//
// You should call this at init()-time like:
//
//	var propWriter func(context.Context, *MyMessage)
//	var propMerger func(context.Context, *MyMessage)
//
//	func init() {
//	  // one of the two function pointers may be nil
//	  MakePropertyModifier("$some/namespace", &propWriter, &propMerger)
//	}
//
// Note that all MakePropertyModifier invocations must happen BEFORE the build
// is Started. Otherwise invoking the returned writer/merger functions will
// panic.
//
// In Go2 this will be less weird:
//
//	type PropertyModifier[T proto.Message] interface {
//	  Write(context.Context, value T) // assigns 'value'
//	  Merge(context.Context, value T) // does proto.Merge(current, value)
//	}
//	func MakePropertyModifier[T proto.Message](ns string) PropertyModifier[T]
func MakePropertyModifier(ns string, writeFnptr, mergeFnptr any) {
	propModifierReservations.reserve(ns, 1)
	writer, merger, _ := getWriteMergerFnValues(true, writeFnptr, mergeFnptr)

	impl := func(args []reflect.Value, op string, opFn func(*outputPropertyState, proto.Message)) []reflect.Value {
		if args[1].IsNil() {
			return nil
		}

		ctx := args[0].Interface().(context.Context)
		msg := args[1].Interface().(proto.Message)

		if st := getState(ctx); st != nil {
			st.excludeCopy(func() bool {
				if prop := st.outputProperties[ns]; prop != nil {
					opFn(prop, msg)
					return true
				}

				panic(errors.Reason(
					"MakePropertyModifier[%s] for namespace %q was created after the current build started: %s",
					op, ns, propModifierReservations.locs.get(ns)).Err())
			})
		} else {
			// noop mode, log incoming property
			val, err := protojson.Marshal(msg)
			if err != nil {
				panic(err)
			}
			logging.Infof(ctx, "%s output property %q: %q", op, ns, string(val))
		}
		return nil
	}

	if writer.Kind() == reflect.Func {
		writer.Set(reflect.MakeFunc(writer.Type(), func(args []reflect.Value) []reflect.Value {
			return impl(args, "writing", (*outputPropertyState).set)
		}))
	}

	if merger.Kind() == reflect.Func {
		merger.Set(reflect.MakeFunc(merger.Type(), func(args []reflect.Value) []reflect.Value {
			return impl(args, "merging", (*outputPropertyState).merge)
		}))
	}
}

func getWriteMergerFnValues(withContext bool, writeFnptr, mergeFnptr any) (writer, merger reflect.Value, msgT protoreflect.Message) {
	if writeFnptr == nil && mergeFnptr == nil {
		panic("at least one of {writeFnptr, mergeFnptr} must be non-nil")
	}

	var msg error
	var typeSig []reflect.Type
	if withContext {
		msg = errors.New("fnptr is not `func[T proto.Message](context.Context, T)`")
		typeSig = []reflect.Type{ctxType, protoMessageType}
	} else {
		msg = errors.New("fnptr is not `func[T proto.Message](T)`")
		typeSig = []reflect.Type{protoMessageType}
	}

	// We assign msgT in both cases in case one of writeFnptr or mergeFnptr is
	// nil; We check that they are the same type when we assert that writer and
	// merger have the same types.
	if writeFnptr != nil {
		writer, msgT = derefFnPtr(msg, writeFnptr, typeSig, nil)
	}
	if mergeFnptr != nil {
		merger, msgT = derefFnPtr(msg, mergeFnptr, typeSig, nil)
	}

	if writeFnptr != nil && mergeFnptr != nil {
		if reflect.TypeOf(writeFnptr) != reflect.TypeOf(mergeFnptr) {
			panic("{writeFnptr, mergeFnptr} types do not match")
		}
	}

	return
}

type outputPropertyState struct {
	mu sync.Mutex

	// The current state of this output property.
	msg proto.Message

	// cached is non-nil when it has an up-to-date serialization of `msg`.
	cached *structpb.Struct
}

func msgIsEmpty(msg proto.Message) bool {
	// see if st.msg is nil, or if it's empty; In either case we return a nil *Struct.
	if msg == nil {
		return true
	}
	isEmpty := true
	msg.ProtoReflect().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		isEmpty = false
		return false // exit on the first callback
	})
	return isEmpty
}

func (st *outputPropertyState) getStructClone() *structpb.Struct {
	if st == nil {
		return nil
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	// see if st.msg is nil, or if it's empty; In either case we return a nil *Struct.
	if msgIsEmpty(st.msg) {
		return nil
	}

	if st.cached == nil {
		json, err := protojson.Marshal(st.msg)
		if err != nil {
			panic(errors.Annotate(err, "marshaling output property").Err())
		}
		st.cached, _ = structpb.NewStruct(nil)
		if err := protojson.Unmarshal(json, st.cached); err != nil {
			panic(errors.Annotate(err, "unmarshaling output property").Err())
		}
	}

	return proto.Clone(st.cached).(*structpb.Struct)
}

func (st *outputPropertyState) set(msg proto.Message) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.cached = nil
	st.msg = proto.Clone(msg)
}

func (st *outputPropertyState) merge(msg proto.Message) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.cached = nil
	if msgIsEmpty(st.msg) {
		st.msg = proto.Clone(msg)
	} else {
		proto.Merge(st.msg, msg)
	}
}
