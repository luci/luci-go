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

package properties_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/luciexe/build/properties"
)

// MyRegistry is an example properties.Registry.
//
// NOTE: MOST users of `properties` will be using it indirectly via
// the [go.chromium.org/luci/luciexe/build.Properties] Registry, and
// registering properties directly with the functions in that package.
//
// This example, however, is potentially useful for writing tests, etc. where
// you may want to create your own properties.State and put it in the context
// for tests.
//
// The Registry holds all schema-property registration. It is effectively
// append-only, and becomes immutable as soon as the first State is
// Instantiated from it.
var MyRegistry = &properties.Registry{}

type MyTopLevelStruct struct {
	Key      string `json:"key"`
	OtherKey string `json:"okey"`
}

// InOutProp is an example of using a Go struct for a schema.
var InOutProp = properties.MustRegister[*MyTopLevelStruct](MyRegistry, "")

// BuildLikeProp is an example of using a proto Message as a schema.
//
// Typically these non top-level namespaces would be registered in different Go
// packages, similar to using the go "flag" package.
var BuildLikeProp = properties.MustRegister[*buildbucketpb.Build](MyRegistry, "$buildLike")

// PerfCounters is an example of using a raw Go map to have a schema-less
// namespace.
var PerfCounters = properties.MustRegisterOut[map[string]int](MyRegistry, "$perf_out")

func ExampleRegistry() {
	rawInputProperties, err := structpb.NewStruct(map[string]any{
		"key":  "hello",
		"okey": "more top level",

		"$buildLike": map[string]any{
			"id": "12345",
		},
	})
	if err != nil {
		panic(err)
	}

	state, err := MyRegistry.Instantiate(context.Background(), rawInputProperties, nil)
	if err != nil {
		panic(err)
	}
	ctx, err := state.SetInContext(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("%#v\n", InOutProp.GetInput(ctx))

	// protos print as textpb - use >>> <<< to make this clearer in the output.
	fmt.Printf(">>>%s<<<\n", BuildLikeProp.GetInput(ctx))

	// We can mutate the output property - every output property, even those
	// registered as input/output properties, starts as a blank object. If you
	// wish to start the output value as a copy of the input value, you can use
	// something like:
	//
	//   BuildLikeProp.SetOutput(proto.Clone(BuildLikeProp.GetInput(ctx)))
	//
	// Note that the input value should be treated as read-only; It is up to your
	// program to ensure this is the case.
	BuildLikeProp.MutateOutput(ctx, func(b *buildbucketpb.Build) (mutated bool) {
		b.Builder = &buildbucketpb.BuilderID{Bucket: "buck", Builder: "bld"}
		return true
	})

	PerfCounters.SetOutput(ctx, map[string]int{
		"alpha": 20,
		"yeet":  -1,
	})

	// At any point the program can serialize the state out to a proto Struct.
	//
	// Most programs will not need to do this explicitly; For a luciexe build,
	// this will happen roughly any time any registered property for any namespace
	// is set or mutated. The luciexe/build library also imposes a maximum 1 qps
	// rate limit for outgoing changes, so even if properties are rapidly updated,
	// a maximum of one build update will happen per second.
	rawStruct, vers, consistent, err := state.Serialize()
	if err != nil {
		panic(err)
	}
	fmt.Print("--------\n\n")
	fmt.Println("vers:", vers)
	// consistent would be `false` if another goroutine set/mutated an output
	// property while Serialize was running. Each individual registered namespace
	// is self-consistent, but there is no consistency between different
	// namespaces.
	fmt.Println("consistent:", consistent)
	fmt.Print("--------\n\n")

	blob, err := protojson.Marshal(rawStruct)
	if err != nil {
		panic(err)
	}

	var out bytes.Buffer
	if err := json.Indent(&out, blob, "", "  "); err != nil {
		panic(err)
	}
	fmt.Println(out.String())

	// Output:
	// &properties_test.MyTopLevelStruct{Key:"hello", OtherKey:"more top level"}
	// >>>id:12345<<<
	// --------
	//
	// vers: 2
	// consistent: true
	// --------
	//
	// {
	//   "$buildLike": {
	//     "builder": {
	//       "bucket": "buck",
	//       "builder": "bld"
	//     }
	//   },
	//   "$perf_out": {
	//     "alpha": 20,
	//     "yeet": -1
	//   },
	//   "key": "",
	//   "okey": ""
	// }
}
