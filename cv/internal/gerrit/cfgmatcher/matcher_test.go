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

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPartitionConfig(t *testing.T) {
	t.Parallel()

	Convey("Matcher works", t, func() {
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
		So(prototext.Unmarshal([]byte(cfgText), cfg), ShouldBeNil)

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
			Convey("matching works", func() {
				So(m.Match("random.host", "luci/go", "refs/heads/main"), ShouldBeEmpty)
				So(m.Match(gHost1, "random/repo", "refs/heads/main"), ShouldBeEmpty)
				So(m.Match(gHost1, "luci/go", "refs/nei/ther"), ShouldBeEmpty)

				So(m.Match(gHost1, "luci/go", "refs/heads/main"), ShouldResemble, ids("g1"))
				So(m.Match(gHost1, "luci/go", "refs/heads/exclud-not-really"), ShouldResemble, ids("g1"))
				So(m.Match(gHost1, "luci/go", "refs/heads/excluded"), ShouldResemble, ids("fallback"))

				So(m.Match(gHost1, "luci/go", "refs/branch-heads/12"), ShouldBeEmpty)
				So(m.Match(gHost1, "luci/go", "refs/branch-heads/123"), ShouldResemble, ids("fallback"))
				So(m.Match(gHost1, "luci/go", "refs/branch-heads/1234"), ShouldResemble, ids("fallback"))
				So(m.Match(gHost1, "luci/go", "refs/branch-heads/12345"), ShouldBeEmpty)

				So(m.Match(gHost1, "luci/go", "refs/heads/g2"), ShouldResemble, ids("g1", "g2"))
				So(m.Match(gHost1, "luci/go", "refs/heads/g2-not-any-more"), ShouldResemble, ids("g1"))
				// Test default.
				So(m.Match(gHost1, "default", "refs/heads/stuff"), ShouldBeEmpty)
				So(m.Match(gHost1, "default", "refs/heads/main"), ShouldBeEmpty)
				So(m.Match(gHost1, "default", "refs/heads/master"), ShouldResemble, ids("g1"))

				// 2nd host with 2 hosts in the same config group.
				So(m.Match(gHost2, "fo/rk", "refs/heads/main"), ShouldResemble, ids("g2"))
			})
		}

		Convey("load from Datstore", func() {
			m, err := LoadMatcher(ctx, luciProject, hash)
			So(err, ShouldBeNil)
			testMatches(m)

			Convey("Serialize/Deserialize", func() {
				bytes, err := m.Serialize()
				So(err, ShouldBeNil)
				m, err := Deserialize(bytes)
				So(err, ShouldBeNil)
				testMatches(m)
			})
		})
	})
}
