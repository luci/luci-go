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

package properties

import (
	"encoding/json"
	"reflect"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucictx"
)

func TestRegisterOptions(t *testing.T) {
	t.Parallel()

	t.Run(`OptSkipFrames`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}

		MustRegister[*struct{}](&r, "$a")
		MustRegister[*struct{}](&r, "$b", OptSkipFrames(1))

		regs := r.listRegistrations()
		assert.That(t, regs["$a"].InputLocation, should.ContainSubstring("register_test.go"))
		// parent of "b" will be in go testing guts, likely testing.go
		assert.That(t, regs["$b"].InputLocation, should.NotContainSubstring("register_test.go"))
	})

	t.Run(`OptIgnoreUnknownFields`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		s := MustRegister[*struct {
			Field int
		}](&r, "$ns", OptIgnoreUnknownFields())

		state, err := r.Instantiate(mustStruct(map[string]any{
			"$ns": map[string]any{
				"Field": 100,
				"other": "hi",
			},
		}), nil)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, s.GetInputFromState(state), should.Match(&struct {
			Field int
		}{Field: 100}))
	})

	t.Run(`OptStrictTopLevelFields`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		MustRegister[*struct{}](&r, "", OptStrictTopLevelFields())

		_, err := r.Instantiate(mustStruct(map[string]any{
			"$ns": map[string]any{
				"stuff": 100,
			},
		}), nil)
		assert.That(t, err, should.ErrLike(`unknown field "$ns"`))
	})

	t.Run(`OptUseJSONNames`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		s := MustRegister[*buildbucketpb.Build](&r, "", OptProtoUseJSONNames())

		state, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

		s.MutateOutputFromState(state, func(b *buildbucketpb.Build) (mutated bool) {
			b.CreatedBy = "someone"
			return true
		})

		s.SetOutputFromState(state, nil) // will set to &buildbucketpb.Build{}

		s.MutateOutputFromState(state, func(b *buildbucketpb.Build) (mutated bool) {
			b.CreatedBy += "else"
			return true
		})

		out, vers, consistent, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, consistent, should.BeTrue)
		assert.That(t, vers, should.Equal[int64](3))
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"createdBy": "else",
		})))
	})

	t.Run(`InputOnly`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		p := MustRegisterIn[*buildbucketpb.Build](&r, "")
		state, err := r.Instantiate(mustStruct(map[string]any{
			"id": 12345,
		}), nil)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, p.GetInputFromState(state), should.Match(&buildbucketpb.Build{
			Id: 12345,
		}))
	})

	t.Run(`OutputOnly`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		p := MustRegisterOut[*buildbucketpb.Build](&r, "")
		state, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

		p.MutateOutputFromState(state, func(b *buildbucketpb.Build) (mutated bool) {
			b.Id = 12345
			return true
		})
	})
}

func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run(`proto`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		top := MustRegister[*buildbucketpb.Build](&r, "")
		sub := MustRegister[*buildbucketpb.Build](&r, "$sub")

		state, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

		top.MutateOutputFromState(state, func(b *buildbucketpb.Build) (mutated bool) {
			b.Id = 12345
			return true
		})

		sub.MutateOutputFromState(state, func(b *buildbucketpb.Build) (mutated bool) {
			// This ensures that we are using jsonpb - otherwise if this is encoded
			// with encoding/json, this will show up in the output as:
			//   {"seconds": <number>, "nanos": <number>}
			b.CreateTime = timestamppb.New(testclock.TestRecentTimeUTC)
			return true
		})

		combined, _, _, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, combined, should.Match(mustStruct(map[string]any{
			"id": "12345",
			"$sub": map[string]any{
				"create_time": "2016-02-03T04:05:06.000000007Z",
			},
		})))
	})

	t.Run(`struct`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		top := MustRegister[*struct {
			ID int
		}](&r, "")
		sub := MustRegister[*struct {
			ID int
		}](&r, "$sub")

		state, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

		top.MutateOutputFromState(state, func(s *struct{ ID int }) (mutated bool) {
			s.ID = 12345
			return true
		})

		sub.MutateOutputFromState(state, func(s *struct{ ID int }) (mutated bool) {
			s.ID = 54321
			return true
		})

		combined, _, _, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, combined, should.Match(mustStruct(map[string]any{
			"ID": 12345,
			"$sub": map[string]any{
				"ID": 54321,
			},
		})))
	})

	t.Run(`split`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		top := MustRegisterInOut[*struct{ In int }, *struct{ Out int }](&r, "")

		state, err := r.Instantiate(mustStruct(map[string]any{
			"In": 100,
		}), nil)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, top.GetInputFromState(state).In, should.Equal(100))

		top.MutateOutputFromState(state, func(s *struct{ Out int }) (mutated bool) {
			s.Out = 200
			return true
		})

		output, _, _, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, output, should.Match(mustStruct(map[string]any{
			"Out": 200.0,
		})))

	})
}

