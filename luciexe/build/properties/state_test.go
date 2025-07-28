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
	"context"
	"strconv"
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStateSerialize(t *testing.T) {
	t.Parallel()

	t.Run(`no top-level`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		p := MustRegister[*struct{ ID int }](&r, "$sub")
		state, err := r.Instantiate(context.Background(), nil, nil)

		assert.NoErr(t, err)
		p.MutateOutputFromState(state, func(s *struct{ ID int }) (mutated bool) {
			s.ID = 200
			return true
		})

		out, _, _, err := state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"$sub": map[string]any{
				"ID": 200,
			},
		})))
	})

	t.Run(`skip empty sub`, func(t *testing.T) {
		r := Registry{}
		top := MustRegister[*struct{ ID int }](&r, "")

		sub := MustRegister[*struct {
			ID int `json:",omitempty"`
		}](&r, "$sub")
		state, err := r.Instantiate(context.Background(), nil, nil)

		assert.NoErr(t, err)
		top.MutateOutputFromState(state, func(s *struct{ ID int }) (mutated bool) {
			s.ID = 200
			return true
		})

		out, _, _, err := state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"ID": 200,
			// sub is omitted because it is empty
		})))

		sub.MutateOutputFromState(state, func(s *struct {
			ID int "json:\",omitempty\""
		}) (mutated bool) {
			s.ID = 1234
			return true
		})

		out, _, _, err = state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"ID": 200,
			"$sub": map[string]any{
				"ID": 1234,
			},
		})))
	})

	t.Run(`leftover fields`, func(t *testing.T) {
		t.Parallel()

		t.Run(`leftover error`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			_, err := r.Instantiate(context.Background(), mustStruct(map[string]any{
				"extra": 100,
			}), nil)
			assert.That(t, err, should.ErrLike("no top-level property registered"))
		})

		t.Run(`collect leftovers into *Struct`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			top := MustRegister[*structpb.Struct](&r, "")
			state, err := r.Instantiate(context.Background(), mustStruct(map[string]any{
				"extra": 100,
			}), nil)
			assert.NoErr(t, err)
			assert.That(t, top.GetInputFromState(state), should.Match(mustStruct(map[string]any{
				"extra": 100,
			})))
		})

		t.Run(`collect leftovers into map`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			top := MustRegister[map[string]any](&r, "")
			state, err := r.Instantiate(context.Background(), mustStruct(map[string]any{
				"extra": 100,
			}), nil)
			assert.NoErr(t, err)
			assert.That(t, top.GetInputFromState(state), should.Match(map[string]any{
				"extra": 100.0,
			}))
		})
	})

	t.Run(`toplevel map with overlap`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}

		top := MustRegister[map[string]any](&r, "")
		mid := MustRegister[map[string]any](&r, "$mid")

		state, err := r.Instantiate(context.Background(), nil, nil)
		assert.NoErr(t, err)

		top.MutateOutputFromState(state, func(m map[string]any) (mutated bool) {
			m["hello"] = "world"
			return true
		})
		mid.MutateOutputFromState(state, func(m map[string]any) (mutated bool) {
			m["inner"] = "peace"
			return true
		})

		// so far, so good
		out, _, _, err := state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"hello": "world",
			"$mid": map[string]any{
				"inner": "peace",
			},
		})))

		top.MutateOutputFromState(state, func(m map[string]any) (mutated bool) {
			m["$mid"] = "oops"
			return true
		})
		_, _, _, err = state.Serialize()
		assert.That(t, err, should.ErrLike(
			`top-level namespace property "$mid" overlaps with registered namespace`))
	})

	t.Run(`no namespace content`, func(t *testing.T) {
		r := Registry{}
		top := MustRegisterOut[map[string]string](&r, "")
		sub := MustRegisterOut[map[string]string](&r, "$sub")
		state, err := r.Instantiate(context.Background(), nil, nil)
		assert.NoErr(t, err)

		out, _, _, err := state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match[*structpb.Struct](nil))

		sub.SetOutputFromState(state, map[string]string{
			"heylo": "world",
		})
		out, _, _, err = state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"$sub": map[string]any{"heylo": "world"},
		})))

		top.SetOutputFromState(state, map[string]string{
			"cool": "beans",
		})
		out, _, _, err = state.Serialize()
		assert.NoErr(t, err)
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"cool": "beans",
			"$sub": map[string]any{
				"heylo": "world",
			},
		})))
	})
}

func TestStateNotify(t *testing.T) {
	t.Parallel()

	var vers int64
	bumpVers := func(version int64) {
		if version > vers {
			vers = version
		}
	}

	r := Registry{}
	top := MustRegister[map[string]any](&r, "")
	state, err := r.Instantiate(context.Background(), nil, bumpVers)
	assert.NoErr(t, err)

	top.MutateOutputFromState(state, func(m map[string]any) (mutated bool) {
		m["hello"] = "world"
		return true
	})

	top.MutateOutputFromState(state, func(m map[string]any) (mutated bool) {
		m["hello"] = m["hello"].(string) + ", neat"
		return true
	})

	assert.Loosely(t, vers, should.Equal(2))
}

func TestConcurrentManipulation(t *testing.T) {
	t.Parallel()

	const N = 200
	const M = 3000

	type OneNumber struct {
		Num int32
	}

	manipulators := make([]RegisteredPropertyOut[*OneNumber], N)

	r := &Registry{}
	for i := range N {
		manipulators[i] = MustRegisterOut[*OneNumber](r, "$"+strconv.Itoa(i))
	}

	ch := make(chan int64, 100)

	state, err := r.Instantiate(context.Background(), nil, func(version int64) {
		ch <- version
	})
	assert.NoErr(t, err)

	// We use this pattern instead of assert.That to ensure that all the
	// goroutines drain correctly.
	hadErrors := false
	defer func() {
		if hadErrors {
			t.Fail()
		}
	}()
	handleErr := func(err error) {
		if err != nil {
			t.Log(err)
			hadErrors = true
		}
	}

	observed := make([]int32, N)
	serialize := func() (int64, bool) {
		ser, version, consistent, err := state.Serialize()
		handleErr(err)
		for ns, val := range ser.Fields {
			i, err := strconv.Atoi(ns[1:])
			handleErr(err)
			observed[i] = int32(val.GetStructValue().Fields["Num"].GetNumberValue())
			handleErr(err)
		}
		return version, consistent
	}

	var wg sync.WaitGroup

	for i := range N {
		wg.Add(1)
		manip := manipulators[i]
		go func() {
			defer wg.Done()

			for j := range M {
				manip.SetOutputFromState(state, &OneNumber{int32(j)})
			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	numInconsistent := 0
	gotVersion := int64(0)
	for version := range ch {
		if version > int64(gotVersion) {
			var consistent bool
			gotVersion, consistent = serialize()
			if !consistent {
				numInconsistent++
			}
		}
	}

	// ok, all goroutines should be drained now, we should be able to observe
	// a consistent state.

	// We should have observed at least one out-of-order delivery
	assert.That(t, numInconsistent, should.BeGreaterThan(1))

	// Every observed value after serialize() should be M-1 - this shows that even
	// though we got inconsistent updates, after processing all version pings we
	// actually did achieve consistency.
	for i, val := range observed {
		if !check.That(t, val, should.Equal[int32](M-1)) {
			t.Logf(">> in observed[%d]", i)
		}
	}

	// When we serialize again, we should get a consistent snapshot, and we should
	// have a version of M*N.
	version, consistent := serialize()
	assert.That(t, consistent, should.BeTrue)
	assert.That(t, version, should.Equal[int64](M*N))
}
