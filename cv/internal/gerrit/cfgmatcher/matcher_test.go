// Copyright 2020 The LUCI Authors.
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

package cfgmatcher

import (
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestPartitionConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Matcher works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const cfgText = `
			config_groups {
				name: "g1"
				gerrit {
					url: "https://1.example.com"
					projects {
						name: "luci/go"
						ref_regexp:         "refs/heads/.+"
						ref_regexp_exclude: "refs/heads/e.+ed"
					}
					projects {
						name: "default"
						# Default regexp is "refs/heads/master".
					}
				}
			}
			config_groups {
				name: "fallback"
				fallback: YES
				gerrit {
					url: "https://1.example.com"
					projects {
						name: "luci/go"
						ref_regexp:         "refs/heads/.+"
						# NOTE: \\d is for proto parser s.t. actual regexp is just \d
						ref_regexp:         "refs/branch-heads/[\\d]{3,4}"
					}
				}
			}
			config_groups {
				name: "g2"
				gerrit {
					url: "https://1.example.com"
					projects {
						name: "luci/go"
						ref_regexp:         "refs/heads/g2"
					}
				}
				gerrit {
					url: "https://2.example.com"
					projects {
						name: "fo/rk"
						ref_regexp:         "refs/heads/main"
					}
				}
			}`
		cfg := &cfgpb.Config{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cfgText), cfg))

		const luciProject = "luci"
		const gHost1 = "1.example.com"
		const gHost2 = "2.example.com"

		prjcfgtest.Create(ctx, luciProject, cfg)
		meta := prjcfgtest.MustExist(ctx, luciProject)
		hash := meta.Hash()

		ids := func(ids ...string) []prjcfg.ConfigGroupID {
			ret := make([]prjcfg.ConfigGroupID, len(ids))
			for i, id := range ids {
				ret[i] = prjcfg.MakeConfigGroupID(hash, id)
			}
			return ret
		}

		testMatches := func(m *Matcher) {
			t.Run("matching works", func(t *ftt.Test) {
				assert.Loosely(t, m.Match("random.host", "luci/go", "refs/heads/main"), should.BeEmpty)
				assert.Loosely(t, m.Match(gHost1, "random/repo", "refs/heads/main"), should.BeEmpty)
				assert.Loosely(t, m.Match(gHost1, "luci/go", "refs/nei/ther"), should.BeEmpty)

				assert.That(t, m.Match(gHost1, "luci/go", "refs/heads/main"), should.Match(ids("g1")))
				assert.That(t, m.Match(gHost1, "luci/go", "refs/heads/exclud-not-really"), should.Match(ids("g1")))
				assert.That(t, m.Match(gHost1, "luci/go", "refs/heads/excluded"), should.Match(ids("fallback")))

				assert.Loosely(t, m.Match(gHost1, "luci/go", "refs/branch-heads/12"), should.BeEmpty)
				assert.That(t, m.Match(gHost1, "luci/go", "refs/branch-heads/123"), should.Match(ids("fallback")))
				assert.That(t, m.Match(gHost1, "luci/go", "refs/branch-heads/1234"), should.Match(ids("fallback")))
				assert.Loosely(t, m.Match(gHost1, "luci/go", "refs/branch-heads/12345"), should.BeEmpty)

				assert.That(t, m.Match(gHost1, "luci/go", "refs/heads/g2"), should.Match(ids("g1", "g2")))
				assert.That(t, m.Match(gHost1, "luci/go", "refs/heads/g2-not-any-more"), should.Match(ids("g1")))
				// Test default.
				assert.Loosely(t, m.Match(gHost1, "default", "refs/heads/stuff"), should.BeEmpty)
				assert.Loosely(t, m.Match(gHost1, "default", "refs/heads/main"), should.BeEmpty)
				assert.That(t, m.Match(gHost1, "default", "refs/heads/master"), should.Match(ids("g1")))

				// 2nd host with 2 hosts in the same config group.
				assert.That(t, m.Match(gHost2, "fo/rk", "refs/heads/main"), should.Match(ids("g2")))
			})
		}

		t.Run("load from Datstore", func(t *ftt.Test) {
			m, err := LoadMatcher(ctx, luciProject, hash)
			assert.NoErr(t, err)
			testMatches(m)

			t.Run("Serialize/Deserialize", func(t *ftt.Test) {
				bytes, err := m.Serialize()
				assert.NoErr(t, err)
				m, err := Deserialize(bytes)
				assert.NoErr(t, err)
				testMatches(m)
			})
		})
	})
}
