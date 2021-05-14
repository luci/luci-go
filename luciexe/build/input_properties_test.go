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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/luciexe/build/internal/testpb"
)

func buildWithInput(json string) *bbpb.Build {
	ret := &bbpb.Build{
		Input: &bbpb.Build_Input{
			Properties: &structpb.Struct{},
		},
	}
	if err := protojson.Unmarshal([]byte(json), ret.Input.Properties); err != nil {
		panic(err)
	}
	return ret
}

func TestInputProperties(t *testing.T) {
	// Intentionally forgo t.Parallel() due to global reservation structures.

	Convey(`Properties Input`, t, func() {
		// reset `reservations` after each test
		defer propReaderReservations.clear()

		var reader func(context.Context) *testpb.Module
		MakePropertyReader("ns", &reader)

		Convey(`empty input`, func() {
			build, ctx, err := Start(context.Background(), nil)
			So(err, ShouldBeNil)
			defer func() { build.End(nil) }()

			data := reader(ctx)
			So(data, ShouldBeNil)
		})

		Convey(`populated input`, func() {
			build, ctx, err := Start(context.Background(), buildWithInput(`{
          "arbitrary": "stuff",
			    "ns": {
						"field": "hello",
						"$cool": "there"
					}
				}`))
			So(err, ShouldBeNil)
			defer func() { build.End(nil) }()

			data := reader(ctx)
			So(data, ShouldResembleProto, &testpb.Module{
				Field:         "hello",
				JsonNameField: "there",
			})
		})

		Convey(`top level properties`, func() {
			props := &testpb.TopLevel{}
			build, _, err := Start(context.Background(), buildWithInput(`{
          "arbitrary": "stuff",

					"field": "hello",
					"$cool": "there",

			    "sub": {
						"ignore": "yes"
					}
				}`), OptParseProperties(props))
			So(err, ShouldBeNil)
			defer func() { build.End(nil) }()

			So(props, ShouldResembleProto, &testpb.TopLevel{
				Field:         "hello",
				JsonNameField: "there",
			})
		})

		Convey(`top and module properties`, func() {
			props := &testpb.TopLevel{}
			_, _, err := Start(context.Background(), buildWithInput(`{
          "arbitrary": "stuff",

					"field": "hello",
					"$cool": "there",

			    "ns": {
						"field": "general",
						"$cool": "kenobi..."
					}
				}`), OptParseProperties(props), OptStrictInputProperties())
			So(err, ShouldErrLike, `parsing top-level properties`, `unknown field "arbitrary"`)

			build, ctx, err := Start(context.Background(), buildWithInput(`{
					"field": "hello",
					"$cool": "there",

			    "ns": {
						"field": "general",
						"$cool": "kenobi..."
					}
				}`), OptParseProperties(props), OptStrictInputProperties())
			So(err, ShouldBeNil)
			defer func() { build.End(nil) }()

			So(props, ShouldResembleProto, &testpb.TopLevel{
				Field:         "hello",
				JsonNameField: "there",
			})

			So(reader(ctx), ShouldResembleProto, &testpb.Module{
				Field:         "general",
				JsonNameField: "kenobi...",
			})

		})

		Convey(`conflict with top properties`, func() {
			var conflict func(context.Context) *testpb.Module
			MakePropertyReader("$cool", &conflict)

			props := &testpb.TopLevel{}
			_, _, err := Start(context.Background(), buildWithInput(`{
          "arbitrary": "stuff",

					"field": "hello",
					"$cool": {
					}
				}`), OptParseProperties(props), OptStrictInputProperties())
			So(err, ShouldErrLike,
				`parsing top-level properties`,
				`use of top-level property message *testpb.TopLevel`,
				`conflicts with MakePropertyReader(ns="$cool") reserved at`,
				`luciexe/build/input_properties_test.go`)
		})
	})
}
