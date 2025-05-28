// Copyright 2025 The LUCI Authors.
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

package errtag_test

import (
	"errors"
	"testing"

	luci_errors "go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	btag    = errtag.Make("boolish", true)
	otag    = errtag.Make("other bool", false)
	stag    = errtag.Make("stringy", "defaultValue")
	stagAdd = errtag.MakeWithMerge("stringy (add)", "defaultValue", func(vals []*string) *string {
		ret := ""
		for _, val := range vals {
			ret += *val
		}
		return &ret
	})
)

func TestErrTag(t *testing.T) {
	t.Parallel()
	t.Run(`basic boolean`, func(t *testing.T) {
		t.Run(`absent`, func(t *testing.T) {
			err := otag.Apply(errors.New("meep"))
			val, ok := btag.Value(err)
			assert.That(t, ok, should.BeFalse)
			assert.That(t, val, should.BeTrue)

			assert.That(t, btag.In(err), should.BeFalse)
			assert.That(t, otag.ValueOrDefault(err), should.BeFalse)
		})

		t.Run(`present`, func(t *testing.T) {
			err := btag.Apply(errors.New("meep"))
			val, ok := btag.Value(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, val, should.BeTrue)

			assert.That(t, btag.In(err), should.BeTrue)
		})

		t.Run(`override`, func(t *testing.T) {
			core := errors.New("meep")
			err := btag.ApplyValue(btag.Apply(core), false)
			val, ok := btag.Value(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, val, should.BeFalse)

			assert.That(t, btag.In(err), should.BeFalse)

			assert.That(t, err, should.ErrLike(core))
		})

		t.Run(`applying to nil is no-op`, func(t *testing.T) {
			assert.Loosely(t, btag.Apply(nil), should.BeNil)
			assert.Loosely(t, btag.ApplyValue(nil, true), should.BeNil)
		})

		t.Run(`apply with luci errors library New`, func(t *testing.T) {
			err := btag.Apply(luci_errors.New("bare"))
			assert.That(t, btag.In(err), should.BeTrue)
			assert.That(t, btag.ValueOrDefault(err), should.BeTrue)
		})

		t.Run(`apply with luci errors library Annotate`, func(t *testing.T) {
			stag2 := stag.WithDefault("newDefault")

			err := stag2.Apply(luci_errors.Fmt("extra: %w", errors.New("bare")))
			assert.That(t, stag.ValueOrDefault(err), should.Equal("newDefault"))
		})

		t.Run(`apply with luci errors library Reason`, func(t *testing.T) {
			err := luci_errors.Reason("bare").Tag(btag).Err()
			assert.That(t, btag.In(err), should.BeTrue)
			assert.That(t, btag.ValueOrDefault(err), should.BeTrue)
		})

		t.Run(`can find through other wrapper`, func(t *testing.T) {
			err := luci_errors.Fmt("extra: %w", stag.ApplyValue(errors.New("bare"), "someval"))
			assert.That(t, err.Error(), should.Equal("extra: bare"))
			assert.That(t, stag.ValueOrDefault(err), should.Equal("someval"))
		})
	})

	t.Run(`basic string`, func(t *testing.T) {
		t.Run(`absent`, func(t *testing.T) {
			err := errors.New("meep")
			val, ok := stag.Value(err)
			assert.That(t, ok, should.BeFalse)
			assert.That(t, val, should.Equal("defaultValue"))

			assert.That(t, stag.In(err), should.BeFalse)
		})

		t.Run(`present`, func(t *testing.T) {
			err := stag.Apply(errors.New("meep"))
			val, ok := stag.Value(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, val, should.Equal("defaultValue"))

			assert.That(t, stag.In(err), should.BeTrue)
		})

		t.Run(`override`, func(t *testing.T) {
			core := errors.New("meep")
			err := stag.ApplyValue(stag.Apply(core), "nope")
			val, ok := stag.Value(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, val, should.Equal("nope"))

			assert.That(t, stag.In(err), should.BeFalse)

			assert.That(t, err, should.ErrLike(core))
		})
	})

	t.Run(`multierror`, func(t *testing.T) {
		// stag has 'pick first' semantics
		merr := errors.Join(
			stag.ApplyValue(errors.New("hello"), "hey"),
			stag.ApplyValue(errors.New("other"), "there"),
		)

		assert.That(t, stag.ValueOrDefault(merr), should.Equal("hey"))
	})

	t.Run(`multierror (merge)`, func(t *testing.T) {
		merr := errors.Join(
			stagAdd.ApplyValue(errors.New("hello"), "hey"),
			stagAdd.ApplyValue(errors.New("other"), "there"),
		)

		assert.That(t, stagAdd.ValueOrDefault(merr), should.Equal("heythere"))
	})

	t.Run(`multierror (empty)`, func(t *testing.T) {
		merr := luci_errors.MultiError{}
		assert.That(t, stag.ValueOrDefault(merr), should.Equal("defaultValue"))
	})

	t.Run(`multierror (contains nil)`, func(t *testing.T) {
		merr := luci_errors.MultiError{nil}
		assert.That(t, stag.ValueOrDefault(merr), should.Equal("defaultValue"))
	})

	t.Run(`multierror (none tagged)`, func(t *testing.T) {
		merr := luci_errors.MultiError{errors.New("fleep")}
		assert.That(t, stag.ValueOrDefault(merr), should.Equal("defaultValue"))
	})

	t.Run(`multierror (singular)`, func(t *testing.T) {
		merr := luci_errors.MultiError{
			stag.ApplyValue(errors.New("other"), "there"),
		}

		assert.That(t, stag.ValueOrDefault(merr), should.Equal("there"))
	})

	t.Run(`multierror (one of many)`, func(t *testing.T) {
		merr := luci_errors.MultiError{
			errors.New("hello"),
			errors.New("nerp"),
			errors.New("blerp"),
			stag.ApplyValue(errors.New("other"), "there"),
		}

		assert.That(t, stag.ValueOrDefault(merr), should.Equal("there"))
	})
}
