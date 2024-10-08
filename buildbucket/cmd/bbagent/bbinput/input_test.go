// Copyright 2019 The LUCI Authors.
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

package bbinput

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

func TestInputOK(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect *bbpb.BBAgentArgs
	}{
		{"basic", "eJwDAAAAAAE", &bbpb.BBAgentArgs{}},
		{"stuff", "eJxTElzEyFeSkVmsAESJCiWpxSUANZQF+g", &bbpb.BBAgentArgs{
			Build: &bbpb.Build{
				SummaryMarkdown: "this is a test",
			},
		}},
	}

	ftt.Run(`Parse (ok)`, t, func(t *ftt.Test) {
		for _, tc := range tests {
			t.Run(tc.name, func(t *ftt.Test) {
				ret, err := Parse(tc.input)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Resemble(tc.expect))
			})
		}
	})
}

func TestInputBad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty", "", "inputs required"},
		{"base64", "!!", "decoding base64"},
		{"zlib", "\n", "opening zlib reader"},
		{"decompress", "eJwXAAAAAAE", "decompressing zlib"},
		{"proto", "eJxLSswDQgAITwJi", "parsing proto"},
	}

	ftt.Run(`Parse (err)`, t, func(t *ftt.Test) {
		for _, tc := range tests {
			t.Run(tc.name, func(t *ftt.Test) {
				_, err := Parse(tc.input)
				assert.Loosely(t, err, should.ErrLike(tc.expect))
			})
		}
	})
}
