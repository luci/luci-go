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
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func TestMatch(t *testing.T) {
	t.Parallel()

	t.Run("simple", shouldPass(Match(100)(100)))
	t.Run("simple diff", shouldFail(Match(100)(101), "Diff"))

	t.Run("simple proto", shouldPass(
		Match(&buildbucketpb.Build{Id: 12345})(&buildbucketpb.Build{Id: 12345})))

	props, err := structpb.NewStruct(map[string]any{
		"heyo": 100,
	})
	if err != nil {
		t.Fatal("could not make struct", err)
	}
	t.Run("struct proto diff", shouldFail(
		Match(&buildbucketpb.Build{Id: 12345, Input: &buildbucketpb.Build_Input{
			Properties: props,
		}})(&buildbucketpb.Build{Id: 12345}), "Diff"))

	type myStruct struct {
		private string
	}
	t.Run("unexported fields",
		shouldFail(Match(myStruct{"hi"})(myStruct{"hi"}),
			"unexported field"))
}

func TestNotMatch(t *testing.T) {
	t.Parallel()

	t.Run("simple", shouldFail(NotMatch(100)(100)))
	t.Run("simple diff", shouldPass(NotMatch(100)(101)))

	t.Run("simple proto", shouldFail(
		NotMatch(&buildbucketpb.Build{Id: 12345})(&buildbucketpb.Build{Id: 12345})))

	props, err := structpb.NewStruct(map[string]any{
		"heyo": 100,
	})
	if err != nil {
		t.Fatal("could not make struct", err)
	}
	t.Run("struct proto diff", shouldPass(
		NotMatch(&buildbucketpb.Build{Id: 12345, Input: &buildbucketpb.Build_Input{
			Properties: props,
		}})(&buildbucketpb.Build{Id: 12345})))

	type myStruct struct {
		private string
	}
	t.Run("unexported fields",
		shouldFail(NotMatch(myStruct{"hi"})(myStruct{"hi"}),
			"unexported field"))
}

func TestResemble(t *testing.T) {
	// Note that Resemble and Match are the same except for unexported fields.

	type myStruct struct {
		private string
	}
	t.Run("unexported fields", shouldPass(Resemble(myStruct{"hi"})(myStruct{"hi"})))
}

func TestResembleTypeWalking(t *testing.T) {
	// t.Parallel() - this involves testing the resembleOptionCache, so do not run
	// this test in parallel with anything else.

	// Ensure this test never leaks option cache entries.
	//
	// If the option cache is working correctly, it's not a big deal to leak, but
	// if it's not, this may help minimize blast radius from this test to others
	// in this package.
	defer resetOptionCache()

	type tcase struct {
		typ               reflect.Type
		expectOptsLen     int
		expectCachedTypes map[reflect.Type]int
	}

	fabType := reflect.TypeFor[struct {
		fieldA string
		fieldB string
	}]()

	type fabRecursive struct {
		fieldA string
		fieldB string

		sub *fabRecursive
	}
	fabRecursiveType := reflect.TypeFor[fabRecursive]()

	runIt := func(tc *tcase, extra ...func(t *testing.T)) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			resetOptionCache()

			opts := extractAllowUnexportedFrom(tc.typ)
			if len(opts) != tc.expectOptsLen {
				t.Fatalf("expected %d opts, got %d: %q", tc.expectOptsLen, len(opts), opts)
			}
			extraEntries := make(map[reflect.Type][]cmp.Option, len(resembleOptionCache))
			for k, v := range resembleOptionCache {
				extraEntries[k] = v
			}
			for typ, count := range tc.expectCachedTypes {
				delete(extraEntries, typ)

				actual, ok := resembleOptionCache[typ]
				if !ok {
					t.Fatalf("missing cached type %s", typ)
				}
				if len(actual) != count {
					t.Fatalf("expected %d cached opts for %s, got %d: %q", count, typ, len(actual), actual)
				}
			}
			if len(extraEntries) > 0 {
				t.Fatalf("got extra resembleOptionCache entries: %q", extraEntries)
			}

			for _, ex := range extra {
				ex(t)
			}
		}
	}

	t.Run("simple types leave no entries", runIt(&tcase{
		reflect.TypeFor[string](),
		0,
		nil,
	}))

	t.Run("struct types with no unexported fields leaves nil entry", runIt(&tcase{
		reflect.TypeFor[struct{ Field string }](),
		0,
		map[reflect.Type]int{
			reflect.TypeFor[struct{ Field string }](): 0,
		},
	}))

	t.Run("struct types with no exported fields leaves single entry", runIt(&tcase{
		fabType,
		1,
		map[reflect.Type]int{
			fabType: 1,
		},
	}, func(t *testing.T) {
		// "and we can see the cache hit in code coverage"
		// note that extractAllowUnexportedFrom(typ) is already cached in this
		// subtest.
		if opts := extractAllowUnexportedFrom(fabType); len(opts) != 1 {
			t.Fatalf("expected %d opts, got %d: %q", 1, len(opts), opts)
		}
	}))

	t.Run("*struct types with no exported fields leaves single entry", runIt(&tcase{
		reflect.PointerTo(fabType),
		1,
		map[reflect.Type]int{
			fabType: 1,
		},
	}))

	t.Run("recursive struct types with no exported fields leaves single entry", runIt(&tcase{
		reflect.PointerTo(fabRecursiveType),
		1,
		map[reflect.Type]int{fabRecursiveType: 1},
	}))

	t.Run("maps with struct keys work", runIt(&tcase{
		reflect.TypeFor[map[struct{ a string }]struct{ b int }](),
		2,
		map[reflect.Type]int{
			reflect.TypeFor[struct{ a string }](): 1,
			reflect.TypeFor[struct{ b int }]():    1,
		},
	}))

	t.Run("proto fields are ignored", runIt(&tcase{
		reflect.TypeFor[struct{ Struct *structpb.Struct }](),
		0,
		map[reflect.Type]int{
			reflect.TypeFor[struct{ Struct *structpb.Struct }](): 0,
		},
	}))

}
