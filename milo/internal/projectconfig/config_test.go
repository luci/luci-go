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

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := gaetesting.TestingContext()
		datastore.GetTestable(c).Consistent(true)

		Convey("Send update", func() {
			c := cfgclient.Use(c, memcfg.New(mockedConfigs))
			So(UpdateProjects(c), ShouldBeNil)

			Convey("Check created Project entities", func() {
				foo := &Project{ID: "foo"}
				So(datastore.Get(c, foo), ShouldBeNil)
				So(foo.HasConfig, ShouldBeTrue)
				So(foo.ACL, ShouldResemble, ACL{
					Groups:     []string{"a", "b"},
					Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
				})

				bar := &Project{ID: "bar"}
				So(datastore.Get(c, bar), ShouldBeNil)
				So(bar.HasConfig, ShouldBeTrue)
				So(bar.ACL, ShouldResemble, ACL{})

				baz := &Project{ID: "baz"}
				So(datastore.Get(c, baz), ShouldBeNil)
				So(baz.HasConfig, ShouldBeFalse)
				So(baz.ACL, ShouldResemble, ACL{
					Groups: []string{"a"},
				})

				external := &Project{ID: "external"}
				So(datastore.Get(c, external), ShouldBeNil)
				So(external.HasConfig, ShouldBeTrue)
				So(external.ACL, ShouldResemble, ACL{
					Identities: []identity.Identity{"user:a@example.com", "user:e@example.com"},
				})
			})

			Convey("Check Console config updated", func() {
				cs, err := GetConsole(c, "foo", "default")
				So(err, ShouldBeNil)
				So(cs.ID, ShouldEqual, "default")
				So(cs.Ordinal, ShouldEqual, 0)
				So(cs.Def.Header, ShouldBeNil)
			})

			Convey("Check Console config updated with header", func() {
				cs, err := GetConsole(c, "foo", "default_header")
				So(err, ShouldBeNil)
				So(cs.ID, ShouldEqual, "default_header")
				So(cs.Ordinal, ShouldEqual, 1)
				So(cs.Def.Header.Id, ShouldEqual, "main_header")
				So(cs.Def.Header.TreeStatusHost, ShouldEqual, "blarg.example.com")
			})

			Convey("Check Console config updated with realm", func() {
				cs, err := GetConsole(c, "foo", "realm_test_console")
				So(err, ShouldBeNil)
				So(cs.ID, ShouldEqual, "realm_test_console")
				So(cs.Ordinal, ShouldEqual, 2)
				So(cs.Realm, ShouldEqual, "foo:fake_realm")
			})

			Convey("Check external Console is resolved", func() {
				cs, err := GetConsole(c, "external", "foo-default")
				So(err, ShouldBeNil)
				So(cs.Ordinal, ShouldEqual, 0)
				So(cs.ID, ShouldEqual, "foo-default")
				So(cs.Def.Id, ShouldEqual, "foo-default")
				So(cs.Def.Name, ShouldEqual, "foo default")
				So(cs.Def.ExternalProject, ShouldEqual, "foo")
				So(cs.Def.ExternalId, ShouldEqual, "default")
				So(cs.Builders, ShouldResemble, []string{"buildbucket/luci.foo.something/bar", "buildbucket/luci.foo.other/baz"})
			})

			Convey("Check user can see external consoles they have access to", func() {
				cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
				cs, err := GetProjectConsoles(cUser, "external")
				So(err, ShouldBeNil)

				ids := make([]string, 0, len(cs))
				for _, c := range cs {
					ids = append(ids, c.ID)
				}
				So(ids, ShouldResemble, []string{"foo-default"})
			})

			Convey("Check user can't see external consoles they don't have access to", func() {
				cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:e@example.com"})
				cs, err := GetProjectConsoles(cUser, "external")
				So(err, ShouldBeNil)

				ids := make([]string, 0, len(cs))
				for _, c := range cs {
					ids = append(ids, c.ID)
				}
				So(ids, ShouldHaveLength, 0)
			})

			Convey("Check second update reorders", func() {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsUpdate))
				So(UpdateProjects(c), ShouldBeNil)

				Convey("Check updated Project entities", func() {
					foo := &Project{ID: "foo"}
					So(datastore.Get(c, foo), ShouldBeNil)
					So(foo.HasConfig, ShouldBeTrue)
					So(foo.ACL, ShouldResemble, ACL{
						Identities: []identity.Identity{"user:a@example.com"},
					})

					bar := &Project{ID: "bar"}
					So(datastore.Get(c, bar), ShouldBeNil)
					So(bar.HasConfig, ShouldBeFalse)
					So(bar.ACL, ShouldResemble, ACL{})

					So(datastore.Get(c, &Project{ID: "baz"}), ShouldEqual, datastore.ErrNoSuchEntity)
				})

				Convey("Check Console config removed", func() {
					cs, err := GetConsole(c, "foo", "default")
					So(err, ShouldNotBeNil)
					So(cs, ShouldBeNil)
				})

				Convey("Check builder group configs in correct order", func() {
					cs, err := GetConsole(c, "foo", "default_header")
					So(err, ShouldBeNil)
					So(cs.ID, ShouldEqual, "default_header")
					So(cs.Ordinal, ShouldEqual, 0)
					So(cs.Def.Header.Id, ShouldEqual, "main_header")
					So(cs.Def.Header.TreeStatusHost, ShouldEqual, "blarg.example.com")
					cs, err = GetConsole(c, "foo", "console.bar")
					So(err, ShouldBeNil)
					So(cs.ID, ShouldEqual, "console.bar")
					So(cs.Ordinal, ShouldEqual, 1)
					So(cs.Builders, ShouldResemble, []string{"buildbucket/luci.foo.something/bar"})

					cs, err = GetConsole(c, "foo", "console.baz")
					So(err, ShouldBeNil)
					So(cs.ID, ShouldEqual, "console.baz")
					So(cs.Ordinal, ShouldEqual, 2)
					So(cs.Builders, ShouldResemble, []string{"buildbucket/luci.foo.other/baz"})
				})

				Convey("Check getting project builder groups in correct order", func() {
					cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:a@example.com"})
					cs, err := GetProjectConsoles(cUser, "foo")
					So(err, ShouldBeNil)

					ids := make([]string, 0, len(cs))
					for _, c := range cs {
						ids = append(ids, c.ID)
					}
					So(ids, ShouldResemble, []string{"default_header", "console.bar", "console.baz"})
				})
			})

			Convey("Check removing Milo config only", func() {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsNoConsole))
				So(UpdateProjects(c), ShouldBeNil)

				Convey("Check kept the Project entity", func() {
					foo := &Project{ID: "foo"}
					So(datastore.Get(c, foo), ShouldBeNil)
					So(foo.HasConfig, ShouldBeFalse)
					So(foo.ACL, ShouldResemble, ACL{
						Groups:     []string{"a", "b"},
						Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
					})
				})

				Convey("Check removed the console", func() {
					cs, err := GetConsole(c, "foo", "default")
					So(err, ShouldNotBeNil)
					So(cs, ShouldBeNil)
				})
			})

			Convey("Check applying broken config", func() {
				c := cfgclient.Use(c, memcfg.New(mockedConfigsBroken))
				So(UpdateProjects(c), ShouldNotBeNil)

				Convey("Check kept the Project entity", func() {
					foo := &Project{ID: "foo"}
					So(datastore.Get(c, foo), ShouldBeNil)
					So(foo.HasConfig, ShouldBeTrue)
					So(foo.ACL, ShouldResemble, ACL{
						Groups:     []string{"a", "b"},
						Identities: []identity.Identity{"user:a@example.com", "user:b@example.com"},
					})
				})

				Convey("Check kept the console", func() {
					_, err := GetConsole(c, "foo", "default")
					So(err, ShouldBeNil)
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
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
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
