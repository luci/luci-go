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
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
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
				assert.Loosely(t, foo.ACL, should.Resemble(ACL{
					Groups:     []string{"a", "b"},
					Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
				}))

				bar := &Project{ID: "bar"}
				assert.Loosely(t, datastore.Get(c, bar), should.BeNil)
				assert.Loosely(t, bar.HasConfig, should.BeTrue)
				assert.Loosely(t, bar.ACL, should.Resemble(ACL{}))

				baz := &Project{ID: "baz"}
				assert.Loosely(t, datastore.Get(c, baz), should.BeNil)
				assert.Loosely(t, baz.HasConfig, should.BeFalse)
				assert.Loosely(t, baz.ACL, should.Resemble(ACL{
					Groups: []string{"a"},
				}))

				external := &Project{ID: "external"}
				assert.Loosely(t, datastore.Get(c, external), should.BeNil)
				assert.Loosely(t, external.HasConfig, should.BeTrue)
				assert.Loosely(t, external.ACL, should.Resemble(ACL{
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
				assert.Loosely(t, cs.Def.Builders, should.Resemble([]*projectconfigpb.Builder{
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
				assert.Loosely(t, cs.Builders, should.Resemble([]string{"buildbucket/luci.foo.something/bar", "buildbucket/luci.foo.other/baz"}))
			})

			t.Run("Check user can see external consoles they have access to", func(t *ftt.Test) {
				cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
				cs, err := GetProjectConsoles(cUser, "external")
				assert.Loosely(t, err, should.BeNil)

				ids := make([]string, 0, len(cs))
				for _, c := range cs {
					ids = append(ids, c.ID)
				}
				assert.Loosely(t, ids, should.Resemble([]string{"foo-default"}))
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
					assert.Loosely(t, foo.ACL, should.Resemble(ACL{
						Identities: []identity.Identity{"user:a@example.com"},
					}))

					bar := &Project{ID: "bar"}
					assert.Loosely(t, datastore.Get(c, bar), should.BeNil)
					assert.Loosely(t, bar.HasConfig, should.BeFalse)
					assert.Loosely(t, bar.ACL, should.Resemble(ACL{}))

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
					assert.Loosely(t, cs.Builders, should.Resemble([]string{"buildbucket/luci.foo.something/bar"}))

					cs, err = GetConsole(c, "foo", "console.baz")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, cs.ID, should.Equal("console.baz"))
					assert.Loosely(t, cs.Ordinal, should.Equal(2))
					assert.Loosely(t, cs.Builders, should.Resemble([]string{"buildbucket/luci.foo.other/baz"}))
				})

				t.Run("Check getting project builder groups in correct order", func(t *ftt.Test) {
					cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
					cs, err := GetProjectConsoles(cUser, "foo")
					assert.Loosely(t, err, should.BeNil)

					ids := make([]string, 0, len(cs))
					for _, c := range cs {
						ids = append(ids, c.ID)
					}
					assert.Loosely(t, ids, should.Resemble([]string{"default_header", "console.bar", "console.baz"}))
				})
			})

			t.Run("Check removing Milo config only", func(t *ftt.Test) {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsNoConsole))
				assert.Loosely(t, UpdateProjects(c), should.BeNil)

				t.Run("Check kept the Project entity", func(t *ftt.Test) {
					foo := &Project{ID: "foo"}
					assert.Loosely(t, datastore.Get(c, foo), should.BeNil)
					assert.Loosely(t, foo.HasConfig, should.BeFalse)
					assert.Loosely(t, foo.ACL, should.Resemble(ACL{
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
					assert.Loosely(t, foo.ACL, should.Resemble(ACL{
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
