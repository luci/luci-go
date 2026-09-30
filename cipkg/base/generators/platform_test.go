// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"
)

func matchPlatform(expected Platform) comparison.Func[Platform] {
	return should.Match(expected, cmp.AllowUnexported(Platform{}, environ.Env{}))
}

func TestParsePlatform(t *testing.T) {
	t.Parallel()

	ftt.Run("ParsePlatform", t, func(t *ftt.Test) {
		t.Run("empty", func(t *ftt.Test) {
			p, err := ParsePlatform("")
			assert.NoErr(t, err)
			assert.That(t, p, matchPlatform(UniversalPlatform()))
			assert.That(t, p.IsZero(), should.BeTrue)
		})

		t.Run("valid", func(t *ftt.Test) {
			p, err := ParsePlatform("os=linux,arch=amd64,abi=gnu")
			assert.NoErr(t, err)
			assert.That(t, p.OS(), should.Equal("linux"))
			assert.That(t, p.Arch(), should.Equal("amd64"))
			assert.That(t, p.Get("abi"), should.Equal("gnu"))
			assert.That(t, p.String(), should.Equal("abi=gnu,arch=amd64,os=linux"))

			expected := NewPlatform("linux", "amd64")
			expected.Set("abi", "gnu")
			assert.That(t, p, matchPlatform(expected))
		})

		t.Run("invalid", func(t *ftt.Test) {
			for _, tc := range []string{
				"os",
				"os=",
				"=linux",
				"os=linux,",
				",os=linux",
				"os=linux,,arch=amd64",
				"os=linux,arch",
			} {
				_, err := ParsePlatform(tc)
				assert.ErrIsLike(t, err, "invalid platform attribute")
			}
		})
	})
}
