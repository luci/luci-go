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

package projectconfig

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/filter/count"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Environment", t, func(t *ftt.Test) {
		c := gaetesting.TestingContext()
		datastore.GetTestable(c).Consistent(true)

		t.Run("Send update", func(t *ftt.Test) {
			c := cfgclient.Use(c, memcfg.New(mockedConfigs))
			assert.Loosely(t, UpdateProjects(c), should.BeNil)

			t.Run("Check created Project entities", func(t *ftt.Test) {
				foo := &Project{ID: "foo"}
				assert.Loosely(t, datastore.Get(c, foo), should.BeNil)
				assert.Loosely(t, foo.HasConfig, should.BeTrue)
				assert.Loosely(t, foo.ACL, should.Match(ACL{
					Groups:     []string{"a", "b"},
					Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
				}))

				bar := &Project{ID: "bar"}
				assert.Loosely(t, datastore.Get(c, bar), should.BeNil)
				assert.Loosely(t, bar.HasConfig, should.BeTrue)
				assert.Loosely(t, bar.ACL, should.Match(ACL{}))

				baz := &Project{ID: "baz"}
				assert.Loosely(t, datastore.Get(c, baz), should.BeNil)
				assert.Loosely(t, baz.HasConfig, should.BeFalse)
				assert.Loosely(t, baz.ACL, should.Match(ACL{
					Groups: []string{"a"},
				}))

				external := &Project{ID: "external"}
				assert.Loosely(t, datastore.Get(c, external), should.BeNil)
				assert.Loosely(t, external.HasConfig, should.BeTrue)
				assert.Loosely(t, external.ACL, should.Match(ACL{
					Identities: []identity.Identity{"user:a@example.com", "user:e@example.com"},
				}))
			})

			t.Run("Check Console config updated", func(t *ftt.Test) {
				cs, err := GetConsole(c, "foo", "default")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cs.ID, should.Equal("default"))
				assert.Loosely(t, cs.Ordinal, should.BeZero)
				assert.Loosely(t, cs.Def.Header, should.BeNil)
			})

			t.Run("Check Console config updated with header", func(t *ftt.Test) {
				cs, err := GetConsole(c, "foo", "default_header")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cs.ID, should.Equal("default_header"))
				assert.Loosely(t, cs.Ordinal, should.Equal(1))
				assert.Loosely(t, cs.Def.Header.Id, should.Equal("main_header"))
				assert.Loosely(t, cs.Def.Header.TreeStatusHost, should.Equal("blarg.example.com"))
			})

			t.Run("Check Console config updated with realm", func(t *ftt.Test) {
				cs, err := GetConsole(c, "foo", "realm_test_console")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cs.ID, should.Equal("realm_test_console"))
				assert.Loosely(t, cs.Ordinal, should.Equal(2))
				assert.Loosely(t, cs.Realm, should.Equal("foo:fake_realm"))
			})

			t.Run("Check Console config updated with builder ID", func(t *ftt.Test) {
				cs, err := GetConsole(c, "foo", "default_header")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cs.Def.Builders, should.Match([]*projectconfigpb.Builder{
					{
						Id: &buildbucketpb.BuilderID{
							Project: "foo",
							Bucket:  "something",
							Builder: "bar",
						},
						Name:      "buildbucket/luci.foo.something/bar",
						Category:  "main|something",
						ShortName: "s",
					},
					{
						Id: &buildbucketpb.BuilderID{
							Project: "foo",
							Bucket:  "other",
							Builder: "baz",
						},
						Name:      "buildbucket/luci.foo.other/baz",
						Category:  "main|other",
						ShortName: "o",
					},
				}))
			})

			t.Run("Check external Console is resolved", func(t *ftt.Test) {
				cs, err := GetConsole(c, "external", "foo-default")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cs.Ordinal, should.BeZero)
				assert.Loosely(t, cs.ID, should.Equal("foo-default"))
				assert.Loosely(t, cs.Def.Id, should.Equal("foo-default"))
				assert.Loosely(t, cs.Def.Name, should.Equal("foo default"))
				assert.Loosely(t, cs.Def.ExternalProject, should.Equal("foo"))
				assert.Loosely(t, cs.Def.ExternalId, should.Equal("default"))
				assert.Loosely(t, cs.Builders, should.Match([]string{"buildbucket/luci.foo.something/bar", "buildbucket/luci.foo.other/baz"}))
			})

			t.Run("Check user can see external consoles they have access to", func(t *ftt.Test) {
				cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
				cs, err := GetProjectConsoles(cUser, "external")
				assert.Loosely(t, err, should.BeNil)

				ids := make([]string, 0, len(cs))
				for _, c := range cs {
					ids = append(ids, c.ID)
				}
				assert.Loosely(t, ids, should.Match([]string{"foo-default"}))
			})

			t.Run("Check user can't see external consoles they don't have access to", func(t *ftt.Test) {
				cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:e@example.com"})
				cs, err := GetProjectConsoles(cUser, "external")
				assert.Loosely(t, err, should.BeNil)

				ids := make([]string, 0, len(cs))
				for _, c := range cs {
					ids = append(ids, c.ID)
				}
				assert.Loosely(t, ids, should.HaveLength(0))
			})

			t.Run("Check second update reorders", func(t *ftt.Test) {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsUpdate))
				assert.Loosely(t, UpdateProjects(c), should.BeNil)

				t.Run("Check updated Project entities", func(t *ftt.Test) {
					foo := &Project{ID: "foo"}
					assert.Loosely(t, datastore.Get(c, foo), should.BeNil)
					assert.Loosely(t, foo.HasConfig, should.BeTrue)
					assert.Loosely(t, foo.ACL, should.Match(ACL{
						Identities: []identity.Identity{"user:a@example.com"},
					}))

					bar := &Project{ID: "bar"}
					assert.Loosely(t, datastore.Get(c, bar), should.BeNil)
					assert.Loosely(t, bar.HasConfig, should.BeFalse)
					assert.Loosely(t, bar.ACL, should.Match(ACL{}))

					assert.Loosely(t, datastore.Get(c, &Project{ID: "baz"}), should.Equal(datastore.ErrNoSuchEntity))
				})

				t.Run("Check Console config removed", func(t *ftt.Test) {
					cs, err := GetConsole(c, "foo", "default")
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, cs, should.BeNil)
				})

				t.Run("Check builder group configs in correct order", func(t *ftt.Test) {
					cs, err := GetConsole(c, "foo", "default_header")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cs.ID, should.Equal("default_header"))
					assert.Loosely(t, cs.Ordinal, should.BeZero)
					assert.Loosely(t, cs.Def.Header.Id, should.Equal("main_header"))
					assert.Loosely(t, cs.Def.Header.TreeStatusHost, should.Equal("blarg.example.com"))
					cs, err = GetConsole(c, "foo", "console.bar")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cs.ID, should.Equal("console.bar"))
					assert.Loosely(t, cs.Ordinal, should.Equal(1))
					assert.Loosely(t, cs.Builders, should.Match([]string{"buildbucket/luci.foo.something/bar"}))

					cs, err = GetConsole(c, "foo", "console.baz")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cs.ID, should.Equal("console.baz"))
					assert.Loosely(t, cs.Ordinal, should.Equal(2))
					assert.Loosely(t, cs.Builders, should.Match([]string{"buildbucket/luci.foo.other/baz"}))
				})

				t.Run("Check getting project builder groups in correct order", func(t *ftt.Test) {
					cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
					cs, err := GetProjectConsoles(cUser, "foo")
					assert.Loosely(t, err, should.BeNil)

					ids := make([]string, 0, len(cs))
					for _, c := range cs {
						ids = append(ids, c.ID)
					}
					assert.Loosely(t, ids, should.Match([]string{"default_header", "console.bar", "console.baz"}))
				})
			})

			t.Run("Check removing Milo config only", func(t *ftt.Test) {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsNoConsole))
				assert.Loosely(t, UpdateProjects(c), should.BeNil)

				t.Run("Check kept the Project entity", func(t *ftt.Test) {
					foo := &Project{ID: "foo"}
					assert.Loosely(t, datastore.Get(c, foo), should.BeNil)
					assert.Loosely(t, foo.HasConfig, should.BeFalse)
					assert.Loosely(t, foo.ACL, should.Match(ACL{
						Groups:     []string{"a", "b"},
						Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
					}))
				})

				t.Run("Check removed the console", func(t *ftt.Test) {
					cs, err := GetConsole(c, "foo", "default")
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, cs, should.BeNil)
				})
			})

			t.Run("Check applying broken config", func(t *ftt.Test) {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsBroken))
				assert.Loosely(t, UpdateProjects(c), should.NotBeNil)

				t.Run("Check kept the Project entity", func(t *ftt.Test) {
					foo := &Project{ID: "foo"}
					assert.Loosely(t, datastore.Get(c, foo), should.BeNil)
					assert.Loosely(t, foo.HasConfig, should.BeTrue)
					assert.Loosely(t, foo.ACL, should.Match(ACL{
						Groups:     []string{"a", "b"},
						Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
					}))
				})

				t.Run("Check kept the console", func(t *ftt.Test) {
					_, err := GetConsole(c, "foo", "default")
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})
	})
}

