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

package should

import (
	"reflect"
	"sync"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/typed"
)

// should.Match needs to keep a cache of type -> []cmp.Option for
// AllowUnexporteds. This is faster in packages which do many tests over the
// same types.
//
// Note that during extractAllowUnexportedFromLocked we intentionally insert
// `nil` into resembleOptionCache so that when walking recursive types we don't
// recurse infinitely. By the time that resembleOptionCacheMu is released, all
// nil entries in resembleOptionCache will be replaced with []cmp.Option if
// there were any options to add, or it will remain nil in the event that no
// structs with unexported fields were discovered. Either way, the next call to
// extractAllowUnexportedFrom/extractAllowUnexportedFromLocked will get a cache
// hit.
var resembleOptionCacheMu sync.RWMutex
var resembleOptionCache = map[reflect.Type][]cmp.Option{}

// resetOptionCache is only called from tests inside the `should` package
// itself.
func resetOptionCache() {
	resembleOptionCacheMu.Lock()
	defer resembleOptionCacheMu.Unlock()
	resembleOptionCache = map[reflect.Type][]cmp.Option{}
}

// We want to be able to ignore any struct fields of type *Something where this
// is a proto message.
var protoMessageType = reflect.TypeFor[proto.Message]()

// extractAllowUnexportedFrom returns a list of cmp.Option which include cmp.AllowUnexported
// options for all struct types which include unexported fields which are
// reachable from `typ`.
//
// The result of this function is cached behind an RWMutex, so the second call
// to this for the same type should be fast.
func extractAllowUnexportedFrom(typ reflect.Type) []cmp.Option {
	if typ == nil {
		return nil
	}

	resembleOptionCacheMu.RLock()
	val, ok := resembleOptionCache[typ]
	resembleOptionCacheMu.RUnlock()
	if ok {
		return val
	}

	resembleOptionCacheMu.Lock()
	defer resembleOptionCacheMu.Unlock()
	return extractAllowUnexportedFromLocked(typ)
}

// extractAllowUnexportedFromLocked checks the cache for a set of
// cmp.AllowUnexported in the cache, and if it's missing, computes it.
func extractAllowUnexportedFromLocked(typ reflect.Type) []cmp.Option {
	// First, check the cache.
	if val, ok := resembleOptionCache[typ]; ok {
		// could be returning []cmp.Option or nil, if something higher in the stack
		// is already working on `typ`, in the case of a recursive type.
		return val
	}

	switch typ.Kind() {
	case reflect.Struct:
		// only structs may have unexported fields. cmp.AllowUnexported will fail
		// for any types that are not Struct.
		var ret []cmp.Option

		// mark this entry in the cache as being worked on - this allows recursive
		// types to compute successfully.
		resembleOptionCache[typ] = nil

		hasUnexported := false
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !hasUnexported && !field.IsExported() {
				// It looks like this struct, itself, has unexported fields, so we at
				// least need an AllowUnexported for ourself.
				ret = append(ret, cmp.AllowUnexported(reflect.New(typ).Elem().Interface()))
				hasUnexported = true
			}
			// We also will want to include AllowUnexporteds for any sub-types implied
			// by this field.
			ret = append(ret, extractAllowUnexportedFromLocked(field.Type)...)
		}

		// actually populate the entry now
		resembleOptionCache[typ] = ret
		return ret

	case reflect.Pointer:
		// proto messages are explicitly handled by the default cmp.Transform from
		// the protocmp package so we do not want to recurse here.
		if typ.Implements(protoMessageType) {
			return nil
		}

		fallthrough
	case reflect.Slice, reflect.Array, reflect.Chan:
		// Chan is included here for completness, but I doubt that you can
		// cmp.Diff one of them with any manner of success.
		return extractAllowUnexportedFromLocked(typ.Elem())

	case reflect.Map:
		// note that key types can be `struct` so we need to explore them as well.
		return append(extractAllowUnexportedFromLocked(typ.Key()), extractAllowUnexportedFromLocked(typ.Elem())...)
	}

	// all other types don't need any special treatment
	return nil
}

// Resemble returns a comparison.Func which checks if the actual value 'resembles'
// `expected`, but implicitly compares unexported fields for compatibility with
// goconvey's ShouldResemble check.
//
// Deprecated: Use should.Match instead, which does not automatically
// add AllowUnexported(expected). should.Match also allows specifying your own
// auxilliary cmp.Options globally and locally, for more advanced interaction
// with cmp.Diff.
//
// Semblance is computed with "github.com/google/go-cmp/cmp", and this
// function automatically adds the following cmp.Options to match behavior of
// coconvey:
//   - "google.golang.org/protobuf/testing/protocmp".Transform()
//   - A direct comparison of protoreflect.Descriptor types. These are
//     documented as being comparable with `==`, but by default `cmp` will
//     recurse into their guts.
//   - A direct comparison of reflect.Type interfaces.
//   - "github.com/google/go-cmp/cmp".AllowUnexported(expected) (if `expected`
//     has an underlying struct type after peeling off slices, pointers, etc.)
//
// It is recommended that you use should.Equal when comparing primitive types.
func Resemble[T any](expected T) comparison.Func[T] {
	return matchImpl(
		"should.Match", expected, extractAllowUnexportedFrom(reflect.TypeOf(expected)))
}

