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
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStateSerialize(t *testing.T) {
	t.Parallel()

	t.Run(`no top-level`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}
		p := MustRegister[*struct{ ID int }](&r, "sub")
		state, err := r.Instantiate(nil, nil)

		assert.That(t, err, should.ErrLike(nil))
		p.MutateOutputFromState(state, func(s *struct{ ID int }) (mutated bool) {
			s.ID = 200
			return true
		})

		out, _, _, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"sub": map[string]any{
				"ID": 200,
			},
		})))
	})

	t.Run(`skip empty sub`, func(t *testing.T) {
		r := Registry{}
		top := MustRegister[*struct{ ID int }](&r, "")

		sub := MustRegister[*struct {
			ID int `json:",omitempty"`
		}](&r, "sub")
		state, err := r.Instantiate(nil, nil)

		assert.That(t, err, should.ErrLike(nil))
		top.MutateOutputFromState(state, func(s *struct{ ID int }) (mutated bool) {
			s.ID = 200
			return true
		})

		out, _, _, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
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
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"ID": 200,
			"sub": map[string]any{
				"ID": 1234,
			},
		})))
	})

	t.Run(`leftover fields`, func(t *testing.T) {
		t.Parallel()

		t.Run(`leftover error`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			_, err := r.Instantiate(mustStruct(map[string]any{
				"extra": 100,
			}), nil)
			assert.That(t, err, should.ErrLike("no top-level property registered"))
		})

		t.Run(`collect leftovers into *Struct`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			top := MustRegister[*structpb.Struct](&r, "")
			state, err := r.Instantiate(mustStruct(map[string]any{
				"extra": 100,
			}), nil)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, top.GetInputFromState(state), should.Match(mustStruct(map[string]any{
				"extra": 100,
			})))
		})

		t.Run(`collect leftovers into map`, func(t *testing.T) {
			t.Parallel()

			r := Registry{}
			top := MustRegister[map[string]any](&r, "")
			state, err := r.Instantiate(mustStruct(map[string]any{
				"extra": 100,
			}), nil)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, top.GetInputFromState(state), should.Match(map[string]any{
				"extra": 100.0,
			}))
		})
	})

	t.Run(`toplevel map with overlap`, func(t *testing.T) {
		t.Parallel()

		r := Registry{}

		top := MustRegister[map[string]any](&r, "")
		mid := MustRegister[map[string]any](&r, "mid")

		state, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

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
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"hello": "world",
			"mid": map[string]any{
				"inner": "peace",
			},
		})))

		top.MutateOutputFromState(state, func(m map[string]any) (mutated bool) {
			m["mid"] = "oops"
			return true
		})
		_, _, _, err = state.Serialize()
		assert.That(t, err, should.ErrLike(
			`top-level namespace property "mid" overlaps with registered namespace`))
	})

	t.Run(`no namespace content`, func(t *testing.T) {
		r := Registry{}
		top := MustRegisterOut[map[string]string](&r, "")
		sub := MustRegisterOut[map[string]string](&r, "sub")
		state, err := r.Instantiate(nil, nil)
		assert.That(t, err, should.ErrLike(nil))

		out, _, _, err := state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, out, should.Match[*structpb.Struct](nil))

		sub.SetOutputFromState(state, map[string]string{
			"heylo": "world",
		})
		out, _, _, err = state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"sub": map[string]any{"heylo": "world"},
		})))

		top.SetOutputFromState(state, map[string]string{
			"cool": "beans",
		})
		out, _, _, err = state.Serialize()
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, out, should.Match(mustStruct(map[string]any{
			"cool": "beans",
			"sub": map[string]any{
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
	state, err := r.Instantiate(nil, bumpVers)
	assert.That(t, err, should.ErrLike(nil))

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
