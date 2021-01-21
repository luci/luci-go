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
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

type resLocations struct {
	mu sync.Mutex
	// locations maps reserved namespace to the source location where they were
	// first reserved.
	locations map[string]string
}

func (r *resLocations) snap() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ret := make(map[string]string, len(r.locations))
	for k, v := range r.locations {
		ret[k] = v
	}
	return ret
}

// skip is the number of frames to skip from your callsite.
//
// skip=1 means "your caller"
func (r *resLocations) reserve(ns string, kind string, skip int) {
	if ns == "" {
		panic(errors.New("empty namespace not allowed"))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if current, ok := r.locations[ns]; ok {
		panic(
			errors.Reason("cannot reserve %s namespace %q: already reserved by %s",
				kind, ns, current).Err())
	}

	reservationLocation := "<unknown>"
	if _, file, line, ok := runtime.Caller(skip + 1); ok {
		reservationLocation = fmt.Sprintf("\"%s:%d\"", file, line)
	}

	if r.locations == nil {
		r.locations = map[string]string{}
	}
	r.locations[ns] = reservationLocation
}

func (r *resLocations) each(cb func(ns string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ns := range r.locations {
		cb(ns)
	}
}

func (r *resLocations) clear(cb func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locations = nil
	if cb != nil {
		cb()
	}
}

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

type outputPropertyReservations struct {
	locs resLocations
}

func (o *outputPropertyReservations) reserve(ns string, skip int) {
	o.locs.reserve(ns, "PropertyModifier", skip+1)
}

func (o *outputPropertyReservations) clear() {
	o.locs.clear(nil)
}

var (
	propReaderReservations   = inputPropertyReservations{}
	propModifierReservations = outputPropertyReservations{}

	ctxType          = reflect.TypeOf((*context.Context)(nil)).Elem()
	protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()
	errorType        = reflect.TypeOf((*error)(nil)).Elem()
)

func cmpArgsProtoT(perr error, expected []reflect.Type, actual func(int) reflect.Type) (ret reflect.Type) {
	for i, ex := range expected {
		cur := actual(i)
		if ex == protoMessageType {
			if !cur.Implements(ex) {
				panic(perr)
			}
			ret = cur
		} else if cur != ex {
			panic(perr)
		}
	}
	return
}

func derefFnPtr(perr error, fnptr interface{}, in, out []reflect.Type) (fn reflect.Value, mkMsg func() proto.Message) {
	val := reflect.ValueOf(fnptr)
	if val.Kind() != reflect.Ptr {
		panic(perr)
	}

	fn = val.Elem()
	var protoType reflect.Type

	fnT := fn.Type()
	if fnT.Kind() != reflect.Func {
		panic(perr)
	}

	if fnT.NumIn() != len(in) {
		panic(perr)
	}
	if pT := cmpArgsProtoT(perr, in, fnT.In); pT != nil {
		protoType = pT
	}

	if fnT.NumOut() != len(out) {
		panic(perr)
	}
	if pT := cmpArgsProtoT(perr, out, fnT.Out); pT != nil {
		protoType = pT
	}

	return fn, func() proto.Message {
		return reflect.New(protoType.Elem()).Interface().(proto.Message)
	}
}

// getReaderFnValue returns the reflect.Value of the underlying function in the
// pointer-to-function `fntptr`, as well as a function to construct `fnptr`'s
// concrete proto.Message return type.
func getReaderFnValue(fnptr interface{}) (reflect.Value, func() proto.Message) {
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

// MakePropertyReader allows your library/module to reserve a section of the
// input properties for itself.
//
// Attempting to reserve duplicate namespaces will panic. The namespace refers
// to the top-level property key. It is recommended that:
//   * The `ns` begins with '$'.
//   * The value after the '$' is the canonical Go package name for your
//     library.
//
// Using the generated function will parse the relevant input property namespace
// as JSONPB, returning the parsed message (and an error, if any).
//
//   var myPropertyReader func(context.Context) *MyPropertyMsg
//   func init() {
//     MakePropertyReader("$some/namespace", &myPropertyReader)
//   }
//
// In Go2 this will be less weird:
//   MakePropertyReader[T proto.Message](ns string) func(context.Context) T
func MakePropertyReader(ns string, fnptr interface{}) {
	fn, mkMsg := getReaderFnValue(fnptr)
	propReaderReservations.reserve(ns, mkMsg, 1)

	fn.Set(reflect.MakeFunc(fn.Type(), func(args []reflect.Value) []reflect.Value {
		cstate := getState(args[0].Interface().(context.Context))
		var msg proto.Message

		if st := cstate.state; st != nil && st.reservedInputProperties[ns] != nil {
			msg = proto.Clone(st.reservedInputProperties[ns])
		} else {
			msg = mkMsg().ProtoReflect().Type().Zero().Interface()
		}

		return []reflect.Value{reflect.ValueOf(msg)}
	}))
}

func getWriteMergerFnValues(writeFnptr, mergeFnptr interface{}) (writer, merger reflect.Value) {
	if writeFnptr == nil && mergeFnptr == nil {
		panic("at least one of {writeFnptr, mergeFnptr} must be non-nil")
	}

	msg := errors.New("fnptr is not `func[T proto.Message](context.Context, T)`")

	if writeFnptr != nil {
		writer, _ = derefFnPtr(msg, writeFnptr, []reflect.Type{ctxType, protoMessageType}, nil)
	}
	if mergeFnptr != nil {
		merger, _ = derefFnPtr(msg, mergeFnptr, []reflect.Type{ctxType, protoMessageType}, nil)
	}

	if writeFnptr != nil && mergeFnptr != nil {
		if reflect.TypeOf(writeFnptr) != reflect.TypeOf(mergeFnptr) {
			panic("{writeFnptr, mergeFnptr} types do not match")
		}
	}

	return
}

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
//   * The `ns` begins with '$'.
//   * The value after the '$' is the canonical Go package name for your
//     library.
//
// You should call this at init()-time like:
//
//   var propWriter func(context.Context, *MyMessage)
//   var propMerger func(context.Context, *MyMessage)
//
//   func init() {
//     // one of the two function pointers may be nil
//     MakePropertyModifier("$some/namespace", &propWriter, &propMerger)
//   }
//
// In Go2 this will be less weird:
//   type PropertyModifier[T proto.Message] interface {
//     Write(context.Context, value T) // assigns 'value'
//     Merge(context.Context, value T) // does proto.Merge(current, value)
//   }
//   func MakePropertyModifier[T proto.Message](ns string) PropertyModifier[T]
func MakePropertyModifier(ns string, writeFnptr, mergeFnptr interface{}) {
	propModifierReservations.reserve(ns, 1)
	writer, merger := getWriteMergerFnValues(writeFnptr, mergeFnptr)

	if writer.Kind() == reflect.Func {
		writer.Set(reflect.MakeFunc(writer.Type(), func(args []reflect.Value) []reflect.Value {
			ctx := args[0].Interface().(context.Context)
			cstate := getState(ctx)
			if args[1].IsNil() {
				return nil
			}
			msg := args[1].Interface().(proto.Message)

			if cstate.state == nil {
				// noop mode, log incoming property
				val, err := protojson.Marshal(msg)
				if err != nil {
					panic(err)
				}
				logging.Infof(ctx, "writing output property %q: %q", ns, string(val))
			} else {
				panic("not implemented")
			}

			return nil
		}))
	}

	if merger.Kind() == reflect.Func {
		merger.Set(reflect.MakeFunc(merger.Type(), func(args []reflect.Value) []reflect.Value {
			ctx := args[0].Interface().(context.Context)
			cstate := getState(ctx)
			if args[1].IsNil() {
				return nil
			}
			msg := args[1].Interface().(proto.Message)

			if cstate.state == nil {
				// noop mode, log incoming property
				val, err := protojson.Marshal(msg)
				if err != nil {
					panic(err)
				}
				logging.Infof(ctx, "merging output property %q: %q", ns, string(val))
			} else {
				panic("not implemented")
			}

			return nil
		}))
	}
}
