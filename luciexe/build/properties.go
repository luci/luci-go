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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

type namespaceReservations struct {
	mu           sync.Mutex
	reservations map[string]string
}

var reservations = map[string]*namespaceReservations{
	"PropertyReader":   {reservations: map[string]string{}},
	"PropertyModifier": {reservations: map[string]string{}},
}

var (
	ctxType          = reflect.TypeOf((*context.Context)(nil)).Elem()
	protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()
	errorType        = reflect.TypeOf((*error)(nil)).Elem()
)

func reserveNamespace(ns string, kind string) {
	if ns == "" {
		panic(errors.New("empty namespace not allowed"))
	}

	r, ok := reservations[kind]
	if !ok {
		panic(errors.New("invalid reservation pool"))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if current, ok := r.reservations[ns]; ok {
		panic(
			errors.Reason("cannot reserve %s namespace %q: already reserved by %s",
				kind, ns, current).Err())
	}

	reservationLocation := "<unknown>"
	if _, file, line, ok := runtime.Caller(1); ok {
		reservationLocation = fmt.Sprintf("\"%s:%d\"", file, line)
	}

	r.reservations[ns] = reservationLocation
}

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

func derefFnPtr(perr error, fnptr interface{}, in, out []reflect.Type) (fn reflect.Value, msgFn func() proto.Message) {
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
		errors.New("fnptr is not `func[T proto.Message](context.Context) (T, error)`"),
		fnptr,
		[]reflect.Type{ctxType},
		[]reflect.Type{protoMessageType, errorType},
	)
}

func errorValue(err error) reflect.Value {
	if err == nil {
		return reflect.Zero(errorType)
	}
	return reflect.ValueOf(err)
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
//   var myPropertyReader func(context.Context) (*MyPropertyMsg, error)
//   func init() {
//     MakePropertyReader("$some/namespace", &myPropertyReader)
//   }
//
// In Go2 this will be less weird:
//   type PropertyReader[T proto.Message] func(context.Context) (T, error)
//   MakePropertyReader[T proto.Message](ns string) PropertyReader[T]
func MakePropertyReader(ns string, fnptr interface{}) {
	reserveNamespace(ns, "PropertyReader")

	fn, msgFn := getReaderFnValue(fnptr)
	fn.Set(reflect.MakeFunc(fn.Type(), func(args []reflect.Value) []reflect.Value {
		cstate := getState(args[0].Interface().(context.Context))
		msg := msgFn()
		var err error

		if cstate.state == nil {
			// noop mode, return empty message and nil error
		} else {
			panic("implement")
		}

		return []reflect.Value{reflect.ValueOf(msg), errorValue(err)}
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
	reserveNamespace(ns, "PropertyModifier")
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