// NotResemble returns a comparison.Func which checks if the actual value
// doesn't 'resemble' `expected`, but implicitly compares unexported fields for
// compatibility with goconvey's ShouldNotResemble check.
//
// Deprecated: Use should.NotMatch instead, which does not automatically
// add AllowUnexported(expected). should.NotMatch also allows specifying your
// own/ auxilliary cmp.Options globally and locally, for more advanced
// interaction with cmp.Diff.
//
// Semblance is computed with "github.com/google/go-cmp/cmp", and this
// function automatically adds the following cmp.Options to match behavior of
// coconvey:
//   - "google.golang.org/protobuf/testing/protocmp".Transform()
//   - A direct comparison of protoreflect.Descriptor types. These are
//     documented as being comparable with `==`, but by default `cmp` will
//     recurse into their guts.
//   - A direct comparison of reflect.Type interfaces.
//   - "github.com/google/go-cmp/cmp".AllowUnexported(expected) (if `expected`
//     has an underlying struct type after peeling off slices, pointers, etc.)
//
// It is recommended that you use should.NotEqual when comparing primitive types.
func NotResemble[T any](expected T) comparison.Func[T] {
	return notMatchImpl(
		"should.NotResemble", expected, extractAllowUnexportedFrom(reflect.TypeOf(expected)))
}

// Match returns a comparison.Func which checks if the actual value matches
// `expected`.
//
// Semblance is computed with "github.com/google/go-cmp/cmp", and this
// function accepts additional cmp.Options to allow for handling of different
// types/fields/filtering proto Message semantics, etc.
//
// For convenience, `opts` implicitly includes:
//   - "google.golang.org/protobuf/testing/protocmp".Transform()
//   - A direct comparison of protoreflect.Descriptor types. These are
//     documented as being comparable with `==`, but by default `cmp` will
//     recurse into their guts.
//   - A direct comparison of reflect.Type interfaces.
//
// This is done via the go.chromium.org/luci/common/testing/registry package,
// which also allows process-wide registration of additional default
// cmp.Options. It is recommended to register cmp.Options in TestMain for any
// types that your package tests which contain unexported fields, or other
// internal details. Note that cmp.Diff implicitly will use `X.Equal(X)`
// methods for any types which implement them as methods, so this may be a good
// first thing to implement.
//
// It is recommended that you use should.Equal when comparing primitive types.
func Match[T any](expected T, opts ...cmp.Option) comparison.Func[T] {
	return matchImpl("should.Match", expected, opts)
}

// NotMatch returns a comparison.Func which checks if the actual value doesn't
// match `expected`.
//
// Semblance is computed with "github.com/google/go-cmp/cmp", and this
// function accepts additional cmp.Options to allow for handling of different
// types/fields/filtering proto Message semantics, etc.
//
// For convenience, `opts` implicitly includes:
//   - "google.golang.org/protobuf/testing/protocmp".Transform()
//   - A direct comparison of protoreflect.Descriptor types. These are
//     documented as being comparable with `==`, but by default `cmp` will
//     recurse into their guts.
//   - A direct comparison of reflect.Type interfaces.
//
// This is done via the go.chromium.org/luci/common/testing/registry package,
// which also allows process-wide registration of additional default
// cmp.Options. It is recommended to register cmp.Options in TestMain for any
// types that your package tests which contain unexported fields, or other
// internal details. Note that cmp.Diff implicitly will use `X.Equal(X)`
// methods for any types which implement them as methods, so this may be a good
// first thing to implement.
//
// It is recommended that you use should.NotEqual when comparing primitive types.
func NotMatch[T any](expected T, opts ...cmp.Option) comparison.Func[T] {
	return notMatchImpl("should.NotMatch", expected, opts)
}

func typedDiff(cmpName string, expected, actual any, opts []cmp.Option) (diff string, fail *failure.Summary) {
	diff, ok := typed.DiffSafe(expected, actual, opts...)
	if !ok {
		return "", comparison.NewSummaryBuilder(cmpName, expected).
			Because("typed.Diff failed: %s", diff).
			Summary
	}
	return diff, nil
}

// matchImpl is the implementation of Match and Resemble.
func matchImpl[T any](cmpName string, expected T, opts []cmp.Option) comparison.Func[T] {
	return func(actual T) *failure.Summary {
		diff, fail := typedDiff(cmpName, expected, actual, opts)
		if fail != nil {
			return fail
		}

		if diff == "" {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName, expected).
			Actual(actual).WarnIfLong().
			Expected(expected).WarnIfLong().
			AddCmpDiff(diff).
			Summary
	}
}

// notMatchImpl is the implementation of Match and Resemble.
func notMatchImpl[T any](cmpName string, expected T, opts []cmp.Option) comparison.Func[T] {
	return func(actual T) *failure.Summary {
		diff, fail := typedDiff(cmpName, expected, actual, opts)
		if fail != nil {
			return fail
		}

		if diff != "" {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName, expected).
			Actual(actual).WarnIfLong().
			Summary
	}
}
