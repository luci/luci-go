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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
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

func derefFnPtr(perr error, fnptr any, in, out []reflect.Type) (fn reflect.Value, msgT protoreflect.Message) {
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

	return fn, reflect.New(protoType.Elem()).Interface().(proto.Message).ProtoReflect()
}
