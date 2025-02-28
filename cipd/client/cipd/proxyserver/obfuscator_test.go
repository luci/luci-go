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

package proxyserver

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

func TestCASURLObfuscator(t *testing.T) {
	t.Parallel()

	ob := NewCASURLObfuscator()
	obj := &proxypb.ProxiedCASObject{
		SignedUrl: "blah",
	}

	t.Run("OK", func(t *testing.T) {
		url, err := ob.Obfuscate(obj)
		assert.NoErr(t, err)
		assert.That(t, url, should.HavePrefix("http://cipd.local/obj/"))

		back, err := ob.Unobfuscate(url)
		assert.NoErr(t, err)
		assert.That(t, back, should.Match(obj))
	})

	t.Run("Wrong URL", func(t *testing.T) {
		_, err := ob.Unobfuscate("http://storage/obj/aaaaa")
		assert.That(t, err, should.ErrLike("unrecognized URL"))
	})

	t.Run("Bad base64", func(t *testing.T) {
		_, err := ob.Unobfuscate("http://cipd.local/obj/!!!!")
		assert.That(t, err, should.ErrLike("bad base64"))
	})

	t.Run("Wrong key", func(t *testing.T) {
		url, err := ob.Obfuscate(obj)
		assert.NoErr(t, err)

		_, err = NewCASURLObfuscator().Unobfuscate(url)
		assert.That(t, err, should.ErrLike("unrecognized CAS object reference"))
	})
}