var fooCfg = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "default"
	name: "default"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/main"
	manifest_name: "REVISION"
	builders: {
		id: {
			project: "foo"
			bucket: "something"
			builder: "bar"
		}
		category: "main|something"
		short_name: "s"
	}
	builders: {
		id: {
			project: "foo"
			bucket: "other"
			builder: "baz"
		}
		category: "main|other"
		short_name: "o"
	}
}
consoles: {
	id: "default_header"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "regexp:refs/heads/also-ok"
	manifest_name: "REVISION"
	builders: {
		id: {
			project: "foo"
			bucket: "something"
			builder: "bar"
		}
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
	header_id: "main_header"
}
consoles: {
	id: "realm_test_console"
	name: "realm_test"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/main"
	realm: "foo:fake_realm"
	manifest_name: "REVISION"
}
metadata_config: {
	test_metadata_properties: {
		schema: "package.name"
		display_items: {
			display_name: "owners"
			path: "owners.email"
		}
	}
}
`

var fooProjectCfg = `
access: "a@example.com"
access: "user:a@example.com"
access: "user:b@example.com"
access: "group:a"
access: "group:a"
access: "group:b"
`

var bazProjectCfg = `
access: "group:a"
`

var fooCfg2 = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "default_header"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/main"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
	header_id: "main_header"
}
consoles: {
	id: "console.bar"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/main"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
}
consoles: {
	id: "console.baz"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/main"
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
}
metadata_config: {
	test_metadata_properties: {
		schema: "package.name"
		display_items: {
			display_name: "owners"
			path: "owners.email"
		}
	}
}
`

var fooProjectCfg2 = `
access: "a@example.com"
`

var externalConsoleCfg = `
consoles: {
	id: "foo-default"
	name: "foo default"
	external_project: "foo"
	external_id: "default"
}
`

var externalProjectCfg = `
access: "a@example.com"
access: "e@example.com"
`

var badConsoleCfg = `
consoles: {
	id: "baz"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/main"
	manifest_name: "REVISION"
	builders: {
		name: ""
	}
	builders: {
		name: "bad/scheme"
	}
	builders: {
		id: {
			project: ""
			bucket: "bucket"
			builder: "builder"
		}
	}
}
`

var mockedConfigs = map[config.Set]memcfg.Files{
	"projects/foo": {
		"${appid}.cfg": fooCfg,
		"project.cfg":  fooProjectCfg,
	},
	"projects/bar": {
		"${appid}.cfg": ``, // empty, but present
		"project.cfg":  ``,
	},
	"projects/baz": {
		// no Milo config
		"project.cfg": bazProjectCfg,
	},
	"projects/external": {
		"${appid}.cfg": externalConsoleCfg,
		"project.cfg":  externalProjectCfg,
	},
}

var mockedConfigsUpdate = map[config.Set]memcfg.Files{
	"projects/foo": {
		"${appid}.cfg": fooCfg2,
		"project.cfg":  fooProjectCfg2,
	},
	"projects/bar": {
		// No milo config any more
		"project.cfg": ``,
	},
	// No project/baz anymore.
}

// A copy of mockedConfigs with projects/foo and projects/external Milo configs
// removed.
var mockedConfigsNoConsole = map[config.Set]memcfg.Files{
	"projects/foo": {
		"project.cfg": fooProjectCfg,
	},
	"projects/bar": {
		"${appid}.cfg": ``, // empty, but present
		"project.cfg":  ``,
	},
	"projects/baz": {
		// no Milo config
		"project.cfg": bazProjectCfg,
	},
	"projects/external": {
		"project.cfg": externalProjectCfg,
	},
}

// A copy of mockedConfigs with projects/foo broken.
var mockedConfigsBroken = map[config.Set]memcfg.Files{
	"projects/foo": {
		"${appid}.cfg": `broken milo config file`,
		"project.cfg":  fooProjectCfg,
	},
	"projects/bar": {
		"${appid}.cfg": ``, // empty, but present
		"project.cfg":  ``,
	},
	"projects/baz": {
		// no Milo config
		"project.cfg": bazProjectCfg,
	},
	"projects/external": {
		"${appid}.cfg": externalConsoleCfg,
		"project.cfg":  externalProjectCfg,
	},
}

// TestOldProtosStillReadable creates a datastore record (in the emulator) using the LegacyConsole type
// and then tries to read it using the Console type.
//
// The field that was once `Def projectconfigpb.Console` became `Def *projectconfigpb.Console` and this
// test, in some small way, makes sure that that path continues to work.
func TestOldProtosStillReadable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	mustMarshal := func(msg proto.Message) []byte {
		b, err := proto.Marshal(msg)
		if err != nil {
			panic(err)
		}
		return b
	}

	// LegacyConsole deliberately uses the old encoding for the proto projectconfigpb.Console.
	//
	// This test ensures that old records stay readable.
	type LegacyConsole struct {
		Kind           string `gae:"$kind,Console"`
		ID             string `gae:"$id"`
		ConfigRevision string `gae:",noindex"`
		EncodedProto   []byte `gae:"Def,noindex"`
	}

	legacyConsole := &LegacyConsole{
		ID:             "moneycat",
		ConfigRevision: "fake config revision",
		EncodedProto: mustMarshal(&projectconfigpb.Console{
			RepoUrl: "fake repo url",
		}),
	}

	expected := &Console{
		ID:             "moneycat",
		ConfigRevision: "fake config revision",
		Def: &projectconfigpb.Console{
			RepoUrl: "fake repo url",
		},
	}

	if err := datastore.Put(ctx, legacyConsole); err != nil {
		t.Errorf("failed to put: %s", err)
	}

	actual := &Console{ID: "moneycat"}

	if err := datastore.Get(ctx, actual); err != nil {
		t.Errorf("failed to get: %s", err)
	}

	assert.That(t, actual, should.Match(expected))
}

