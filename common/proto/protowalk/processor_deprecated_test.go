// Copyright 2022 The LUCI Authors.
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

package protowalk

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDeprecated(t *testing.T) {
	t.Parallel()

	msg := &Outer{
		Deprecated: "hey",
		SingleInner: &Inner{
			Regular:    "things",
			Deprecated: "yo",
		},
		MapInner: map[string]*Inner{
			"schwoot": {
				Deprecated: "thing",
				SingleEmbed: &Inner_Embedded{
					Deprecated: "yarp",
				},
				MultiEmbed: []*Inner_Embedded{
					{Deprecated: "yay"},
					{Regular: "ignore"},
				},
				Recursive: &Inner_Recursive{
					Regular: 1,
					Next: &Inner_Recursive{
						Regular: 2,
					},
				},
			},
		},
		MultiDeprecated: []*Inner{
			{Regular: "something"},
			{Deprecated: "something else"},
		},
	}
	walker := NewWalker[*Outer](&DeprecatedProcessor{})
	assert.That(t, walker.Execute(msg).Strings(), should.Match([]string{
		`.deprecated: deprecated`,
		`.single_inner.deprecated: deprecated`,
		`.map_inner["schwoot"].deprecated: deprecated`,
		`.map_inner["schwoot"].single_embed.deprecated: deprecated`,
		`.map_inner["schwoot"].multi_embed[0].deprecated: deprecated`,
		`.map_inner["schwoot"].recursive: deprecated`,
		`.multi_deprecated: deprecated`,
		`.multi_deprecated[1].deprecated: deprecated`,
	}))
}
