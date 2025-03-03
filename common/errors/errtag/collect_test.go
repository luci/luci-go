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
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCollect(t *testing.T) {
	t.Parallel()

	t.Run(`nil error`, func(t *testing.T) {
		assert.Loosely(t, errtag.Collect(nil), should.BeNil)
	})

	t.Run(`untagged error`, func(t *testing.T) {
		assert.Loosely(t, errtag.Collect(errors.New("bob")), should.BeNil)
	})

	t.Run(`btag error`, func(t *testing.T) {
		assert.Loosely(t, errtag.Collect(btag.Apply(errors.New("bob"))), should.Match(errtag.CollectedValues{
			btag.Key(): true,
		}))
	})

	t.Run(`multierror`, func(t *testing.T) {
		err := errors.Join(
			btag.Apply(errors.New("a")),
			btag.ApplyValue(errors.New("b"), false),
			stag.ApplyValue(errors.New("c"), "nerple"),
		)
		collection := errtag.Collect(err)
		assert.Loosely(t, collection, should.Match(errtag.CollectedValues{
			// 'pick first'
			btag.Key(): true,
			stag.Key(): "nerple",
		}))

		t.Run(`format`, func(t *testing.T) {
			assert.That(t, collection.String(), should.Match(strings.Join(
				[]string{
					`boolish: true`,
					`stringy: "nerple"`,
				},
				"\n",
			)))
		})
	})

	t.Run(`exclude`, func(t *testing.T) {
		err := errors.Join(
			btag.Apply(errors.New("a")),
			btag.ApplyValue(errors.New("b"), false),
			stag.ApplyValue(errors.New("c"), "nerple"),
		)
		assert.Loosely(t, errtag.Collect(err, btag), should.Match(errtag.CollectedValues{
			stag.Key(): "nerple",
		}))
	})
}