func makeConsoleCfgText(id string, numBuilders int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "consoles: {\n\tid: %q\n\tbuilder_view_only: true\n", id)
	for i := range numBuilders {
		fmt.Fprintf(&sb, "\tbuilders: {\n\t\tname: \"buildbucket/luci.large.ci/builder-%s-%d\"\n\t}\n", id, i)
	}
	sb.WriteString("}\n")
	return sb.String()
}

func TestBatchConsoles(t *testing.T) {
	t.Parallel()

	makeResolved := func(builderCounts ...int) []resolvedConsole {
		out := make([]resolvedConsole, len(builderCounts))
		for i, n := range builderCounts {
			builders := make([]*projectconfigpb.Builder, n)
			for j := range n {
				builders[j] = &projectconfigpb.Builder{Name: fmt.Sprintf("b-%d", j)}
			}
			out[i] = resolvedConsole{
				ordinal: i,
				def: &projectconfigpb.Console{
					Id:       fmt.Sprintf("c-%d", i),
					Builders: builders,
				},
			}
		}
		return out
	}

	ftt.Run("batchConsoles", t, func(t *ftt.Test) {
		t.Run("Empty input returns nil", func(t *ftt.Test) {
			assert.Loosely(t, batchConsoles(nil, 64, 4000), should.BeNil)
		})

		t.Run("Splits by maxConsoles", func(t *ftt.Test) {
			counts := make([]int, 150)
			for i := range counts {
				counts[i] = 1
			}
			batches := batchConsoles(makeResolved(counts...), 64, 4000)
			assert.Loosely(t, batches, should.HaveLength(3))
			assert.Loosely(t, batches[0], should.HaveLength(64))
			assert.Loosely(t, batches[1], should.HaveLength(64))
			assert.Loosely(t, batches[2], should.HaveLength(22))
		})

		t.Run("Splits by maxBuilders", func(t *ftt.Test) {
			batches := batchConsoles(makeResolved(1500, 1500, 1500, 1500), 64, 4000)
			assert.Loosely(t, batches, should.HaveLength(2))
			assert.Loosely(t, batches[0], should.HaveLength(2))
			assert.Loosely(t, batches[1], should.HaveLength(2))
		})

		t.Run("Isolates single console exceeding maxBuilders", func(t *ftt.Test) {
			batches := batchConsoles(makeResolved(5000, 500), 64, 4000)
			assert.Loosely(t, batches, should.HaveLength(2))
			assert.Loosely(t, batches[0], should.HaveLength(1))
			assert.Loosely(t, batches[0][0].ordinal, should.Equal(0))
			assert.Loosely(t, batches[1], should.HaveLength(1))
			assert.Loosely(t, batches[1][0].ordinal, should.Equal(1))
		})
	})
}

