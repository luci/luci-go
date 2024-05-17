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

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
)

func TestMatch(t *testing.T) {
	t.Parallel()

	t.Run("simple", shouldPass(Match(100)(100)))
	t.Run("simple fail", shouldFail(Match(100)(101), "Diff"))

	t.Run("simple proto", shouldPass(
		Match(&buildbucketpb.Build{Id: 12345})(&buildbucketpb.Build{Id: 12345})))

	props, err := structpb.NewStruct(map[string]any{
		"heyo": 100,
	})
	if err != nil {
		t.Fatal("could not make struct", err)
	}
	t.Run("struct proto fail", shouldFail(
		Match(&buildbucketpb.Build{Id: 12345, Input: &buildbucketpb.Build_Input{
			Properties: props,
		}})(&buildbucketpb.Build{Id: 12345}), "Diff"))

	t.Run("unexported fields", func(t *testing.T) {
		type myStruct struct {
			private string
		}
		mustPanicLike(t, "unexported field", func() {
			Match(myStruct{"hi"})(myStruct{"hi"})
		})
	})
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

	ftt.Run("with clean state", t, func(t *ftt.Test) {
		// Always start with an empty cache
		resetOptionCache()

		t.Run("simple types leave no entries", func(t *ftt.Test) {
			assert.Loosely(t, extractAllowUnexportedFrom(reflect.TypeOf("hello")), BeNil)
			assert.Loosely(t, resembleOptionCache, BeEmpty)
		})

		t.Run("struct types with no unexported fields leaves nil entry", func(t *ftt.Test) {
			type myStruct struct {
				Field string
			}
			typ := reflect.TypeFor[myStruct]()
			assert.Loosely(t, extractAllowUnexportedFrom(typ), BeNil)
			assert.Loosely(t, resembleOptionCache, HaveLength(1))
			assert.Loosely(t, resembleOptionCache[typ], BeNil)
		})

		t.Run("struct types with no exported fields leaves single entry", func(t *ftt.Test) {
			type myStruct struct {
				fieldA string
				fieldB string
			}
			typ := reflect.TypeFor[myStruct]()
			assert.Loosely(t, extractAllowUnexportedFrom(typ), HaveLength(1))
			assert.Loosely(t, resembleOptionCache, HaveLength(1))
			assert.Loosely(t, resembleOptionCache[typ], HaveLength(1))

			t.Run("and we can see the cache hit in code coverage", func(t *ftt.Test) {
				// note that extractAllowUnexportedFrom(typ) is already cached in this
				// subtest.
				assert.Loosely(t, extractAllowUnexportedFrom(typ), HaveLength(1))
			})
		})

		t.Run("*struct types with no exported fields leaves single entry", func(t *ftt.Test) {
			type myStruct struct {
				fieldA string
				fieldB string
			}
			typ := reflect.TypeFor[*myStruct]()
			assert.Loosely(t, extractAllowUnexportedFrom(typ), HaveLength(1))
			assert.Loosely(t, resembleOptionCache, HaveLength(1))
			// note that myStruct is in cache, not *myStruct
			assert.Loosely(t, resembleOptionCache[typ.Elem()], HaveLength(1))
		})

		t.Run("recursive struct types with no exported fields leaves single entry", func(t *ftt.Test) {
			type myStruct struct {
				fieldA string
				fieldB string

				sub *myStruct
			}
			typ := reflect.TypeFor[*myStruct]()
			assert.Loosely(t, extractAllowUnexportedFrom(typ), HaveLength(1))
			assert.Loosely(t, resembleOptionCache, HaveLength(1))
			// note that myStruct is in cache, not *myStruct
			assert.Loosely(t, resembleOptionCache[typ.Elem()], HaveLength(1))
		})

		t.Run("maps with struct keys work", func(t *ftt.Test) {
			type myKey struct {
				field string
			}
			type myValue struct {
				thing int
			}
			assert.Loosely(t, extractAllowUnexportedFrom(reflect.TypeFor[map[myKey]*myValue]()), HaveLength(2))
			assert.Loosely(t, resembleOptionCache, HaveLength(2))
			assert.Loosely(t, resembleOptionCache[reflect.TypeFor[myKey]()], HaveLength(1))
			assert.Loosely(t, resembleOptionCache[reflect.TypeFor[myValue]()], HaveLength(1))
		})

		t.Run("proto fields are ignored", func(t *ftt.Test) {
			type myStruct struct {
				Struct *structpb.Struct
			}
			assert.Loosely(t, extractAllowUnexportedFrom(reflect.TypeFor[myStruct]()), BeEmpty)
			assert.Loosely(t, resembleOptionCache, HaveLength(1))
			assert.Loosely(t, resembleOptionCache[reflect.TypeFor[myStruct]()], BeNil)
		})

	})
}
