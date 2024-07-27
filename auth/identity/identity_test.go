// Copyright 2015 The LUCI Authors.
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

package identity

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestIdentity(t *testing.T) {
	ftt.Run("MakeIdentity works", t, func(t *ftt.Test) {
		id, err := MakeIdentity("anonymous:anonymous")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal(Identity("anonymous:anonymous")))
		assert.Loosely(t, id.Kind(), should.Equal(Anonymous))
		assert.Loosely(t, id.Value(), should.Equal("anonymous"))
		assert.Loosely(t, id.Email(), should.BeEmpty)

		_, err = MakeIdentity("bad ident")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Validate works", t, func(t *ftt.Test) {
		assert.Loosely(t, Identity("user:abc@example.com").Validate(), should.BeNil)
		assert.Loosely(t, Identity("user:").Validate(), should.NotBeNil)
		assert.Loosely(t, Identity(":abc").Validate(), should.NotBeNil)
		assert.Loosely(t, Identity("abc@example.com").Validate(), should.NotBeNil)
		assert.Loosely(t, Identity("user:abc").Validate(), should.NotBeNil)
	})

	ftt.Run("Kind works", t, func(t *ftt.Test) {
		assert.Loosely(t, Identity("user:abc@example.com").Kind(), should.Equal(User))
		assert.Loosely(t, Identity("???").Kind(), should.Equal(Anonymous))
	})

	ftt.Run("Value works", t, func(t *ftt.Test) {
		assert.Loosely(t, Identity("service:abc").Value(), should.Equal("abc"))
		assert.Loosely(t, Identity("???").Value(), should.Equal("anonymous"))
	})

	ftt.Run("Email works", t, func(t *ftt.Test) {
		assert.Loosely(t, Identity("user:abc@example.com").Email(), should.Equal("abc@example.com"))
		assert.Loosely(t, Identity("service:abc").Email(), should.BeEmpty)
		assert.Loosely(t, Identity("???").Email(), should.BeEmpty)
	})
}