func TestUpdateProjectsBatching(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateProjects batching", t, func(t *ftt.Test) {
		t.Run("Batches large builder counts and skips unchanged consoles on update", func(t *ftt.Test) {
			c := gaetesting.TestingContext()
			datastore.GetTestable(c).Consistent(true)

			// 5 consoles * 1,500 builders = 7,500 builders (> maxBuildersPerBatch = 4,000),
			// partitioning into 3 batches: [c0, c1], [c2, c3], [c4].
			var initialCfg strings.Builder
			for i := range 5 {
				initialCfg.WriteString(makeConsoleCfgText(fmt.Sprintf("c%d", i), 1500))
			}

			cInitial, counterInitial := count.FilterRDS(cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/large": {
					"${appid}.cfg": initialCfg.String(),
					"project.cfg":  `access: "a@example.com"`,
				},
			})))
			assert.Loosely(t, UpdateProjects(cInitial), should.BeNil)
			assert.Loosely(t, counterInitial.RunInTransaction.Successes(), should.Equal(3))

			c0Before, err := GetConsole(c, "large", "c0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c0Before.Ordinal, should.Equal(0))
			assert.Loosely(t, c0Before.Builders, should.HaveLength(1500))
			initialRev := c0Before.ConfigRevision

			// Insert a new console at index 1, shifting c1..c4 to ordinals 2..5
			// while keeping c0 unchanged at ordinal 0.
			var shiftedCfg strings.Builder
			shiftedCfg.WriteString(makeConsoleCfgText("c0", 1500))
			shiftedCfg.WriteString(makeConsoleCfgText("c_inserted", 1500))
			for i := 1; i < 5; i++ {
				shiftedCfg.WriteString(makeConsoleCfgText(fmt.Sprintf("c%d", i), 1500))
			}

			cShifted, counterShifted := count.FilterRDS(cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/large": {
					"${appid}.cfg": shiftedCfg.String(),
					"project.cfg":  `access: "a@example.com"`,
				},
			})))
			assert.Loosely(t, UpdateProjects(cShifted), should.BeNil)
			assert.Loosely(t, counterShifted.RunInTransaction.Successes(), should.Equal(3))

			// c0 did not change definition or ordinal, so it was not rewritten.
			c0After, err := GetConsole(c, "large", "c0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c0After.Ordinal, should.Equal(0))
			assert.Loosely(t, c0After.ConfigRevision, should.Equal(initialRev))

			// c_inserted and shifted consoles c1..c4 were written with updated ordinals.
			inserted, err := GetConsole(c, "large", "c_inserted")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inserted.Ordinal, should.Equal(1))
			assert.Loosely(t, inserted.ConfigRevision, should.NotEqual(initialRev))

			for i := 1; i < 5; i++ {
				con, err := GetConsole(c, "large", fmt.Sprintf("c%d", i))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, con.Ordinal, should.Equal(i+1))
				assert.Loosely(t, con.ConfigRevision, should.Equal(inserted.ConfigRevision))
			}

			// Modify only the last console (c4 at ordinal 5); batches 0 and 1 have
			// zero changed consoles and issue no PutMulti calls.
			var tailEditCfg strings.Builder
			tailEditCfg.WriteString(makeConsoleCfgText("c0", 1500))
			tailEditCfg.WriteString(makeConsoleCfgText("c_inserted", 1500))
			for i := 1; i < 4; i++ {
				tailEditCfg.WriteString(makeConsoleCfgText(fmt.Sprintf("c%d", i), 1500))
			}
			tailEditCfg.WriteString(makeConsoleCfgText("c4", 1501))

			cTail, counterTail := count.FilterRDS(cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/large": {
					"${appid}.cfg": tailEditCfg.String(),
					"project.cfg":  `access: "a@example.com"`,
				},
			})))
			assert.Loosely(t, UpdateProjects(cTail), should.BeNil)
			assert.Loosely(t, counterTail.RunInTransaction.Successes(), should.Equal(3))
			assert.Loosely(t, counterTail.PutMulti.Successes(), should.Equal(1))

			c4After, err := GetConsole(c, "large", "c4")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c4After.Builders, should.HaveLength(1501))
		})

		t.Run("Batches large console counts (> maxConsolesPerBatch)", func(t *ftt.Test) {
			c := gaetesting.TestingContext()
			datastore.GetTestable(c).Consistent(true)

			// 130 consoles (> 2 * 64) -> 3 batches (64, 64, 2).
			var manyCfg strings.Builder
			wantIDs := make([]string, 130)
			for i := range 130 {
				id := fmt.Sprintf("con-%03d", i)
				wantIDs[i] = id
				manyCfg.WriteString(makeConsoleCfgText(id, 1))
			}

			cMany, counter := count.FilterRDS(cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/many": {
					"${appid}.cfg": manyCfg.String(),
					"project.cfg":  `access: "a@example.com"`,
				},
			})))
			assert.Loosely(t, UpdateProjects(cMany), should.BeNil)
			assert.Loosely(t, counter.RunInTransaction.Successes(), should.Equal(3))

			cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
			consoles, err := GetProjectConsoles(cUser, "many")
			assert.Loosely(t, err, should.BeNil)
			gotIDs := make([]string, len(consoles))
			for i, con := range consoles {
				gotIDs[i] = con.ID
			}
			assert.Loosely(t, gotIDs, should.Match(wantIDs))
		})

		t.Run("Updates external console when upstream project changes", func(t *ftt.Test) {
			c := gaetesting.TestingContext()
			datastore.GetTestable(c).Consistent(true)

			downstreamCfg := `
consoles: {
	id: "ext"
	name: "ext"
	external_project: "upstream"
	external_id: "main"
}
`
			c1 := cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/upstream": {
					"${appid}.cfg": makeConsoleCfgText("main", 1),
					"project.cfg":  `access: "a@example.com"`,
				},
				"projects/downstream": {
					"${appid}.cfg": downstreamCfg,
					"project.cfg":  `access: "a@example.com"`,
				},
			}))
			assert.Loosely(t, UpdateProjects(c1), should.BeNil)

			extBefore, err := GetConsole(c, "downstream", "ext")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, extBefore.Builders, should.HaveLength(1))

			// Update upstream without changing downstream's config revision.
			c2 := cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/upstream": {
					"${appid}.cfg": makeConsoleCfgText("main", 2),
					"project.cfg":  `access: "a@example.com"`,
				},
				"projects/downstream": {
					"${appid}.cfg": downstreamCfg,
					"project.cfg":  `access: "a@example.com"`,
				},
			}))
			assert.Loosely(t, UpdateProjects(c2), should.BeNil)

			extAfter, err := GetConsole(c, "downstream", "ext")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, extAfter.Builders, should.HaveLength(2))
		})

		t.Run("Fails upfront without partial writes if later batch has invalid external console", func(t *ftt.Test) {
			c := gaetesting.TestingContext()
			datastore.GetTestable(c).Consistent(true)

			var brokenCfg strings.Builder
			for i := range 65 {
				brokenCfg.WriteString(makeConsoleCfgText(fmt.Sprintf("con-%03d", i), 1))
			}
			brokenCfg.WriteString(`
consoles: {
	id: "bad-ext"
	name: "bad-ext"
	external_project: "nonexistent"
	external_id: "main"
}
`)
			cBroken := cfgclient.Use(c, memcfg.New(map[config.Set]memcfg.Files{
				"projects/broken": {
					"${appid}.cfg": brokenCfg.String(),
					"project.cfg":  `access: "a@example.com"`,
				},
			}))
			assert.Loosely(t, UpdateProjects(cBroken), should.NotBeNil)

			// First batch must not have been partially committed.
			_, err := GetConsole(c, "broken", "con-000")
			assert.Loosely(t, err, should.Equal(ErrConsoleNotFound))
		})
	})
}