func TestRegister_Errors(t *testing.T) {
	t.Parallel()

	t.Run(`finalized registry`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		_, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, func() {
			MustRegister[*struct{}](&r, "")
		}, should.PanicLikeString("already finalized"))
	})

	t.Run(`duplicate registration`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		MustRegister[*struct{}](&r, "")

		assert.That(t, func() {
			MustRegister[*struct{}](&r, "")
		}, should.PanicLikeString("already registered"))
	})

	t.Run(`overlapping registration`, func(t *testing.T) {
		t.Parallel()

		type problematicField struct {
			F int `json:"$f"`
		}

		t.Run(`top-level -> sub`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			MustRegister[*problematicField](&r, "")
			assert.That(t, func() {
				MustRegister[*problematicField](&r, "$f")
			}, should.PanicLikeString(`cannot register namespace "$f"`))
		})

		t.Run(`sub -> top-level`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			MustRegister[*problematicField](&r, "$f")
			assert.That(t, func() {
				MustRegister[*problematicField](&r, "")
			}, should.PanicLikeString(`cannot register top-level property namespace`))
		})
	})

	t.Run(`non-pointer struct`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		assert.That(t, func() {
			MustRegister[struct{ ID int }](&r, "")
		}, should.PanicLikeString("non-struct-pointer"))
	})
}

func TestTopLevelGeneric(t *testing.T) {
	t.Parallel()

	t.Run(`*Struct`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}

		top := MustRegister[*structpb.Struct](&r, "")
		mid := MustRegister[*structpb.Struct](&r, "$something")

		state, err := r.Instantiate(mustStruct(map[string]any{
			"random": "junk",
			"$something": map[string]any{
				"a":    100,
				"cool": "stuff",
			},
			"other": 20,
		}), nil)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, top.GetInputFromState(state), should.Match(mustStruct(map[string]any{
			"random": "junk",
			"other":  20,
		})))
		assert.That(t, mid.GetInputFromState(state), should.Match(mustStruct(map[string]any{
			"a":    100,
			"cool": "stuff",
		})))
	})

	t.Run(`map`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}

		top := MustRegister[map[string]any](&r, "")
		mid := MustRegister[map[string]any](&r, "$something", OptJSONUseNumber())

		state, err := r.Instantiate(mustStruct(map[string]any{
			"random": "junk",
			"$something": map[string]any{
				"a":    100,
				"cool": "stuff",
			},
			"other": 20,
		}), nil)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, top.GetInputFromState(state), should.Match(map[string]any{
			"random": "junk",
			"other":  20.,
		}))
		assert.That(t, mid.GetInputFromState(state), should.Match(map[string]any{
			"a":    json.Number("100"),
			"cool": "stuff",
		}))
	})
}

