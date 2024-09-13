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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestInstantiate(t *testing.T) {
	t.Parallel()

	type someStruct struct {
		Field int
	}

	t.Run(`no top-level`, func(t *testing.T) {
		t.Parallel()

		r := &Registry{}
		_ = MustRegisterIn[*someStruct](r, "$sub")

		t.Run(`extra properties`, func(t *testing.T) {
			t.Parallel()

			_, err := r.Instantiate(mustStruct(map[string]any{
				"bogus": 100,
			}), nil)
			assert.That(t, err, should.ErrLike("leftover top-level properties"))
		})

		t.Run(`extra namespaces OK`, func(t *testing.T) {
			t.Parallel()

			_, err := r.Instantiate(mustStruct(map[string]any{
				"$bogus": 100,
			}), nil)
			assert.That(t, err, should.ErrLike(nil))
		})

		t.Run(`bad sub decode`, func(t *testing.T) {
			t.Parallel()

			_, err := r.Instantiate(mustStruct(map[string]any{
				"$sub": map[string]any{
					"other": 100,
				},
			}), nil)
			assert.That(t, err, should.ErrLike(`unknown field "other"`))
		})

		t.Run(`sub not struct`, func(t *testing.T) {
			t.Parallel()

			_, err := r.Instantiate(mustStruct(map[string]any{
				"$sub": 100,
			}), nil)
			assert.That(t, err, should.ErrLike(`input is not Struct`))
		})
	})

	t.Run(`direct top-level namespace`, func(t *testing.T) {
		t.Parallel()

		type weirdStruct struct {
			Bogus int `json:"$bogus"`
		}

		r := &Registry{}
		w := MustRegisterIn[*weirdStruct](r, "", OptStrictTopLevelFields())

		t.Run(`extra namespaces`, func(t *testing.T) {
			state, err := r.Instantiate(mustStruct(map[string]any{
				"$bogus": 100,
			}), nil)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, w.GetInputFromState(state).Bogus, should.Equal(100))
		})
	})

	t.Run(`strict top-level`, func(t *testing.T) {
		t.Parallel()

		r := &Registry{}
		_ = MustRegisterIn[*someStruct](r, "", OptStrictTopLevelFields())

		t.Run(`extra namespaces`, func(t *testing.T) {
			_, err := r.Instantiate(mustStruct(map[string]any{
				"$bogus": 100,
			}), nil)
			assert.That(t, err, should.ErrLike(`unknown field "$bogus"`))
		})
	})
}
