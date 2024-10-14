// Copyright 2021 The LUCI Authors.
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

package internal

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNormalizeURL(t *testing.T) {
	t.Parallel()

	ftt.Run("Normalizes good URLs", t, func(ctx *ftt.Test) {
		cases := []struct {
			in  string
			out string
		}{
			{"", "/"},
			{"/", "/"},
			{"/?asd=def#blah", "/?asd=def#blah"},
			{"/abc/def", "/abc/def"},
			{"/blah//abc///def/", "/blah/abc/def/"},
			{"/blah/..//./abc/", "/abc/"},
			{"/abc/%2F/def", "/abc/def"},
		}
		for _, c := range cases {
			out, err := NormalizeURL(c.in)
			if err != nil {
				ctx.Logf("Failed while checking %q\n", c.in)
				assert.Loosely(ctx, err, should.BeNil)
			}
			assert.Loosely(ctx, out, should.Equal(c.out))
		}
	})

	ftt.Run("Rejects bad URLs", t, func(ctx *ftt.Test) {
		cases := []string{
			"//",
			"///",
			"://",
			":",
			"http://another/abc/def",
			"abc/def",
			"//host.example.com",
		}
		for _, c := range cases {
			_, err := NormalizeURL(c)
			if err == nil {
				ctx.Logf("Didn't fail while testing %q\n", c)
			}
			assert.Loosely(ctx, err, should.NotBeNil)
		}
	})
}