func TestGetVisibleFields(t *testing.T) {
	t.Parallel()

	type tcase struct {
		value  any
		expect []string
	}

	runIt := func(tc tcase) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()

			assert.That(t,
				stringset.NewFromSlice(tc.expect...),
				should.Match(getVisibleFields(reflect.TypeOf(tc.value))),
				truth.LineContext())
		}
	}

	t.Run(`basic struct`, runIt(tcase{
		&struct {
			Field int
			other string
		}{},
		[]string{"Field"},
	}))

	type embed struct {
		Field int
	}
	type embed2 struct {
		embed
	}
	type embed3 struct {
		embed `json:"nerp"`
	}

	t.Run(`embed struct`, runIt(tcase{
		&struct {
			embed
			Other string
		}{},
		[]string{"Field", "Other"},
	}))

	t.Run(`double embed struct`, runIt(tcase{
		&struct {
			embed2
		}{},
		[]string{"Field"},
	}))

	t.Run(`double embed partial struct`, runIt(tcase{
		&struct {
			embed3
		}{},
		[]string{"nerp"},
	}))

	t.Run(`embed explicit`, runIt(tcase{
		&struct {
			Worp  embed
			Other string
		}{},
		[]string{"Worp", "Other"},
	}))

	t.Run(`json struct tag`, runIt(tcase{
		&struct {
			Another string `json:"cool"`
			Thing   string `json:"-"`
		}{},
		[]string{"cool"},
	}))

	t.Run(`json struct tag - dash`, runIt(tcase{
		&struct {
			Another string `json:"-,"`
			Thing   string `json:"-"`
		}{},
		[]string{"-"},
	}))

	t.Run(`json struct tag - embed`, runIt(tcase{
		&struct {
			embed `json:"NEAT"`
		}{},
		[]string{"NEAT"},
	}))

	t.Run(`duplicate name`, runIt(tcase{
		&struct {
			field int `json:"NEAT"`
			Woah  int `json:"NEAT"`
		}{},
		[]string{"NEAT"},
	}))

	type recursive struct {
		*recursive
		Field int
	}
	t.Run(`recursive`, runIt(tcase{
		&recursive{},
		[]string{"Field"},
	}))

	t.Run(`proto`, runIt(tcase{
		&buildbucketpb.Step{},
		[]string{
			"endTime",
			"end_time",
			"logs",
			"mergeBuild",
			"merge_build",
			"name",
			"startTime",
			"start_time",
			"status",
			"summaryMarkdown",
			"summary_markdown",
			"tags",
		},
	}))

	// this has explicit [json_name] field name annotations, so doesn't contain
	// secretBytes.
	t.Run(`proto (explicit json_name)`, runIt(tcase{
		&lucictx.Swarming{},
		[]string{
			"secret_bytes",
			"task",
		},
	}))
}

func TestRegisterOptionsErrors(t *testing.T) {
	t.Parallel()

	type ro []RegisterOption
	type tcase struct {
		namespace string
		opts      ro
		in        any
		out       any

		err any
	}

	run := func(tc tcase) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			t.Parallel()

			_, err := loadRegOpts(tc.namespace, tc.opts, reflect.TypeOf(tc.in), reflect.TypeOf(tc.out))
			assert.That(t, err, should.ErrLike(tc.err), truth.LineContext())
		}
	}

	t.Run(`OptIgnoreUnknownFields + OptStrictTopLevelFields`, run(tcase{
		"", ro{OptIgnoreUnknownFields(), OptStrictTopLevelFields()},
		&struct{}{}, nil,
		"not compatible",
	}))

	t.Run(`OptStrictTopLevelFields + OptIgnoreUnknownFields`, run(tcase{
		"", ro{OptStrictTopLevelFields(), OptIgnoreUnknownFields()},
		&struct{}{}, nil,
		"not compatible",
	}))

	t.Run(`OptIgnoreUnknownFields for output-only`, run(tcase{
		"", ro{OptIgnoreUnknownFields()},
		nil, &struct{}{},
		"not compatible with output-only",
	}))

	t.Run(`OptStrictTopLevelFields for sub namespace`, run(tcase{
		"$hi", ro{OptStrictTopLevelFields()},
		&struct{}{}, &struct{}{},
		`not compatible with namespace "$hi"`,
	}))

	t.Run(`OptStrictTopLevelFields for output-only`, run(tcase{
		"", ro{OptStrictTopLevelFields()},
		nil, &struct{}{},
		`not compatible with output-only`,
	}))

	t.Run(`OptStrictTopLevelFields for map types`, run(tcase{
		"", ro{OptStrictTopLevelFields()},
		map[string]string{}, &struct{}{},
		`not compatible with type map[string]string`,
	}))

	t.Run(`OptProtoUseJSONNames for input-only`, run(tcase{
		"", ro{OptProtoUseJSONNames()},
		map[string]string{}, nil,
		`not compatible with input-only`,
	}))

	t.Run(`OptProtoUseJSONNames for struct`, run(tcase{
		"", ro{OptProtoUseJSONNames()},
		nil, &struct{}{},
		`not compatible with non-proto type *struct {}`,
	}))

	t.Run(`OptProtoUseJSONNames for struct`, run(tcase{
		"", ro{OptProtoUseJSONNames()},
		nil, map[string]any{},
		`not compatible with non-proto type map[string]interface {}`,
	}))

	t.Run(`OptJSONUseNumber for output-only`, run(tcase{
		"", ro{OptJSONUseNumber()},
		nil, map[string]any{},
		`not compatible with output-only`,
	}))

	t.Run(`OptJSONUseNumber for proto`, run(tcase{
		"", ro{OptJSONUseNumber()},
		&buildbucketpb.Build{}, nil,
		`not compatible with proto *buildbucketpb.Build`,
	}))
}
