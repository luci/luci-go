// Copyright 2023 The LUCI Authors.
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

// Package data holds data manipulation functions.
//
// Most functionality should be put into a subpackage of data, but it's
// acceptable to have small, common, functions here which don't fit into
// a subpackage.
package data

import (
	"reflect"
)

// LosslessConvertTo will attempt to convert a go value into a given type losslessly.
//
// We only want to perform conversions which are ALWAYS lossless, which means:
//   - untyped nil -> interface, pointer, slice
//   - intX to intY where X <= Y
//   - uintX to uintY where X <= Y
//   - uintX to intY where X < Y
//   - floatX to floatY where X <= Y
//   - complexX to complexY where X <= Y
//   - string to (string, []byte, []rune)
//   - all other conversions implemented by reflect.Value.Convert.
//
// Other conversions to string allowed by reflect.Type.ConvertibleTo are
// disallowed, because it allows e.g. int->string, but in the sense of
// string(rune(1938)), which is almost certainly NOT what you want.
// ConvertibleTo also allows lossy conversions (e.g. int64->int8).
//
// If you arrive here because Assert(t, something, Comparison[else]) worked in
// a surprising way, you probably need to add more conditions to this function.
//
// Note the "ALWAYS lossless" condition is important - we do not want to allow
// int64(100) -> int8 even though this hasn't lost data. This is because if the
// values change, this function will start returning ok=false. We want this
// conversion to ALWAYS fail with a type conversion error, requiring the caller
// author to change the input/output types explicitly.
func LosslessConvertTo[T any](value any) (T, bool) {
	var ret T
	ok := LosslessConvertToReflect(value, reflect.ValueOf(&ret).Elem())
	return ret, ok
}

// LosslessConvertToReflect is the same as LosslessConvertTo, except that it
// takes a set-able reflect.Value rather than directly returning the converted
// value.
//
// This will panic if `to` is not settable.
func LosslessConvertToReflect(value any, to reflect.Value) (ok bool) {
	if !to.CanSet() {
		panic("LosslessConvertToReflect: to is not settable")
	}

	if value == nil {
		switch to.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Chan, reflect.Slice,
			reflect.Map, reflect.Func:
			// just keep `to` as the default value
			return true
		}
		// else: nil -> * is not allowed
		return false
	}

	from := reflect.ValueOf(value)
	fromT, toT := from.Type(), to.Type()
	fromK, toK := fromT.Kind(), toT.Kind()

	switch {
	case toK == reflect.Interface:
		ok := fromT.ConvertibleTo(toT)
		if ok {
			to.Set(from.Convert(toT))
		}
		return ok

	case fromK >= reflect.Int && fromK <= reflect.Int64:
		if toK >= reflect.Int && toK <= reflect.Int64 && toT.Bits() >= fromT.Bits() {
			to.SetInt(from.Int())
			return true
		}

	case fromK >= reflect.Uint && fromK <= reflect.Uint64:
		switch {
		case toK >= reflect.Uint && toK <= reflect.Uint64 && toT.Bits() >= fromT.Bits():
			to.SetUint(from.Uint())
			return true
		case toK >= reflect.Int && toK <= reflect.Int64 && toT.Bits() > fromT.Bits():
			to.SetInt(int64(from.Uint()))
			return true
		}

	case fromK >= reflect.Float32 && fromK <= reflect.Float64:
		if toK >= reflect.Float32 && toK <= reflect.Float64 && toT.Bits() >= fromT.Bits() {
			to.SetFloat(from.Float())
			return true
		}

	case fromK >= reflect.Complex64 && fromK <= reflect.Complex128:
		if toK >= reflect.Complex64 && toK <= reflect.Complex128 && toT.Bits() >= fromT.Bits() {
			to.SetComplex(from.Complex())
			return true
		}

	case fromK == reflect.String:
		switch {
		case toK == reflect.String:
			to.SetString(from.String())
			return true

		case toK == reflect.Slice && toT.Elem().Kind() == reflect.Uint8:
			// []byte
			to.SetBytes([]byte(from.String()))
			return true

		case toK == reflect.Slice && toT.Elem().Kind() == reflect.Int32:
			// []rune - we use Convert because reflect has a fancy internal
			// implementation for setRunes that we can't use :/
			to.Set(from.Convert(toT))
			return true
		}

	case fromT.ConvertibleTo(toT):
		// We rely on the default conversion rules for all other target types.
		to.Set(from.Convert(toT))
		return true

	}

	return false
}
