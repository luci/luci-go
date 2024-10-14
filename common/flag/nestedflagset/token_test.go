// Copyright 2016 The LUCI Authors.
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

package nestedflagset

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestToken(t *testing.T) {
	ftt.Run(`An empty token`, t, func(t *ftt.Test) {
		tok := token("")

		t.Run(`When split`, func(t *ftt.Test) {
			name, value := tok.split()

			t.Run(`The "name" field should be zero-valued`, func(t *ftt.Test) {
				assert.Loosely(t, name, should.BeEmpty)
			})

			t.Run(`The "value" field should be zero-valued`, func(t *ftt.Test) {
				assert.Loosely(t, value, should.BeEmpty)
			})
		})
	})

	ftt.Run(`A token whose value is "name"`, t, func(t *ftt.Test) {
		tok := token("name")

		t.Run(`When split`, func(t *ftt.Test) {
			name, value := tok.split()

			t.Run(`The "name" field should be "name"`, func(t *ftt.Test) {
				assert.Loosely(t, name, should.Equal("name"))
			})

			t.Run(`The "value" field should be zero-valued`, func(t *ftt.Test) {
				assert.Loosely(t, value, should.BeEmpty)
			})
		})
	})

	ftt.Run(`A token whose value is "name=value"`, t, func(t *ftt.Test) {
		tok := token("name=value")

		t.Run(`When split`, func(t *ftt.Test) {
			name, value := tok.split()

			t.Run(`The "name" field should be "name"`, func(t *ftt.Test) {
				assert.Loosely(t, name, should.Equal("name"))
			})

			t.Run(`The "value" field should be "value"`, func(t *ftt.Test) {
				assert.Loosely(t, value, should.Equal("value"))
			})
		})
	})

	ftt.Run(`A token whose value is "name=value=1=2=3"`, t, func(t *ftt.Test) {
		tok := token("name=value=1=2=3")

		t.Run(`When split`, func(t *ftt.Test) {
			name, value := tok.split()

			t.Run(`The "name" field should be "name"`, func(t *ftt.Test) {
				assert.Loosely(t, name, should.Equal("name"))
			})

			t.Run(`The "value" field should "value=1=2=3"`, func(t *ftt.Test) {
				assert.Loosely(t, value, should.Equal("value=1=2=3"))
			})
		})
	})
}
